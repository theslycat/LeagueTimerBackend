package service

import (
	"fmt"
	"sort"

	"wxcloudrun-golang/db/model"
)

// —— 赛制规则常量 ——
// 比赛采用 A、B 两个赛段（各 8 公里），每人完成一个赛段，全队六棒。
// 第一、三、五棒：A 段 1号点 → 2号点（男生）；
// 第二、四棒：B 段 2号点 → 1号点（男生）；第六棒：B 段（女生），终点在 1号点。
// 每名选手自实际出发时刻起 50 分钟时限；超时棒次团体计分按 50 分钟计。
const (
	// RuleTimeLimitMs 个人时限：50 分钟
	RuleTimeLimitMs int64 = 50 * 60 * 1000
)

// 规则校验错误类别
const (
	ruleKindInvalid  = iota // 状态不满足，直接拒绝
	ruleKindEarly           // 备用带早于规则时刻，警告后放行
	ruleKindConflict        // 撤销被后续记录依赖
)

// ruleError 规则校验错误：kind 决定错误码，earliestMs 仅 ruleKindEarly 有值
type ruleError struct {
	kind       int
	msg        string
	earliestMs int64
}

func (e *ruleError) Error() string { return e.msg }

func invalidRule(format string, args ...interface{}) *ruleError {
	return &ruleError{kind: ruleKindInvalid, msg: fmt.Sprintf(format, args...)}
}

func locateError(what string) *ruleError {
	return &ruleError{kind: ruleKindInvalid, msg: what + "不存在"}
}

/* ———————— 状态推导 ———————— */

// LegState 单棒状态（名单 + 事件流推导，不落库）
type LegState struct {
	RunnerId   int64
	No         int
	Name       string
	Gender     string
	Started    bool
	StartMs    int64
	Belt       string // normal|black
	EarlyStart bool   // 备用带早发（警告放行）
	Finished   bool
	FinishMs   int64
	FinishType string // exchange|solo
	Withdrew   bool
}

// OnCourse 在途：已出发且未完成、未退赛
func (l *LegState) OnCourse() bool { return l.Started && !l.Finished && !l.Withdrew }

// DurationMs 实际用时（未完赛为 nil）：完成时刻 - 出发时刻
func (l *LegState) DurationMs() *int64 {
	if !l.Finished {
		return nil
	}
	d := l.FinishMs - l.StartMs
	if d < 0 {
		d = 0
	}
	return &d
}

// ScoredMs 团体计分用时：未超过 50 分钟按实际，超时按 50 分钟计
func (l *LegState) ScoredMs() *int64 {
	if !l.Finished {
		return nil
	}
	d := *l.DurationMs()
	if d > RuleTimeLimitMs {
		d = RuleTimeLimitMs
	}
	return &d
}

// Overtime 是否超时完赛（实际用时超过 50 分钟）
func (l *LegState) Overtime() bool {
	if !l.Finished {
		return false
	}
	return *l.DurationMs() > RuleTimeLimitMs
}

// DeadlineMs 在途选手的个人时限：出发 + 50 分钟
func (l *LegState) DeadlineMs() *int64 {
	if !l.OnCourse() {
		return nil
	}
	d := l.StartMs + RuleTimeLimitMs
	return &d
}

// TeamState 队伍状态
type TeamState struct {
	TeamId   int64
	TeamName string
	Legs     []*LegState // 按棒次 no 升序
	legByNo  map[int]*LegState
}

// Leg 按棒次编号取棒（编号仅用于展示，规则判断一律按顺序位置）
func (t *TeamState) Leg(no int) *LegState { return t.legByNo[no] }

// FirstLeg 第一棒：名单顺序最靠前的一棒（不依赖编号是否为 1）
func (t *TeamState) FirstLeg() *LegState {
	if len(t.Legs) == 0 {
		return nil
	}
	return t.Legs[0]
}

// LastLeg 终点棒：名单顺序最后一棒（不依赖编号是否为 6）
func (t *TeamState) LastLeg() *LegState {
	if len(t.Legs) == 0 {
		return nil
	}
	return t.Legs[len(t.Legs)-1]
}

