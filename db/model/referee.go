package model

import "time"

// 裁判角色常量（主裁判不落库，referee 表只存副裁判）
const (
	RoleChief  = "chief"
	RoleDeputy = "deputy"
)

// Referee 副裁判模型（用户名唯一，密码以 PBKDF2-SHA256 摘要存储）
type Referee struct {
	Id           int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"column:username;size:64;uniqueIndex" json:"username"`
	PasswordHash string    `gorm:"column:password_hash;size:256" json:"-"`
	DisplayName  string    `gorm:"column:display_name;size:64" json:"displayName"`
	Role         string    `gorm:"column:role;size:16" json:"role"`
	CreatedAt    time.Time `gorm:"column:created_at;type:datetime(3)" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;type:datetime(3)" json:"updatedAt"`
}
