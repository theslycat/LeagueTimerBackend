package dao

import (
	"time"

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
type RaceInterface interface {
	GetRace() (*model.Race, error)
	StartRace(now time.Time) (*model.Race, error)
	PauseRace(now time.Time) (*model.Race, error)
	TerminateRace(now time.Time) (*model.Race, error)
	ResetRace(now time.Time) (*model.Race, error)
}

// RaceInterfaceImp 比赛单例数据模型实现
type RaceInterfaceImp struct{}

// RecordInterface 交接记录数据模型接口
type RecordInterface interface {
	ListRecords() ([]model.Record, error)
	CreateRecord(now time.Time, runnerId int64, passMs int64, recordedBy string, clientAt *int64) (*model.Record, error)
}

// RecordInterfaceImp 交接记录数据模型实现
type RecordInterfaceImp struct{}

// 各数据模型实现实例
var (
	RefereeImp RefereeInterface = &RefereeInterfaceImp{}
	TeamImp    TeamInterface    = &TeamInterfaceImp{}
	RunnerImp  RunnerInterface  = &RunnerInterfaceImp{}
	RaceImp    RaceInterface    = &RaceInterfaceImp{}
	RecordImp  RecordInterface  = &RecordInterfaceImp{}
)