// IsFirst / IsLast 按位置判断首棒与终点棒
func (t *TeamState) IsFirst(leg *LegState) bool {
	return leg != nil && len(t.Legs) > 0 && t.Legs[0] == leg
}

func (t *TeamState) IsLast(leg *LegState) bool {
	return leg != nil && len(t.Legs) > 0 && t.Legs[len(t.Legs)-1] == leg
}

// legIndex 该棒在队伍顺序中的下标（不在名单中返回 -1）
func (t *TeamState) legIndex(leg *LegState) int {
	for i, l := range t.Legs {
		if l == leg {
			return i
		}
	}
	return -1
}

// NextLegOf / PrevLegOf 按名单顺序取相邻一棒（越界返回 nil）
func (t *TeamState) NextLegOf(leg *LegState) *LegState {
	i := t.legIndex(leg)
	if i < 0 || i+1 >= len(t.Legs) {
		return nil
	}
	return t.Legs[i+1]
}

func (t *TeamState) PrevLegOf(leg *LegState) *LegState {
	i := t.legIndex(leg)
	if i <= 0 {
		return nil
	}
	return t.Legs[i-1]
}

// HasWithdrawal 队内是否有选手退赛（该队不参与团体排名）
func (t *TeamState) HasWithdrawal() bool {
	for _, l := range t.Legs {
		if l.Withdrew {
			return true
		}
	}
	return false
}

// FinishedCount 已完成棒数
func (t *TeamState) FinishedCount() int {
	n := 0
	for _, l := range t.Legs {
		if l.Finished {
			n++
		}
	}
	return n
}

// AllFinished 全队六棒均完成本人完整赛段（断带但全员完成仍成立）
func (t *TeamState) AllFinished() bool {
	if len(t.Legs) == 0 {
		return false
	}
	for _, l := range t.Legs {
		if !l.Finished {
			return false
		}
	}
	return true
}

// Qualified 团体排名资格：全员完成且无退赛
func (t *TeamState) Qualified() bool { return t.AllFinished() && !t.HasWithdrawal() }

// TeamScoreMs 团体总成绩 = 各棒计分用时之和（未完赛为 nil）
func (t *TeamState) TeamScoreMs() *int64 {
	if !t.AllFinished() {
		return nil
	}
	var sum int64
	for _, l := range t.Legs {
		sum += *l.ScoredMs()
	}
	return &sum
}

// OnCourseLeg 当前在途的一棒（多棒在途取棒次最高者；无在途为 nil）
func (t *TeamState) OnCourseLeg() *LegState {
	var found *LegState
	for _, l := range t.Legs {
		if l.OnCourse() && (found == nil || l.No > found.No) {
			found = l
		}
	}
	return found
}

// CurrentLegNo 当前进度棒次（已出发的最高棒次；均未出发为 0）
func (t *TeamState) CurrentLegNo() int {
	no := 0
	for _, l := range t.Legs {
		if l.Started && l.No > no {
			no = l.No
		}
	}
	return no
}

// Snapshot 比赛推导快照：名单 + 事件流 → 每棒状态
type Snapshot struct {
	Teams        []*TeamState
	teamById     map[int64]*TeamState
	teamByRunner map[int64]*TeamState
	legByRunner  map[int64]*LegState
}

