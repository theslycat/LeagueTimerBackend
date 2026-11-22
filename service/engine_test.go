package service

import (
	"testing"

	"wxcloudrun-golang/db/model"
)

// demoRoster 两队六棒名单：第一至五棒男生、第六棒女生
func demoRoster() ([]model.Team, []model.Runner) {
	teams := []model.Team{{Id: 1, Name: "青云跑团"}, {Id: 2, Name: "山岳学院"}}
	runners := make([]model.Runner, 0, 12)
	for t := int64(1); t <= 2; t++ {
		for no := 1; no <= 6; no++ {
			gender := "M"
			if no == 6 {
				gender = "F"
			}
			runners = append(runners, model.Runner{
				Id: t*100 + int64(no), TeamId: t, No: no, Name: "选手", Gender: gender,
			})
		}
	}
	return teams, runners
}

func ev(typ string, runnerId int64, ms int64) model.RaceEvent {
	return model.RaceEvent{Id: ms, Type: typ, RunnerId: runnerId, Ms: ms}
}

func TestRuleTimeLimit(t *testing.T) {
	if RuleTimeLimitMs != 50*60*1000 {
		t.Fatalf("个人时限应为 50 分钟，实际 %d", RuleTimeLimitMs)
	}
}

func TestFirstLegStartsOnGun(t *testing.T) {
	teams, runners := demoRoster()
	snap := BuildSnapshot(teams, runners, nil, true)
	for _, ts := range snap.Teams {
		first := ts.Leg(1)
		if !first.Started || first.StartMs != 0 || first.Belt != model.BeltNormal {
			t.Fatalf("发令后第一棒应在 0ms 常规带出发：%+v", first)
		}
	}
	// 未发令
	snapIdle := BuildSnapshot(teams, runners, nil, false)
	if snapIdle.Teams[0].Leg(1).Started {
		t.Fatal("未发令时第一棒不应出发")
	}
}

func TestExchangeChainStartsNextLeg(t *testing.T) {
	teams, runners := demoRoster()
	r1 := runners[0] // 队1 第1棒
	r2 := runners[1] // 队1 第2棒
	next := r2.Id
	evx := model.RaceEvent{Type: model.EventExchange, RunnerId: r1.Id, NextRunnerId: &next, Ms: 1500000}
	snap := BuildSnapshot(teams, runners, []model.RaceEvent{evx}, true)

	leg1 := snap.Teams[0].Leg(1)
	leg2 := snap.Teams[0].Leg(2)
	if !leg1.Finished || leg1.FinishMs != 1500000 || leg1.FinishType != model.FinishExchange {
		t.Fatalf("第一棒应交接完成：%+v", leg1)
	}
	if !leg2.Started || leg2.StartMs != 1500000 || leg2.Belt != model.BeltNormal {
		t.Fatalf("交接应同时启动第二棒：%+v", leg2)
	}
	if d := leg1.DurationMs(); *d != 1500000 {
		t.Fatalf("第一棒实际用时应为 25:00，实际 %d", *d)
	}
}

func TestLegSixFinishIsTerminal(t *testing.T) {
	teams, runners := demoRoster()
	// 完成前五棒交接链后，第六棒在途
	snap := snapAfter(teams, runners)
	leg6 := snap.Teams[0].Leg(6)
	if !leg6.OnCourse() {
		t.Fatalf("前五棒交接后第六棒应在途：%+v", leg6)
	}
	// 第六棒交换校验：无下一棒（冲线），返回 nil next
	events := evChain(teams, runners, 1, 5)
	r6 := runnerOf(runners, 1, 6)
	next, rerr := BuildSnapshot(teams, runners, events, true).CheckExchange(r6.Id)
	if rerr != nil {
		t.Fatalf("第六棒应允许冲线：%v", rerr)
	}
	if next != nil {
		t.Fatal("第六棒冲线不应有下一棒")
	}
	// 冲线后全队完成
	events = append(events, model.RaceEvent{Id: 2000, Type: model.EventExchange, RunnerId: r6.Id, Ms: 9 * 60 * 1000 * 1000})
	snap = BuildSnapshot(teams, runners, events, true)
	if !snap.Teams[0].AllFinished() {
		t.Fatal("第六棒冲线且前五棒完成后应判定全队完成")
	}
}

