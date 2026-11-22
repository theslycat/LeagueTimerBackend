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
	// ErrRaceNotRunning 比赛非 running 状态，无法记录成绩
	ErrRaceNotRunning = errors.New("比赛未开始或已暂停")
	// ErrRecordDuplicated 同一比赛同一选手已有记录（唯一索引冲突）
	ErrRecordDuplicated = errors.New("该选手已记录交接时间")
	// ErrRaceAlreadyRunning start 时比赛已在进行中
	ErrRaceAlreadyRunning = errors.New("比赛已在进行中")
	// ErrRaceNotStarted pause 时比赛尚未开始
	ErrRaceNotStarted = errors.New("比赛尚未开始")
	// ErrRacePaused pause 时比赛已处于暂停状态
	ErrRacePaused = errors.New("比赛已暂停")
	// ErrRaceFinished start/pause/terminate 时比赛已终止，需先 reset
	ErrRaceFinished = errors.New("比赛已终止，请先重置")
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

// startTransitionError start 的乐观更新未命中时，结合当前状态给出错误
func startTransitionError(status string) error {
	switch status {
	case model.StatusRunning:
		return ErrRaceAlreadyRunning
	case model.StatusFinished:
		return ErrRaceFinished
	default:
		return errors.New("当前比赛状态不允许开始")
	}
}

// pauseTransitionError pause 时按当前状态给出错误
func pauseTransitionError(status string) error {
	switch status {
	case model.StatusIdle:
		return ErrRaceNotStarted
	case model.StatusPaused:
		return ErrRacePaused
	case model.StatusFinished:
		return ErrRaceFinished
	default:
		return errors.New("当前比赛状态不允许暂停")
	}
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
	var teams []model.Team
	err := db.Get().Order("id ASC").Find(&teams).Error
	return teams, err
}

// ListRunners 选手列表（按 team_id、no 升序）
func (imp *RunnerInterfaceImp) ListRunners() ([]model.Runner, error) {
	var runners []model.Runner
	err := db.Get().Order("team_id ASC, no ASC").Find(&runners).Error
	return runners, err
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

// StartRace 开始比赛：idle|paused -> running（乐观更新，WHERE 命中状态才生效）
func (imp *RaceInterfaceImp) StartRace(now time.Time) (*model.Race, error) {
	cli := db.Get()
	res := cli.Model(&model.Race{}).
		Where("id = ? AND status IN ?", model.SingleRaceId,
			[]string{model.StatusIdle, model.StatusPaused}).
		Updates(map[string]interface{}{
			"status":     model.StatusRunning,
			"started_at": now,
			"updated_at": now,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		// 状态竞争失败：读回当前状态给出具体错误
		race, err := imp.GetRace()
		if err != nil {
			return nil, err
		}
		return nil, startTransitionError(race.Status)
	}
	return imp.GetRace()
}

// PauseRace 暂停比赛：running -> paused。
// 行锁事务内读取-冻结时钟，started_at 置 NULL，base_ms 累加本段耗时。
func (imp *RaceInterfaceImp) PauseRace(now time.Time) (*model.Race, error) {
	cli := db.Get()
	var paused *model.Race
	err := cli.Transaction(func(tx *gorm.DB) error {
		race, err := loadRaceForUpdate(tx)
		if err != nil {
			return err
		}
		if race.Status != model.StatusRunning {
			return pauseTransitionError(race.Status)
		}
		baseMs := elapsedSince(now, race.StartedAt, race.BaseMs)
		if err := updateRace(tx, map[string]interface{}{
			"status":     model.StatusPaused,
			"base_ms":    baseMs,
			"started_at": nil,
			"updated_at": now,
		}); err != nil {
			return err
		}
		paused = &model.Race{
			Id:        race.Id,
			Status:    model.StatusPaused,
			StartedAt: nil,
			BaseMs:    baseMs,
			UpdatedAt: now,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paused, nil
}

// TerminateRace 终止比赛：running|paused|idle -> finished。
// running 时先按 pause 逻辑冻结时钟再置 finished。
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
			// 先冻结时钟：把本段耗时并入 base_ms
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

// ResetRace 重置比赛：任意状态 -> idle，base_ms 清零，并删除全部交接记录。
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
			"updated_at": now,
		}); err != nil {
			return err
		}
		// 清空全部交接记录（比赛为单例，等价于删除所有 record）
		if err := tx.Where("race_id = ?", model.SingleRaceId).
			Delete(&model.Record{}).Error; err != nil {
			return err
		}
		reset = &model.Race{
			Id:        model.SingleRaceId,
			Status:    model.StatusIdle,
			StartedAt: nil,
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

// ListRecords 交接记录列表（按 pass_ms 升序，同成绩按 id 升序）
func (imp *RecordInterfaceImp) ListRecords() ([]model.Record, error) {
	var records []model.Record
	err := db.Get().Order("pass_ms ASC, id ASC").Find(&records).Error
	return records, err
}

// CreateRecord 记录交接时间。
// 事务内锁定比赛单例行：校验 running 状态、计算 server_ms、防重复记录，
// 唯一索引 (race_id, runner_id) 兜底并发插入冲突。
func (imp *RecordInterfaceImp) CreateRecord(now time.Time, runnerId int64, passMs int64, recordedBy string, clientAt *int64) (*model.Record, error) {
	cli := db.Get()
	var created *model.Record
	err := cli.Transaction(func(tx *gorm.DB) error {
		race, err := loadRaceForUpdate(tx)
		if err != nil {
			return err
		}
		if race.Status != model.StatusRunning {
			return ErrRaceNotRunning
		}
		var count int64
		if err := tx.Model(&model.Record{}).
			Where("race_id = ? AND runner_id = ?", model.SingleRaceId, runnerId).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrRecordDuplicated
		}
		record := &model.Record{
			RaceId:     model.SingleRaceId,
			RunnerId:   runnerId,
			PassMs:     passMs,
			ServerMs:   elapsedSince(now, race.StartedAt, race.BaseMs),
			RecordedBy: recordedBy,
			ClientAt:   clientAt,
		}
		if err := tx.Create(record).Error; err != nil {
			if IsDuplicateEntryError(err) {
				return ErrRecordDuplicated
			}
			return err
		}
		created = record
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}