// BuildSnapshot 由名单与事件流构建状态快照（纯函数）。
// raceStarted：比赛是否已发令（发令即视为各队第一棒于 0ms 出发，常规带）。
func BuildSnapshot(teams []model.Team, runners []model.Runner, events []model.RaceEvent, raceStarted bool) *Snapshot {
	s := &Snapshot{
		Teams:        make([]*TeamState, 0, len(teams)),
		teamById:     make(map[int64]*TeamState, len(teams)),
		teamByRunner: make(map[int64]*TeamState, len(runners)),
		legByRunner:  make(map[int64]*LegState, len(runners)),
	}
	for i := range teams {
		ts := &TeamState{TeamId: teams[i].Id, TeamName: teams[i].Name, legByNo: map[int]*LegState{}}
		s.Teams = append(s.Teams, ts)
		s.teamById[teams[i].Id] = ts
	}
	for i := range runners {
		team := s.teamById[runners[i].TeamId]
		if team == nil {
			continue
		}
		leg := &LegState{
			RunnerId: runners[i].Id,
			No:       runners[i].No,
			Name:     runners[i].Name,
			Gender:   runners[i].Gender,
		}
		team.Legs = append(team.Legs, leg)
		team.legByNo[leg.No] = leg
		s.legByRunner[leg.RunnerId] = leg
		s.teamByRunner[leg.RunnerId] = team
	}
	for _, team := range s.Teams {
		sort.SliceStable(team.Legs, func(i, j int) bool { return team.Legs[i].No < team.Legs[j].No })
		// 发令：第一棒统一于 0ms 出发（第一棒按名单位置取，不依赖编号是否为 1）
		if raceStarted {
			if first := team.FirstLeg(); first != nil && !first.Started {
				first.Started = true
				first.StartMs = 0
				first.Belt = model.BeltNormal
			}
		}
	}
	// 事件按发生顺序重放
	ordered := make([]model.RaceEvent, len(events))
	copy(ordered, events)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Ms != ordered[j].Ms {
			return ordered[i].Ms < ordered[j].Ms
		}
		return ordered[i].Id < ordered[j].Id
	})
	for i := range ordered {
		s.apply(&ordered[i])
	}
	return s
}

// apply 重放单个事件（对异常数据防御性跳过）
func (s *Snapshot) apply(ev *model.RaceEvent) {
	leg := s.legByRunner[ev.RunnerId]
	if leg == nil {
		return
	}
	switch ev.Type {
	case model.EventExchange:
		// 已退赛的选手不会完成交接（防御性跳过）
		if leg.Withdrew {
			return
		}
		// 到达选手完成；同时为下一棒开始计时
		if !leg.Started {
			leg.Started = true
			leg.StartMs = ev.Ms
			leg.Belt = model.BeltNormal
		}
		leg.Finished = true
		leg.FinishMs = ev.Ms
		leg.FinishType = model.FinishExchange
		if ev.NextRunnerId != nil {
			if next := s.legByRunner[*ev.NextRunnerId]; next != nil && !next.Started {
				next.Started = true
				next.StartMs = ev.Ms
				next.Belt = model.BeltNormal
			}
		}
	case model.EventBackupStart:
		if !leg.Started {
			leg.Started = true
			leg.StartMs = ev.Ms
			leg.Belt = model.BeltBlack
			leg.EarlyStart = ev.Early
		}
	case model.EventSoloFinish:
		if !leg.Withdrew {
			leg.Finished = true
			leg.FinishMs = ev.Ms
			leg.FinishType = model.FinishSolo
		}
	case model.EventWithdraw:
		// 已完成赛段的选手退赛无意义（防御性跳过）
		if !leg.Finished {
			leg.Withdrew = true
		}
	}
}

// locate 定位选手所属队伍与棒次
func (s *Snapshot) locate(runnerId int64) (*TeamState, *LegState, *ruleError) {
	team := s.teamByRunner[runnerId]
	leg := s.legByRunner[runnerId]
	if team == nil || leg == nil {
		return nil, nil, locateError("选手")
	}
	return team, leg, nil
}

/* ———————— 事件前置校验 ———————— */

// CheckExchange 交接：到达选手须在途；下一棒须存在且未出发；第 6 棒为冲线。
// 返回的 *LegState 为自动派生的下一棒（冲线时为 nil）。
func (s *Snapshot) CheckExchange(runnerId int64) (*LegState, *ruleError) {
	team, leg, rerr := s.locate(runnerId)
	if rerr != nil {
		return nil, rerr
	}
	if !leg.Started {
		return nil, invalidRule("第%d棒 %s 尚未出发", leg.No, leg.Name)
	}
	if leg.Finished {
		return nil, invalidRule("第%d棒 %s 已完成，请勿重复记录", leg.No, leg.Name)
	}
	if leg.Withdrew {
		return nil, invalidRule("第%d棒 %s 已退赛，不能记录交接", leg.No, leg.Name)
	}
	// 终点棒（名单位置最后一棒）：冲线，无下一棒
	if team.IsLast(leg) {
		return nil, nil
	}
	next := team.NextLegOf(leg)
	if next == nil {
		return nil, invalidRule("第%d棒之后的选手不在名单中，无法交接", leg.No)
	}
	if next.Started {
		return nil, invalidRule("第%d棒 %s 已（用备用带）出发，请改用「补录到达」", next.No, next.Name)
	}
	return next, nil
}

