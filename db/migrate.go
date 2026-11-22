package db

import (
	"gorm.io/gorm"

	"wxcloudrun-golang/db/model"
)

// Migrate 自动建表（gorm AutoMigrate）并执行幂等种子数据初始化。
// main.go 中 db.Init 成功后调用。
func Migrate() error {
	cli := Get()
	if err := cli.AutoMigrate(
		&model.Referee{},
		&model.Team{},
		&model.Runner{},
		&model.Race{},
		&model.Record{},
	); err != nil {
		return err
	}
	return seed(cli)
}

// seed 幂等种子：race 单例行、演示队伍与选手。
// 仅在对应表为空时写入，重复启动不会产生重复数据。
func seed(cli *gorm.DB) error {
	// race 单例：idle
	var raceCount int64
	if err := cli.Model(&model.Race{}).Count(&raceCount).Error; err != nil {
		return err
	}
	if raceCount == 0 {
		race := model.Race{Id: model.SingleRaceId, Status: model.StatusIdle, BaseMs: 0}
		if err := cli.Create(&race).Error; err != nil {
			return err
		}
	}

	// 队伍与选手（空表才插入，沿用前端演示名单）
	var teamCount int64
	if err := cli.Model(&model.Team{}).Count(&teamCount).Error; err != nil {
		return err
	}
	if teamCount == 0 {
		type seedRunner struct {
			No   int
			Name string
		}
		type seedTeam struct {
			Name    string
			Runners []seedRunner
		}
		teams := []seedTeam{
			{"青云跑团", []seedRunner{{101, "陈家豪"}, {102, "林亦舟"}}},
			{"山岳学院", []seedRunner{{103, "周子昂"}, {104, "苏婉清"}}},
			{"江畔大学", []seedRunner{{105, "何宇航"}, {106, "顾晓岚"}}},
			{"北岭俱乐部", []seedRunner{{107, "邵一鸣"}, {108, "罗雨桐"}}},
			{"曙光中学", []seedRunner{{109, "高梓涵"}, {110, "郑天翊"}}},
		}
		for _, st := range teams {
			team := model.Team{Name: st.Name}
			if err := cli.Create(&team).Error; err != nil {
				return err
			}
			for _, sr := range st.Runners {
				runner := model.Runner{TeamId: team.Id, No: sr.No, Name: sr.Name}
				if err := cli.Create(&runner).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}