// snapAfter 辅助：完成指定队前五棒的交接链后构建快照
func snapAfter(teams []model.Team, runners []model.Runner) *Snapshot {
	return BuildSnapshot(teams, runners, evChain(teams, runners, 1, 5), true)
}

// evChain 构造某队第 1..n 棒的交接事件链
func evChain(teams []model.Team, runners []model.Runner, teamId int64, upTo int) []model.RaceEvent {
	id := int64(1000)
	events := make([]model.RaceEvent, 0, upTo)
	for no := 1; no <= upTo; no++ {
		cur := runnerOf(runners, teamId, no)
		var next *int64
		if no < 6 {
			n := runnerOf(runners, teamId, no+1)
			next = &n.Id
		}
		id++
		events = append(events, model.RaceEvent{
			Id: id, Type: model.EventExchange, RunnerId: cur.Id,
			NextRunnerId: next, Ms: int64(no) * 1500000,
		})
	}
	return events
}

func runnerOf(runners []model.Runner, teamId int64, no int) model.Runner {
	for i := range runners {
		if runners[i].TeamId == teamId && runners[i].No == no {
			return runners[i]
		}
	}
	panic("runner not found")
}

func snapRunner(teams []model.Team, runners []model.Runner, teamId int64, no int) model.Runner {
	return runnerOf(runners, teamId, no)
}

func TestExchangeValidation(t *testing.T) {
	teams, runners := demoRoster()
	r1 := runnerOf(runners, 1, 1)

	// 未出发不能交接（比赛未发令）
	snap := BuildSnapshot(teams, runners, nil, false)
	if _, rerr := snap.CheckExchange(r1.Id); rerr == nil {
		t.Fatal("未发令时第一棒不能交接")
	}

	// 已完成后重复交接
	snap = BuildSnapshot(teams, runners, evChain(teams, runners, 1, 1), true)
	if _, rerr := snap.CheckExchange(r1.Id); rerr == nil {
		t.Fatal("已完成选手不能重复交接")
	}

	// 正常交接通过，返回下一棒
	next, rerr := BuildSnapshot(teams, runners, nil, true).CheckExchange(r1.Id)
	if rerr != nil || next == nil || next.No != 2 {
		t.Fatalf("正常交接应返回下一棒：%v %+v", rerr, next)
	}
}

func TestBackupStartValidation(t *testing.T) {
	teams, runners := demoRoster()
	r2 := runnerOf(runners, 1, 2)

	snap := BuildSnapshot(teams, runners, nil, true)
	// 第二棒可出发，但最早时刻 = 第一棒出发(0) + 50min
	if _, rerr := snap.CheckBackupStart(r2.Id, 1000); rerr == nil || rerr.kind != ruleKindEarly {
		t.Fatalf("第一棒在途未满 50 分钟应拒绝备用带：%v", rerr)
	}
	if _, rerr := snap.CheckBackupStart(r2.Id, 50*60*1000); rerr != nil {
		t.Fatalf("满 50 分钟应放行：%v", rerr)
	}

	// 第一棒完成交接后不许备用带
	snap = BuildSnapshot(teams, runners, evChain(teams, runners, 1, 1), true)
	if _, rerr := snap.CheckBackupStart(r2.Id, 99999999); rerr == nil {
		t.Fatal("上一棒已交接，无需备用带")
	}
}

