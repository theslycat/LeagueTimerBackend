package service

import (
	"net/http"
	"sort"

	"wxcloudrun-golang/db/dao"
	"wxcloudrun-golang/db/model"
)

/* ———————— 对外视图 ———————— */

// legVO 单棒状态视图
type legVO struct {
	RunnerId   int64  `json:"runnerId"`
	No         int    `json:"no"`
	Name       string `json:"name"`
	Gender     string `json:"gender"`
	Status     string `json:"status"` // waiting|oncourse|finished|withdrew
	Started    bool   `json:"started"`
	StartMs    *int64 `json:"startMs"`
	Belt       string `json:"belt"` // normal|black
	EarlyStart bool   `json:"earlyStart"`
	FinishMs   *int64 `json:"finishMs"`
	FinishType string `json:"finishType"` // exchange|solo
	DurationMs *int64 `json:"durationMs"` // 实际用时
	ScoredMs   *int64 `json:"scoredMs"`   // 团体计分用时（超时按 50 分钟计）
	Overtime   bool   `json:"overtime"`
	DeadlineMs *int64 `json:"deadlineMs"` // 在途选手的个人时限
}

// teamBoardVO 团体榜视图
type teamBoardVO struct {
	Id            int64    `json:"id"`
	Name          string   `json:"name"`
	Status        string   `json:"status"` // finished|running|waiting|disqualified
	Qualified     bool     `json:"qualified"`
	Rank          *int     `json:"rank"`    // 仅资格队伍有名次
	ScoreMs       *int64   `json:"scoreMs"` // 团体总成绩
	ActiveLegNo   int      `json:"activeLegNo"`
	OnCourseLegNo int      `json:"onCourseLegNo"`
	FinishedLegs  int      `json:"finishedLegs"`
	TotalLegs     int      `json:"totalLegs"`
	Legs          []*legVO `json:"legs"`
}

// individualVO 个人实际用时榜视图
type individualVO struct {
	Rank       *int   `json:"rank"`
	RunnerId   int64  `json:"runnerId"`
	No         int    `json:"no"`
	Name       string `json:"name"`
	TeamId     int64  `json:"teamId"`
	TeamName   string `json:"teamName"`
	DurationMs int64  `json:"durationMs"`
	Overtime   bool   `json:"overtime"`
}

// onCourseVO 在途选手看板视图
type onCourseVO struct {
	RunnerId    int64  `json:"runnerId"`
	No          int    `json:"no"`
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	TeamId      int64  `json:"teamId"`
	TeamName    string `json:"teamName"`
	StartMs     int64  `json:"startMs"`
	ElapsedMs   int64  `json:"elapsedMs"`
	RemainingMs int64  `json:"remainingMs"`
	Overdue     bool   `json:"overdue"`
	Station     int    `json:"station"` // 将到达的交接点：奇数棒 2 号点，偶数棒 1 号点
}

// eventVO 比赛事件视图（流水 / 撤销面板）
type eventVO struct {
	Id             int64  `json:"id"`
	Type           string `json:"type"`
	RunnerId       int64  `json:"runnerId"`
	RunnerNo       int    `json:"runnerNo"`
	RunnerName     string `json:"runnerName"`
	NextRunnerId   *int64 `json:"nextRunnerId"`
	NextRunnerNo   *int   `json:"nextRunnerNo"`
	NextRunnerName string `json:"nextRunnerName"`
	TeamId         int64  `json:"teamId"`
	TeamName       string `json:"teamName"`
	Ms             int64  `json:"ms"`
	Early          bool   `json:"early"`
	RecordedBy     string `json:"recordedBy"`
	CreatedAt      int64  `json:"createdAt"`
}

// rankingData GET /api/ranking 响应（前端 1 秒轮询）
type rankingData struct {
	Status      string          `json:"status"`
	StartedAtMs *int64          `json:"startedAtMs"` // 发令墙钟（unix ms）
	ElapsedMs   int64           `json:"elapsedMs"`
	Teams       []*teamBoardVO  `json:"teams"`
	IndividualM []*individualVO `json:"individualM"`
	IndividualF []*individualVO `json:"individualF"`
	OnCourse    []*onCourseVO   `json:"onCourse"`
	Events      []*eventVO      `json:"events"`
}

/* ———————— 构建 ———————— */

