package dao

import (
	"time"

	"gorm.io/gorm"

	"wxcloudrun-golang/db"
	"wxcloudrun-golang/db/model"
)

// RefereeInterface 副裁判数据模型接口
type RefereeInterface interface {
	GetRefereeByUsername(username string) (*model.Referee, error)
	ListReferees() ([]model.Referee, error)
	CreateReferee(referee *model.Referee) error
	DeleteRefereeByUsername(username string) (int64, error)
}

// RefereeInterfaceImp 副裁判数据模型实现
type RefereeInterfaceImp struct{}

// TeamInterface 队伍数据模型接口
type TeamInterface interface {
	ListTeams() ([]model.Team, error)
}

// TeamInterfaceImp 队伍数据模型实现
type TeamInterfaceImp struct{}

// RunnerInterface 选手数据模型接口
type RunnerInterface interface {
	ListRunners() ([]model.Runner, error)
	GetRunnerById(id int64) (*model.Runner, error)
}

// RunnerInterfaceImp 选手数据模型实现
type RunnerInterfaceImp struct{}

// RaceInterface 比赛单例数据模型接口
// 按赛制没有「暂停」：发令后计时不可中止（误发令用终止 + 重置补救），
// 因此不再提供 PauseRace。
type RaceInterface interface {
	GetRace() (*model.Race, error)
	StartRace(now time.Time) (*model.Race, error)
	TerminateRace(now time.Time) (*model.Race, error)
	ResetRace(now time.Time) (*model.Race, error)
}

// RaceInterfaceImp 比赛单例数据模型实现
type RaceInterfaceImp struct{}

// EventInterface 比赛事件数据模型接口（读取；写入走 WithRaceTx 事务）
type EventInterface interface {
	ListEvents() ([]model.RaceEvent, error)
}

// EventInterfaceImp 比赛事件数据模型实现
type EventInterfaceImp struct{}

// RosterTeamInput 名单整体替换的输入：队伍名 + 六棒选手（TeamId 由写入时分配）
type RosterTeamInput struct {
	Name    string
	Runners []model.Runner
}

// ParticipantInterface 参赛名单数据模型接口
type ParticipantInterface interface {
	// ReplaceRoster 事务内锁定比赛单例行：仅 idle 状态允许整体替换名单，
	// 同时清空历史事件，防止名单与事件错位。
	ReplaceRoster(now time.Time, input []RosterTeamInput) error
}

// ParticipantInterfaceImp 参赛名单数据模型实现
type ParticipantInterfaceImp struct{}

// 各数据模型实现实例
var (
	RefereeImp     RefereeInterface     = &RefereeInterfaceImp{}
	TeamImp        TeamInterface        = &TeamInterfaceImp{}
	RunnerImp      RunnerInterface      = &RunnerInterfaceImp{}
	RaceImp        RaceInterface        = &RaceInterfaceImp{}
	EventImp       EventInterface       = &EventInterfaceImp{}
	ParticipantImp ParticipantInterface = &ParticipantInterfaceImp{}
)

// WithRaceTx 锁定比赛单例行（SELECT ... FOR UPDATE）后执行 fn：
// 所有事件写入 / 撤销 / 名单替换都在该锁内串行执行，避免并发推导竞态。
func WithRaceTx(fn func(tx *gorm.DB, race *model.Race) error) error {
	return db.Get().Transaction(func(tx *gorm.DB) error {
		race, err := loadRaceForUpdate(tx)
		if err != nil {
			return err
		}
		return fn(tx, race)
	})
}

// ListTeamsTx / LoadTeams 读取队伍列表（按 id 升序）
func LoadTeams(cli *gorm.DB) ([]model.Team, error) {
	var teams []model.Team
	err := cli.Order("id ASC").Find(&teams).Error
	return teams, err
}

// LoadRunners 读取选手列表（按 team_id、no 升序）
func LoadRunners(cli *gorm.DB) ([]model.Runner, error) {
	var runners []model.Runner
	err := cli.Order("team_id ASC, no ASC").Find(&runners).Error
	return runners, err
}

// LoadEvents 读取全部比赛事件（按 ms、id 升序，即事件发生顺序）
func LoadEvents(cli *gorm.DB) ([]model.RaceEvent, error) {
	var events []model.RaceEvent
	err := cli.Order("ms ASC, id ASC").Find(&events).Error
	return events, err
}
