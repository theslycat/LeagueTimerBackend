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
		&model.RaceEvent{},
	); err != nil {
		return err
	}
	return seed(cli)
}

// seed 幂等种子：race 单例行、演示队伍与选手。
// 仅在对应表为空时写入，重复启动不会产生重复数据。
// 演示名单遵循赛制：六棒制，第一至五棒男生、第六棒女生。
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

	// 队伍与选手（空表才插入；正式名单由主裁判在登录页导入）
	var teamCount int64
	if err := cli.Model(&model.Team{}).Count(&teamCount).Error; err != nil {
		return err
	}
	if teamCount == 0 {
		male := func(no int, name string) model.Runner {
			return model.Runner{No: no, Name: name, Gender: "M"}
		}
		female := func(no int, name string) model.Runner {
			return model.Runner{No: no, Name: name, Gender: "F"}
		}
		teams := [][]model.Runner{
			{
				male(1, "陈家豪"), male(2, "林亦舟"), male(3, "周子昂"),
				male(4, "何宇航"), male(5, "邵一鸣"), female(6, "苏婉清"),
			},
			{
				male(1, "高梓涵"), male(2, "郑天翊"), male(3, "罗雨桐"),
				male(4, "顾晓岚"), male(5, "王靖杰"), female(6, "李静姝"),
			},
		}
		for i, runners := range teams {
			team := model.Team{Name: "演示跑团 " + string(rune('A'+i))}
			if err := cli.Create(&team).Error; err != nil {
				return err
			}
			for j := range runners {
				runner := runners[j]
				runner.TeamId = team.Id
				if err := cli.Create(&runner).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}