// RankingHandler GET /api/ranking 实时榜单（无需登录，前端 1 秒轮询）。
// 返回团体/个人榜、在途选手 50 分钟看板与事件流水，全部由名单 + 事件流推导。
func RankingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodError(w, r.Method)
		return
	}
	race, err := dao.RaceImp.GetRace()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	teams, err := dao.TeamImp.ListTeams()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	runners, err := dao.RunnerImp.ListRunners()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	events, err := dao.EventImp.ListEvents()
	if err != nil {
		fail(w, internalError(err))
		return
	}

	raceStarted := race.Status != model.StatusIdle
	snap := BuildSnapshot(teams, runners, events, raceStarted)
	elapsed := raceElapsedMs(race)

	sendOk(w, buildRankingData(race, snap, elapsed, teams, runners, events))
}

func buildRankingData(race *model.Race, snap *Snapshot, elapsed int64,
	teams []model.Team, runners []model.Runner, events []model.RaceEvent) *rankingData {

	data := &rankingData{
		Status:      race.Status,
		ElapsedMs:   elapsed,
		Teams:       make([]*teamBoardVO, 0, len(snap.Teams)),
		IndividualM: buildIndividual(snap, "M"),
		IndividualF: buildIndividual(snap, "F"),
		OnCourse:    buildOnCourse(snap, elapsed),
		Events:      make([]*eventVO, 0, len(events)),
	}
	if race.GunAt != nil {
		gun := unixMilli(*race.GunAt)
		data.StartedAtMs = &gun
	}

	// 团体榜：资格队伍按总分排名 → 进行中按进度 → 已除名殿后
	rankByTeam := map[int64]*int{}
	qualified := make([]*TeamState, 0)
	for _, ts := range snap.Teams {
		if ts.Qualified() {
			qualified = append(qualified, ts)
		}
	}
	for _, rt := range RankByScore(qualified) {
		rankByTeam[rt.Team.TeamId] = rt.Rank
	}
	ordered := orderTeamsForBoard(snap, elapsed)

	for _, ts := range ordered {
		vo := &teamBoardVO{
			Id:           ts.TeamId,
			Name:         ts.TeamName,
			Qualified:    ts.Qualified(),
			Rank:         rankByTeam[ts.TeamId],
			ScoreMs:      ts.TeamScoreMs(),
			ActiveLegNo:  ts.CurrentLegNo(),
			FinishedLegs: ts.FinishedCount(),
			TotalLegs:    len(ts.Legs),
			Legs:         make([]*legVO, 0, len(ts.Legs)),
		}
		if oc := ts.OnCourseLeg(); oc != nil {
			vo.OnCourseLegNo = oc.No
		}
		switch {
		case ts.HasWithdrawal():
			vo.Status = "disqualified"
		case ts.AllFinished():
			vo.Status = "finished"
		case vo.ActiveLegNo > 0:
			vo.Status = "running"
		default:
			vo.Status = "waiting"
		}
		for _, leg := range ts.Legs {
			vo.Legs = append(vo.Legs, buildLegVO(leg))
		}
		data.Teams = append(data.Teams, vo)
	}

	for i := range events {
		data.Events = append(data.Events, buildEventVO(&events[i], teams, runners))
	}
	return data
}

// orderTeamsForBoard 榜单顺序：资格队伍（已按总分排名）→
// 进行中 / 未出发（按进度）→ 已退赛除名队伍（按进度）。
func orderTeamsForBoard(snap *Snapshot, elapsed int64) []*TeamState {
	type entry struct {
		group int
		key   int64
		team  *TeamState
	}
	entries := make([]entry, 0, len(snap.Teams))
	for _, ts := range snap.Teams {
		e := entry{team: ts, key: teamProgressKey(ts, elapsed)}
		switch {
		case ts.Qualified():
			e.group = 0
		case ts.HasWithdrawal():
			e.group = 2
		default:
			e.group = 1
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].group != entries[j].group {
			return entries[i].group < entries[j].group
		}
		return entries[i].key > entries[j].key
	})
	result := make([]*TeamState, 0, len(entries))
	for i := range entries {
		result = append(result, entries[i].team)
	}
	return result
}

