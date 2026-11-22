package dao

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"wxcloudrun-golang/db"
	"wxcloudrun-golang/db/model"
)

// 业务哨兵错误：文案即对外展示的错误信息
var (
	// ErrRaceNotRunning 比赛非 running 状态，无法记录事件
	ErrRaceNotRunning = errors.New("比赛未开始或已结束")
	// ErrRaceAlreadyRunning start 时比赛已在进行中
	ErrRaceAlreadyRunning = errors.New("比赛已在进行中")
	// ErrRaceNotStarted 操作时比赛尚未开始
	ErrRaceNotStarted = errors.New("比赛尚未开始")
	// ErrRaceFinished start/terminate 时比赛已终止，需先 reset
	ErrRaceFinished = errors.New("比赛已终止，请先重置")
	// ErrRaceNotIdle 名单替换等仅 idle 状态允许的操作
	ErrRaceNotIdle = errors.New("比赛已开始，重置比赛后才能修改名单")
	// ErrEventNotFound 待撤销的事件不存在
	ErrEventNotFound = errors.New("该记录不存在")
)

// IsDuplicateEntryError 判断是否为 MySQL 唯一索引冲突（Error 1062）
func IsDuplicateEntryError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate entry") || strings.Contains(msg, "error 1062")
}

// elapsedSince 计算自 startedAt 起经过的毫秒数
func elapsedSince(now time.Time, startedAt *time.Time, baseMs int64) int64 {
	elapsed := baseMs
	if startedAt != nil {
		if delta := now.Sub(*startedAt).Milliseconds(); delta > 0 {
			elapsed += delta
		}
	}
	return elapsed
}

// loadRaceForUpdate 在事务中锁定并读取比赛单例行（SELECT ... FOR UPDATE）
func loadRaceForUpdate(tx *gorm.DB) (*model.Race, error) {
	var race model.Race
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", model.SingleRaceId).First(&race).Error; err != nil {
		return nil, err
	}
	return &race, nil
}

// updateRace 更新比赛单例行
func updateRace(tx *gorm.DB, updates map[string]interface{}) error {
	return tx.Model(&model.Race{}).
		Where("id = ?", model.SingleRaceId).Updates(updates).Error
}

// GetRefereeByUsername 按用户名查询副裁判
func (imp *RefereeInterfaceImp) GetRefereeByUsername(username string) (*model.Referee, error) {
	var referee model.Referee
	err := db.Get().Where("username = ?", username).First(&referee).Error
	if err != nil {
		return nil, err
	}
	return &referee, nil
}

// ListReferees 副裁判列表（仅 deputy，按创建时间升序）
func (imp *RefereeInterfaceImp) ListReferees() ([]model.Referee, error) {
	var referees []model.Referee
	err := db.Get().Where("role = ?", model.RoleDeputy).
		Order("created_at ASC, id ASC").Find(&referees).Error
	return referees, err
}

// CreateReferee 创建副裁判
func (imp *RefereeInterfaceImp) CreateReferee(referee *model.Referee) error {
	return db.Get().Create(referee).Error
}

// DeleteRefereeByUsername 按用户名删除副裁判，返回删除行数
func (imp *RefereeInterfaceImp) DeleteRefereeByUsername(username string) (int64, error) {
	res := db.Get().Where("username = ? AND role = ?", username, model.RoleDeputy).
		Delete(&model.Referee{})
	return res.RowsAffected, res.Error
}

// ListTeams 队伍列表（按 id 升序）
func (imp *TeamInterfaceImp) ListTeams() ([]model.Team, error) {
	return LoadTeams(db.Get())
}

// ListRunners 选手列表（按 team_id、no 升序）
func (imp *RunnerInterfaceImp) ListRunners() ([]model.Runner, error) {
	return LoadRunners(db.Get())
}

// GetRunnerById 按主键查询选手
func (imp *RunnerInterfaceImp) GetRunnerById(id int64) (*model.Runner, error) {
	var runner model.Runner
	err := db.Get().Where("id = ?", id).First(&runner).Error
	if err != nil {
		return nil, err
	}
	return &runner, nil
}

// GetRace 读取比赛单例行
func (imp *RaceInterfaceImp) GetRace() (*model.Race, error) {
	var race model.Race
	err := db.Get().Where("id = ?", model.SingleRaceId).First(&race).Error
	if err != nil {
		return nil, err
	}
	return &race, nil
}