// CheckBackupStart 备用带出发：本棒未出发、上一棒在途；
// 最早合法时刻 = 上一棒实际出发 + 50 分钟（早于此返回 ruleKindEarly）。
func (s *Snapshot) CheckBackupStart(runnerId int64, ms int64) (int64, *ruleError) {
	_, leg, rerr := s.locate(runnerId)
	if rerr != nil {
		return 0, rerr
	}
	if leg.Started {
		return 0, invalidRule("第%d棒 %s 已出发", leg.No, leg.Name)
	}
	team := s.teamByRunner[runnerId]
	if team.IsFirst(leg) {
		return 0, invalidRule("第一棒随发令统一出发，无需备用带")
	}
	prev := team.PrevLegOf(leg)
	if prev == nil {
		return 0, invalidRule("第%d棒之前的选手不在名单中", leg.No)
	}
	if !prev.Started {
		return 0, invalidRule("上一棒尚未出发")
	}
	if prev.Finished {
		return 0, invalidRule("上一棒已完成交接，无需备用带")
	}
	earliest := prev.StartMs + RuleTimeLimitMs
	if ms < earliest {
		return earliest, &ruleError{
			kind: ruleKindEarly,
			msg:  fmt.Sprintf("未到最早出发时刻（上一棒出发 + 50 分钟）"), earliestMs: earliest,
		}
	}
	return 0, nil
}

// CheckSoloFinish 补录到达：本棒在途且下一棒已用备用带出发。
func (s *Snapshot) CheckSoloFinish(runnerId int64) *ruleError {
	team, leg, rerr := s.locate(runnerId)
	if rerr != nil {
		return rerr
	}
	if !leg.Started {
		return invalidRule("第%d棒 %s 尚未出发", leg.No, leg.Name)
	}
	if leg.Finished {
		return invalidRule("第%d棒 %s 已完成，请勿重复记录", leg.No, leg.Name)
	}
	if leg.Withdrew {
		return invalidRule("第%d棒 %s 已退赛，不能补录成绩", leg.No, leg.Name)
	}
	if team.IsLast(leg) {
		return invalidRule("第%d棒为终点棒，请使用「冲线」记录", leg.No)
	}
	next := team.NextLegOf(leg)
	if next == nil {
		return invalidRule("第%d棒之后的选手不在名单中", leg.No)
	}
	if !next.Started {
		return invalidRule("下一棒尚未出发：正常到达请使用「交接」，补录仅用于备用带已发出的情况")
	}
	return nil
}

// CheckWithdraw 退赛：本棒在途。
func (s *Snapshot) CheckWithdraw(runnerId int64) *ruleError {
	_, leg, rerr := s.locate(runnerId)
	if rerr != nil {
		return rerr
	}
	if !leg.Started {
		return invalidRule("第%d棒 %s 尚未出发", leg.No, leg.Name)
	}
	if leg.Finished {
		return invalidRule("第%d棒 %s 已完成赛段，无需退赛", leg.No, leg.Name)
	}
	if leg.Withdrew {
		return invalidRule("第%d棒 %s 已退赛", leg.No, leg.Name)
	}
	return nil
}

/* ———————— 撤销依赖检查 ———————— */