// teamProgressKey 进度键：完成棒数为主，在途棒已跑时间为辅（越大越领先）
func teamProgressKey(ts *TeamState, elapsed int64) int64 {
	key := int64(ts.FinishedCount()) * 10_000_000
	if oc := ts.OnCourseLeg(); oc != nil {
		ran := elapsed - oc.StartMs
		if ran > 0 {
			key += ran
		}
	}
	return key
}

func buildLegVO(leg *LegState) *legVO {
	vo := &legVO{
		RunnerId:   leg.RunnerId,
		No:         leg.No,
		Name:       leg.Name,
		Gender:     leg.Gender,
		Started:    leg.Started,
		Belt:       leg.Belt,
		EarlyStart: leg.EarlyStart,
		DeadlineMs: leg.DeadlineMs(),
		DurationMs: leg.DurationMs(),
		ScoredMs:   leg.ScoredMs(),
		Overtime:   leg.Overtime(),
	}
	if leg.Started {
		start := leg.StartMs
		vo.StartMs = &start
	}
	if leg.Finished {
		finish := leg.FinishMs
		vo.FinishMs = &finish
		vo.FinishType = leg.FinishType
		vo.Status = "finished"
	} else if leg.Withdrew {
		vo.Status = "withdrew"
	} else if leg.Started {
		vo.Status = "oncourse"
	} else {
		vo.Status = "waiting"
	}
	return vo
}

func buildOnCourse(snap *Snapshot, elapsed int64) []*onCourseVO {
	rows := snap.OnCourseRows(elapsed)
	result := make([]*onCourseVO, 0, len(rows))
	for _, row := range rows {
		result = append(result, &onCourseVO{
			RunnerId:    row.Leg.RunnerId,
			No:          row.Leg.No,
			Name:        row.Leg.Name,
			Gender:      row.Leg.Gender,
			TeamId:      row.TeamId,
			TeamName:    row.TeamName,
			StartMs:     row.Leg.StartMs,
			ElapsedMs:   row.ElapsedMs,
			RemainingMs: row.RemainingMs,
			Overdue:     row.RemainingMs < 0,
			Station:     row.Station,
		})
	}
	return result
}

func buildIndividual(snap *Snapshot, gender string) []*individualVO {
	rows := snap.IndividualRows(gender)
	result := make([]*individualVO, 0, len(rows))
	for i, row := range rows {
		vo := &individualVO{
			RunnerId:   row.Leg.RunnerId,
			No:         row.Leg.No,
			Name:       row.Leg.Name,
			TeamId:     row.TeamId,
			TeamName:   row.TeamName,
			DurationMs: *row.Leg.DurationMs(),
			Overtime:   row.Leg.Overtime(),
		}
		// 同分并列名次，后续顺延（1,1,3）
		rank := i + 1
		if i > 0 && rows[i].Leg.DurationMs() == rows[i-1].Leg.DurationMs() {
			rank = *result[i-1].Rank
		}
		vo.Rank = &rank
		result = append(result, vo)
	}
	return result
}

// buildEventVO 事件视图：补全选手 / 下一棒 / 队伍名称
func buildEventVO(ev *model.RaceEvent, teams []model.Team, runners []model.Runner) *eventVO {
	runnerById := make(map[int64]*model.Runner, len(runners))
	for i := range runners {
		runnerById[runners[i].Id] = &runners[i]
	}
	teamById := make(map[int64]*model.Team, len(teams))
	for i := range teams {
		teamById[teams[i].Id] = &teams[i]
	}
	vo := &eventVO{
		Id:           ev.Id,
		Type:         ev.Type,
		RunnerId:     ev.RunnerId,
		NextRunnerId: ev.NextRunnerId,
		Ms:           ev.Ms,
		Early:        ev.Early,
		RecordedBy:   ev.RecordedBy,
		CreatedAt:    unixMilli(ev.CreatedAt),
	}
	if runner := runnerById[ev.RunnerId]; runner != nil {
		vo.RunnerNo = runner.No
		vo.RunnerName = runner.Name
		vo.TeamId = runner.TeamId
		if team := teamById[runner.TeamId]; team != nil {
			vo.TeamName = team.Name
		}
	}
	if ev.NextRunnerId != nil {
		if next := runnerById[*ev.NextRunnerId]; next != nil {
			nextNo := next.No
			vo.NextRunnerNo = &nextNo
			vo.NextRunnerName = next.Name
		}
	}
	return vo
}