func TestTimeoutScoring(t *testing.T) {
	teams, runners := demoRoster()
	r1 := runnerOf(runners, 1, 1)
	r2 := runnerOf(runners, 1, 2)
	next := r2.Id

	// 第一棒 8:00 出发，9:25（55min）仍未交接；备用带 50min 时发出
	events := []model.RaceEvent{
		{Id: 1, Type: model.EventBackupStart, RunnerId: r2.Id, Ms: 50 * 60 * 1000},
		{Id: 2, Type: model.EventSoloFinish, RunnerId: r1.Id, Ms: 55 * 60 * 1000},
	}
	_ = next
	snap := BuildSnapshot(teams, runners, events, true)
	leg1 := snap.Teams[0].Leg(1)
	if !leg1.Finished || leg1.FinishType != model.FinishSolo {
		t.Fatalf("超时选手应补录完成：%+v", leg1)
	}
	if d := leg1.DurationMs(); *d != 55*60*1000 {
		t.Fatalf("实际用时应 55:00：%d", *d)
	}
	if s := leg1.ScoredMs(); *s != 50*60*1000 {
		t.Fatalf("团体计分应按 50:00：%d", *s)
	}
	if !leg1.Overtime() {
		t.Fatal("应判定为超时")
	}
	leg2 := snap.Teams[0].Leg(2)
	if !leg2.Started || leg2.Belt != model.BeltBlack {
		t.Fatalf("第二棒应黑带出发：%+v", leg2)
	}
	// 第二棒 50min 出发，从自己的实际出发时刻重新计算 50 分钟
	if dl := leg2.DeadlineMs(); dl != nil && *dl != 100*60*1000 {
		t.Fatalf("第二棒时限应为其出发+50min：%d", *dl)
	}
}

func TestWithdrawBreaksQualification(t *testing.T) {
	teams, runners := demoRoster()
	r2 := runnerOf(runners, 1, 2)
	events := append(evChain(teams, runners, 1, 1),
		model.RaceEvent{Id: 99, Type: model.EventWithdraw, RunnerId: r2.Id, Ms: 2000000})
	snap := BuildSnapshot(teams, runners, events, true)
	ts := snap.Teams[0]
	if !ts.HasWithdrawal() || ts.Qualified() {
		t.Fatal("有退赛的队伍不应有团体排名资格")
	}
	// 队2 完成全程后具备资格
	snap2 := BuildSnapshot(teams, runners, evChain(teams, runners, 2, 6), true)
	if !snap2.Teams[1].Qualified() {
		t.Fatal("全员完成且无退赛应有资格")
	}
}

func TestSoloFinishValidation(t *testing.T) {
	teams, runners := demoRoster()
	r1 := runnerOf(runners, 1, 1)
	r6 := runnerOf(runners, 1, 6)

	snap := BuildSnapshot(teams, runners, nil, true)
	if rerr := snap.CheckSoloFinish(r1.Id); rerr == nil {
		t.Fatal("下一棒未出发时不能补录（应走交接）")
	}
	if rerr := snap.CheckSoloFinish(r6.Id); rerr == nil {
		t.Fatal("第六棒无后续，不能补录")
	}

	withBackup := append([]model.RaceEvent{},
		model.RaceEvent{Id: 1, Type: model.EventBackupStart, RunnerId: runnerOf(runners, 1, 2).Id, Ms: 50 * 60 * 1000})
	if rerr := BuildSnapshot(teams, runners, withBackup, true).CheckSoloFinish(r1.Id); rerr != nil {
		t.Fatalf("备用带已发出后应允许补录：%v", rerr)
	}
}

func TestDeleteEventDependency(t *testing.T) {
	teams, runners := demoRoster()
	chain := evChain(teams, runners, 1, 2)
	// 撤销第一棒交接（启动了第二棒，且第二棒已有完成记录）→ 拒绝
	if rerr := CheckDeleteEvent(chain, chain[0].Id); rerr == nil {
		t.Fatal("被依赖的交接不应允许撤销")
	}
	// 撤销最后一条 → 允许
	if rerr := CheckDeleteEvent(chain, chain[len(chain)-1].Id); rerr != nil {
		t.Fatalf("链尾记录应允许撤销：%v", rerr)
	}
	// 不存在
	if rerr := CheckDeleteEvent(chain, 99999); rerr == nil {
		t.Fatal("不存在的记录应报错")
	}
}

