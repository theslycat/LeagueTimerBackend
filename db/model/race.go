package model

import "time"

// 比赛状态常量：比赛为 id=1 的单例行
const (
	StatusIdle     = "idle"
	StatusRunning  = "running"
	StatusPaused   = "paused"
	StatusFinished = "finished"
)

// SingleRaceId 比赛单例行主键，恒为 1
const SingleRaceId int64 = 1

// Race 比赛单例模型
// Status    当前状态：idle|running|paused|finished
// StartedAt 当前这一运行段的起始时刻（终止后置 NULL）
// GunAt     发令墙钟时刻（8:00 统一发令；终止后保留用于回显绝对时间）
// BaseMs    冻结前累计的毫秒数（elapsed = base_ms + running ? now-started_at : 0）
type Race struct {
	Id        int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Status    string     `gorm:"column:status;size:16" json:"status"`
	StartedAt *time.Time `gorm:"column:started_at;type:datetime(3)" json:"startedAt"`
	GunAt     *time.Time `gorm:"column:gun_at;type:datetime(3)" json:"gunAt"`
	BaseMs    int64      `gorm:"column:base_ms;default:0" json:"baseMs"`
	UpdatedAt time.Time  `gorm:"column:updated_at;type:datetime(3)" json:"updatedAt"`
}