// StartRace 发令开始比赛：idle|paused -> running。
// 行锁事务内设置 started_at 与 gun_at（gun_at 保留首次发令时刻）。
func (imp *RaceInterfaceImp) StartRace(now time.Time) (*model.Race, error) {
	cli := db.Get()
	var started *model.Race
	err := cli.Transaction(func(tx *gorm.DB) error {
		race, err := loadRaceForUpdate(tx)
		if err != nil {
			return err
		}
		if race.Status != model.StatusIdle && race.Status != model.StatusPaused {
			if race.Status == model.StatusRunning {
				return ErrRaceAlreadyRunning
			}
			return ErrRaceFinished
		}
		updates := map[string]interface{}{
			"status":     model.StatusRunning,
			"started_at": now,
			"updated_at": now,
		}
		// 首次发令记录 gun_at；异常暂停后恢复不覆盖原发令时刻
		if race.GunAt == nil {
			updates["gun_at"] = now
		}
		if err := updateRace(tx, updates); err != nil {
			return err
		}
		started = &model.Race{
			Id:        race.Id,
			Status:    model.StatusRunning,
			StartedAt: &now,
			GunAt:     race.GunAt,
			BaseMs:    race.BaseMs,
			UpdatedAt: now,
		}
		if race.GunAt == nil {
			started.GunAt = &now
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return started, nil
}

// TerminateRace 终止比赛：running|paused|idle -> finished。
// running 时先冻结时钟再置 finished；gun_at 保留用于回显。
func (imp *RaceInterfaceImp) TerminateRace(now time.Time) (*model.Race, error) {
	cli := db.Get()
	var finished *model.Race
	err := cli.Transaction(func(tx *gorm.DB) error {
		race, err := loadRaceForUpdate(tx)
		if err != nil {
			return err
		}
		if race.Status == model.StatusFinished {
			return ErrRaceFinished
		}
		baseMs := race.BaseMs
		updates := map[string]interface{}{
			"status":     model.StatusFinished,
			"updated_at": now,
		}
		if race.Status == model.StatusRunning {
			baseMs = elapsedSince(now, race.StartedAt, race.BaseMs)
			updates["base_ms"] = baseMs
			updates["started_at"] = nil
		}
		if err := updateRace(tx, updates); err != nil {
			return err
		}
		finished = &model.Race{
			Id:        race.Id,
			Status:    model.StatusFinished,
			StartedAt: nil,
			GunAt:     race.GunAt,
			BaseMs:    baseMs,
			UpdatedAt: now,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return finished, nil
}

// ResetRace 重置比赛：任意状态 -> idle，计时清零，清空全部比赛事件。
// 名单保留（如需修改名单，主裁判在 idle 状态下重新导入）。
func (imp *RaceInterfaceImp) ResetRace(now time.Time) (*model.Race, error) {
	cli := db.Get()
	var reset *model.Race
	err := cli.Transaction(func(tx *gorm.DB) error {
		if _, err := loadRaceForUpdate(tx); err != nil {
			return err
		}
		if err := updateRace(tx, map[string]interface{}{
			"status":     model.StatusIdle,
			"base_ms":    0,
			"started_at": nil,
			"gun_at":     nil,
			"updated_at": now,
		}); err != nil {
			return err
		}
		// 清空全部比赛事件（比赛为单例，等价于删除所有事件）
		if err := tx.Where("race_id = ?", model.SingleRaceId).
			Delete(&model.RaceEvent{}).Error; err != nil {
			return err
		}
		reset = &model.Race{
			Id:        model.SingleRaceId,
			Status:    model.StatusIdle,
			StartedAt: nil,
			GunAt:     nil,
			BaseMs:    0,
			UpdatedAt: now,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reset, nil
}

// ListEvents 全部比赛事件（按 ms、id 升序）
func (imp *EventInterfaceImp) ListEvents() ([]model.RaceEvent, error) {
	return LoadEvents(db.Get())
}

// ReplaceRoster 整体替换参赛名单（仅 idle）：
// 清空队伍、选手与历史事件后写入新名单。
func (imp *ParticipantInterfaceImp) ReplaceRoster(now time.Time, input []RosterTeamInput) error {
	cli := db.Get()
	return cli.Transaction(func(tx *gorm.DB) error {
		race, err := loadRaceForUpdate(tx)
		if err != nil {
			return err
		}
		if race.Status != model.StatusIdle {
			return ErrRaceNotIdle
		}
		if err := tx.Where("race_id = ?", model.SingleRaceId).
			Delete(&model.RaceEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).
			Delete(&model.Runner{}).Error; err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).
			Delete(&model.Team{}).Error; err != nil {
			return err
		}
		for i := range input {
			team := model.Team{Name: input[i].Name}
			if err := tx.Create(&team).Error; err != nil {
				return err
			}
			for j := range input[i].Runners {
				runner := input[i].Runners[j]
				runner.Id = 0
				runner.TeamId = team.Id
				if err := tx.Create(&runner).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