// CheckDeleteEvent 撤销事件：交接 / 备用带出发若有后续事件依赖则不可删除，
// 须先撤销更晚的记录。补录到达与退赛不阻塞任何记录，可随时撤销。
func CheckDeleteEvent(events []model.RaceEvent, id int64) *ruleError {
	var target *model.RaceEvent
	for i := range events {
		if events[i].Id == id {
			target = &events[i]
			break
		}
	}
	if target == nil {
		return locateError("记录")
	}
	switch target.Type {
	case model.EventExchange:
		if target.NextRunnerId != nil {
			for i := range events {
				other := &events[i]
				if other.Id != target.Id && other.RunnerId == *target.NextRunnerId {
					return &ruleError{kind: ruleKindConflict,
						msg: "该交接启动的下一棒已有后续记录，请先撤销其后更晚的记录"}
				}
			}
		}
	case model.EventBackupStart:
		for i := range events {
			other := &events[i]
			if other.Id != target.Id && other.RunnerId == target.RunnerId {
				return &ruleError{kind: ruleKindConflict,
					msg: "该选手已有后续记录，请先撤销其后更晚的记录"}
			}
		}
	}
	return nil
}

/* ———————— 排名与看板推导 ———————— */

// OnCourseRow 在途选手看板行
type OnCourseRow struct {
	Leg         *LegState
	TeamId      int64
	TeamName    string
	ElapsedMs   int64 // 个人已跑时间
	RemainingMs int64 // 距个人时限剩余
	Station     int   // 将到达的交接点：奇数棒 → 2号点，偶数棒 → 1号点
}

// OnCourseRows 在途选手列表（剩余时间升序，最紧迫的在前）
func (s *Snapshot) OnCourseRows(elapsedMs int64) []*OnCourseRow {
	rows := make([]*OnCourseRow, 0)
	for _, team := range s.Teams {
		for _, leg := range team.Legs {
			if !leg.OnCourse() {
				continue
			}
			remaining := int64(0)
			if deadline := leg.DeadlineMs(); deadline != nil {
				remaining = *deadline - elapsedMs
			}
			station := 1
			if leg.No%2 == 1 {
				station = 2
			}
			rows = append(rows, &OnCourseRow{
				Leg:         leg,
				TeamId:      team.TeamId,
				TeamName:    team.TeamName,
				ElapsedMs:   elapsedMs - leg.StartMs,
				RemainingMs: remaining,
				Station:     station,
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].RemainingMs < rows[j].RemainingMs })
	return rows
}

// IndividualRow 个人实际用时榜行
type IndividualRow struct {
	Leg      *LegState
	TeamId   int64
	TeamName string
}

// IndividualRows 个人榜（按实际用时升序，仅完赛者；退赛者不参与个人奖评选）
func (s *Snapshot) IndividualRows(gender string) []*IndividualRow {
	rows := make([]*IndividualRow, 0)
	for _, team := range s.Teams {
		for _, leg := range team.Legs {
			if !leg.Finished || leg.Gender != gender {
				continue
			}
			rows = append(rows, &IndividualRow{Leg: leg, TeamId: team.TeamId, TeamName: team.TeamName})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return *rows[i].Leg.DurationMs() < *rows[j].Leg.DurationMs()
	})
	return rows
}

// RankedTeam 团体榜队伍（排序与名次）
type RankedTeam struct {
	Team *TeamState
	Rank *int // 仅「全员完成且无退赛」的队伍有名次
}

// RankByScore 对有资格队伍按团体总分排名（同分并列，后续名次顺延）
func RankByScore(qualified []*TeamState) []*RankedTeam {
	sort.SliceStable(qualified, func(i, j int) bool {
		return *qualified[i].TeamScoreMs() < *qualified[j].TeamScoreMs()
	})
	result := make([]*RankedTeam, 0, len(qualified))
	for i := range qualified {
		rank := 1
		if i > 0 && *qualified[i].TeamScoreMs() == *qualified[i-1].TeamScoreMs() {
			// 与前一名同分：并列（沿用前一名的名次）
			rank = *result[len(result)-1].Rank
		} else if i > 0 {
			rank = i + 1
		}
		r := rank
		result = append(result, &RankedTeam{Team: qualified[i], Rank: &r})
	}
	return result
}
