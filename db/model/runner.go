package model

// Runner 选手模型
type Runner struct {
	Id     int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TeamId int64  `gorm:"column:team_id;index" json:"teamId"`
	No     int    `gorm:"column:no" json:"no"`
	Name   string `gorm:"column:name;size:64" json:"name"`
}
