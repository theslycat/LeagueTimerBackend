package model

// 赛制常量：全队六棒、每人一个赛段（第一/三/五棒 A 段，第二/四/六棒 B 段）
const (
	LegsPerTeam = 6
)

// Runner 选手模型
// No     棒次 1-6（同时是交接顺序）
// Gender 性别：M|F。按赛制第一至五棒为男生，第六棒为女生
type Runner struct {
	Id     int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TeamId int64  `gorm:"column:team_id;index" json:"teamId"`
	No     int    `gorm:"column:no" json:"no"`
	Name   string `gorm:"column:name;size:64" json:"name"`
	Gender string `gorm:"column:gender;size:1;default:M" json:"gender"`
}