func TestTeamScoreAndRanking(t *testing.T) {
	teams, runners := demoRoster()
	// 队1、队2 全员完成（队2 每棒慢 1 分钟）
	events := append(evChain(teams, runners, 1, 6), evChain(teams, runners, 2, 6)...)
	for i := range events {
		if events[i].RunnerId/100 == 2 {
			events[i].Ms += 60 * 1000
		}
	}
	snap := BuildSnapshot(teams, runners, events, true)

	q1 := snap.Teams[0].TeamScoreMs()
	if q1 == nil || *q1 != 6*1500000 {
		t.Fatalf("队1 总分应为 6×25:00：%v", q1)
	}
	ranked := RankByScore([]*TeamState{snap.Teams[0], snap.Teams[1]})
	if len(ranked) != 2 || *ranked[0].Rank != 1 || *ranked[1].Rank != 2 {
		t.Fatalf("不同总分应排出 1、2 名")
	}
	if ranked[0].Team.TeamId != 1 {
		t.Fatal("队1 总分更少应排第一")
	}

	// 同分并列：两队名次均为 1
	sameScore := append(evChain(teams, runners, 1, 6), evChain(teams, runners, 2, 6)...)
	snapTie := BuildSnapshot(teams, runners, sameScore, true)
	tied := RankByScore([]*TeamState{snapTie.Teams[0], snapTie.Teams[1]})
	if *tied[0].Rank != 1 || *tied[1].Rank != 1 {
		t.Fatalf("同分应并列第 1：%d %d", *tied[0].Rank, *tied[1].Rank)
	}
}

func TestOnCourseBoardOrdering(t *testing.T) {
	teams, runners := demoRoster()
	// 队1 第二棒 25:00 出发；队2 第一棒出发（时限 50:00）
	events := evChain(teams, runners, 1, 1)
	snap := BuildSnapshot(teams, runners, events, true)
	rows := snap.OnCourseRows(30 * 60 * 1000) // 30:00
	if len(rows) != 2 {
		t.Fatalf("应在途 2 人：%d", len(rows))
	}
	// 队1 第二棒 30:00-25:00 已跑 5min，剩 45min；队2 第一棒剩 20min → 更紧迫在前
	if rows[0].Leg.No != 1 || rows[0].TeamId != 2 {
		t.Fatalf("看板应按剩余时间升序：%+v", rows)
	}
	// 交接点：第一/三/五棒到 2 号点，第二/四/六棒到 1 号点
	if rows[0].Station != 2 || rows[1].Station != 1 {
		t.Fatalf("交接点方向错误：%d %d", rows[0].Station, rows[1].Station)
	}
}

func TestIndividualRankingExcludesWithdrawn(t *testing.T) {
	teams, runners := demoRoster()
	r2 := runnerOf(runners, 1, 2)
	r2m := runnerOf(runners, 2, 2)
	// 队1：第一棒交接启动第二棒，第二棒在途退赛
	// 队2：前两棒正常交接完成
	events := append(append(evChain(teams, runners, 1, 1), evChain(teams, runners, 2, 2)...),
		model.RaceEvent{Id: 98, Type: model.EventWithdraw, RunnerId: r2.Id, Ms: 3600000},
	)
	snap := BuildSnapshot(teams, runners, events, true)
	rows := snap.IndividualRows("M")
	for _, row := range rows {
		if row.Leg.RunnerId == r2.Id {
			t.Fatal("退赛选手不应出现在个人榜")
		}
	}
	// 队2 的第二棒应出现
	found := false
	for _, row := range rows {
		if row.Leg.RunnerId == r2m.Id {
			found = true
		}
	}
	if !found {
		t.Fatal("正常完赛选手应在个人榜")
	}
	// 女子榜：第六棒无人完赛 → 空
	frows := snap.IndividualRows("F")
	if len(frows) != 0 {
		t.Fatal("无人完赛时女子榜应为空")
	}
}
