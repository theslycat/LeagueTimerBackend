package model

import "time"

// 比赛事件类型：计时系统为追加式事件流，一切状态由名单 + 事件推导
const (
	// EventExchange 交接：前一棒完成计时，同时为下一棒开始计时（第6棒冲线时无下一棒）
	EventExchange = "exchange"
	// EventBackupStart 备用黑色接力带出发（前棒满 50 分钟未交接或已退赛）
	EventBackupStart = "backup_start"
	// EventSoloFinish 补录到达：备用带已发出后，原超时选手完成本人完整赛段
	EventSoloFinish = "solo_finish"
	// EventWithdraw 退赛：选手中止比赛，本队失去团体排名资格
	EventWithdraw = "withdraw"
)

// 接力带类型
const (
	BeltNormal = "normal" // 常规交接带
	BeltBlack  = "black"  // 备用黑色接力带
)

// 完成方式
const (
	FinishExchange = "exchange" // 常规交接完成
	FinishSolo     = "solo"     // 备用带发出后补录完成
)

// RaceEvent 比赛事件模型
// RunnerId     事件主体选手：交接 = 到达交接区的选手；其余类型 = 该选手本人
// NextRunnerId 交接事件自动派生的下一棒（第 6 棒冲线时为 NULL）
// Ms           事件发生时刻（自开赛累计毫秒，与界面比赛时钟同基准）
// Early        备用带出发早于规则允许时刻（前棒出发 + 50 分钟），经确认放行
type RaceEvent struct {
	Id           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	RaceId       int64     `gorm:"column:race_id;index" json:"raceId"`
	Type         string    `gorm:"column:type;size:16" json:"type"`
	RunnerId     int64     `gorm:"column:runner_id;index" json:"runnerId"`
	NextRunnerId *int64    `gorm:"column:next_runner_id" json:"nextRunnerId"`
	Ms           int64     `gorm:"column:ms" json:"ms"`
	Early        bool      `gorm:"column:early" json:"early"`
	ServerMs     int64     `gorm:"column:server_ms" json:"serverMs"`
	RecordedBy   string    `gorm:"column:recorded_by;size:64" json:"recordedBy"`
	ClientAt     *int64    `gorm:"column:client_at" json:"clientAt"`
	CreatedAt    time.Time `gorm:"column:created_at;type:datetime(3)" json:"createdAt"`
}
