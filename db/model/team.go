package model

// Team 队伍模型
type Team struct {
	Id   int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name string `gorm:"column:name;size:64;uniqueIndex" json:"name"`
}
