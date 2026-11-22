package service

import (
	"net/http"

	"wxcloudrun-golang/db/dao"
)

// runnerVO 选手对外视图
type runnerVO struct {
	Id   int64  `json:"id"`
	No   int    `json:"no"`
	Name string `json:"name"`
}

// teamVO 队伍对外视图
type teamVO struct {
	Id      int64       `json:"id"`
	Name    string      `json:"name"`
	Runners []*runnerVO `json:"runners"`
}

type participantsData struct {
	Teams []*teamVO `json:"teams"`
}

// ParticipantsHandler GET /api/participants 队伍与选手名单（无需登录）。
// 队伍按 id、选手按号码 no 升序。
func ParticipantsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodError(w, r.Method)
		return
	}
	teams, err := dao.TeamImp.ListTeams()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	// ListRunners 已按 team_id、no 排序，直接按队伍分组即可
	runners, err := dao.RunnerImp.ListRunners()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	runnersByTeam := make(map[int64][]*runnerVO, len(teams))
	for i := range runners {
		runner := &runners[i]
		runnersByTeam[runner.TeamId] = append(runnersByTeam[runner.TeamId], &runnerVO{
			Id:   runner.Id,
			No:   runner.No,
			Name: runner.Name,
		})
	}
	list := make([]*teamVO, 0, len(teams))
	for i := range teams {
		team := &teams[i]
		members := runnersByTeam[team.Id]
		if members == nil {
			members = []*runnerVO{}
		}
		list = append(list, &teamVO{Id: team.Id, Name: team.Name, Runners: members})
	}
	sendOk(w, &participantsData{Teams: list})
}
