package service

import (
	"net/http"

	"wxcloudrun-golang/db/dao"
	"wxcloudrun-golang/db/model"
)

// rankingRowVO 排名行：已记录 rank=1..k；未记录 rank/passMs/gapMs 为 null
type rankingRowVO struct {
	Rank     *int   `json:"rank"`
	RunnerId int64  `json:"runnerId"`
	No       int    `json:"no"`
	Name     string `json:"name"`
	TeamId   int64  `json:"teamId"`
	TeamName string `json:"teamName"`
	PassMs   *int64 `json:"passMs"`
	GapMs    *int64 `json:"gapMs"`
	Status   string `json:"status"`
}

// rankingTeamVO 队伍完成度统计
type rankingTeamVO struct {
	Id       int64  `json:"id"`
	Name     string `json:"name"`
	Recorded int    `json:"recorded"`
	Total    int    `json:"total"`
}

type rankingData struct {
	Status    string           `json:"status"`
	ElapsedMs int64            `json:"elapsedMs"`
	Rows      []*rankingRowVO  `json:"rows"`
	Teams     []*rankingTeamVO `json:"teams"`
}

// RankingHandler GET /api/ranking 实时排名（无需登录，前端 1 秒轮询）。
// 实现为三次简单查询 + 内存组装，无锁、无 N+1，保持轻量。
// 比赛未运行时也照常返回已记录成绩。
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
	// ListRunners 已按 team_id、no 排序
	runners, err := dao.RunnerImp.ListRunners()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	// ListRecords 已按 pass_ms 升序
	records, err := dao.RecordImp.ListRecords()
	if err != nil {
		fail(w, internalError(err))
		return
	}

	teamName := make(map[int64]string, len(teams))
	for i := range teams {
		teamName[teams[i].Id] = teams[i].Name
	}
	runnerById := make(map[int64]*model.Runner, len(runners))
	for i := range runners {
		runnerById[runners[i].Id] = &runners[i]
	}
	recordByRunner := make(map[int64]bool, len(records))
	for i := range records {
		recordByRunner[records[i].RunnerId] = true
	}

	// 已记录的按 pass_ms 升序：rank=1..k，gapMs=与第一名之差
	rows := make([]*rankingRowVO, 0, len(runners))
	var firstPassMs int64
	for i := range records {
		record := &records[i]
		runner, ok := runnerById[record.RunnerId]
		if !ok {
			continue
		}
		if len(rows) == 0 {
			firstPassMs = record.PassMs
		}
		rank := len(rows) + 1
		passMs := record.PassMs
		gapMs := int64(0)
		if record.PassMs > firstPassMs {
			gapMs = record.PassMs - firstPassMs
		}
		rows = append(rows, &rankingRowVO{
			Rank:     &rank,
			RunnerId: runner.Id,
			No:       runner.No,
			Name:     runner.Name,
			TeamId:   runner.TeamId,
			TeamName: teamName[runner.TeamId],
			PassMs:   &passMs,
			GapMs:    &gapMs,
			Status:   "recorded",
		})
	}
	// 未记录的跟随其后，按队伍 id、号码 no 排序（ListRunners 顺序即满足）
	for i := range runners {
		runner := &runners[i]
		if recordByRunner[runner.Id] {
			continue
		}
		rows = append(rows, &rankingRowVO{
			RunnerId: runner.Id,
			No:       runner.No,
			Name:     runner.Name,
			TeamId:   runner.TeamId,
			TeamName: teamName[runner.TeamId],
			Status:   "running",
		})
	}

	// 队伍完成度：recorded/total
	teamRecorded := make(map[int64]int, len(teams))
	teamTotal := make(map[int64]int, len(teams))
	for i := range runners {
		teamTotal[runners[i].TeamId]++
	}
	for i := range records {
		if runner, ok := runnerById[records[i].RunnerId]; ok {
			teamRecorded[runner.TeamId]++
		}
	}
	teamVOs := make([]*rankingTeamVO, 0, len(teams))
	for i := range teams {
		team := &teams[i]
		teamVOs = append(teamVOs, &rankingTeamVO{
			Id:       team.Id,
			Name:     team.Name,
			Recorded: teamRecorded[team.Id],
			Total:    teamTotal[team.Id],
		})
	}

	sendOk(w, &rankingData{
		Status:    race.Status,
		ElapsedMs: raceElapsedMs(race),
		Rows:      rows,
		Teams:     teamVOs,
	})
}
