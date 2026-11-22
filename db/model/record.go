package model

import "time"

// Record 交接时间记录模型
// PassMs     成绩：自开赛累计毫秒数
// ServerMs   服务端落库那一刻比赛的累计毫秒数（审计用）
// ClientAt   客户端上报的墙钟毫秒时间戳，可空
type Record struct {
	Id         int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	RaceId     int64     `gorm:"column:race_id;uniqueIndex:uk_race_runner" json:"raceId"`
	RunnerId   int64     `gorm:"column:runner_id;uniqueIndex:uk_race_runner" json:"runnerId"`
	PassMs     int64     `gorm:"column:pass_ms" json:"passMs"`
	ServerMs   int64     `gorm:"column:server_ms" json:"serverMs"`
	RecordedBy string    `gorm:"column:recorded_by;size:64" json:"recordedBy"`
	ClientAt   *int64    `gorm:"column:client_at" json:"clientAt"`
	CreatedAt  time.Time `gorm:"column:created_at;type:datetime(3)" json:"createdAt"`
}
