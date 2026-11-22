package service

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"wxcloudrun-golang/db/dao"
	"wxcloudrun-golang/db/model"
)

// participantsData GET /api/participants 响应
type participantsData struct {
	Teams []*teamVO `json:"teams"`
}

// teamVO 队伍对外视图
type teamVO struct {
	Id      int64       `json:"id"`
	Name    string      `json:"name"`
	Runners []*runnerVO `json:"runners"`
}

// runnerVO 选手对外视图
type runnerVO struct {
	Id     int64  `json:"id"`
	No     int    `json:"no"`
	Name   string `json:"name"`
	Gender string `json:"gender"`
}

// ParticipantsHandler GET /api/participants 队伍与选手名单（无需登录）；
// PUT /api/participants 整体替换名单（仅主裁判，仅未开赛）。
func ParticipantsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
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
		sendOk(w, buildParticipantsData(teams, runners))
	case http.MethodPut:
		authMiddleware(model.RoleChief, putParticipants)(w, r)
	default:
		methodError(w, r.Method)
	}
}

func buildParticipantsData(teams []model.Team, runners []model.Runner) *participantsData {
	runnersByTeam := make(map[int64][]*runnerVO, len(teams))
	for i := range runners {
		runner := &runners[i]
		runnersByTeam[runner.TeamId] = append(runnersByTeam[runner.TeamId], &runnerVO{
			Id:     runner.Id,
			No:     runner.No,
			Name:   runner.Name,
			Gender: runner.Gender,
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
	return &participantsData{Teams: list}
}

/* ———————— 名单导入（仅主裁判，仅未开赛） ———————— */

// rosterPutData PUT /api/participants 请求体：整份名单
type rosterPutData struct {
	Teams []rosterTeamInput `json:"teams"`
}

// rosterTeamInput 输入队伍
type rosterTeamInput struct {
	Name    string              `json:"name"`
	Runners []rosterRunnerInput `json:"runners"`
}

// rosterRunnerInput 输入选手
type rosterRunnerInput struct {
	No     int    `json:"no"`
	Name   string `json:"name"`
	Gender string `json:"gender"`
}

// putParticipants 校验并写入名单：
// 每队恰 6 棒（1-6 各一棒）；第一至五棒男生、第六棒女生；
// 队伍名 1-32 字且不重复；选手名 1-32 字。比赛开始后拒绝。
func putParticipants(w http.ResponseWriter, r *http.Request, claims *Claims) {
	var req rosterPutData
	if err := decodeBody(r, &req); err != nil {
		fail(w, genericError("请求参数格式错误"))
		return
	}
	if len(req.Teams) == 0 {
		fail(w, newBizError(CodeRosterInvalid, "名单不能为空"))
		return
	}

	input := make([]dao.RosterTeamInput, 0, len(req.Teams))
	teamNames := make(map[string]bool, len(req.Teams))
	for ti := range req.Teams {
		st := &req.Teams[ti]
		name := strings.TrimSpace(st.Name)
		if name == "" || utf8.RuneCountInString(name) > 32 {
			fail(w, newBizError(CodeRosterInvalid, "队伍名不能为空且不超过 32 个字符"))
			return
		}
		if teamNames[name] {
			fail(w, newBizError(CodeRosterInvalid, "队伍名重复："+name))
			return
		}
		teamNames[name] = true
		if len(st.Runners) != model.LegsPerTeam {
			fail(w, newBizError(CodeRosterInvalid,
				"每队须恰好 6 名选手（"+name+"）"))
			return
		}
		runners := make([]model.Runner, 0, model.LegsPerTeam)
		seenNo := make(map[int]bool, model.LegsPerTeam)
		for ri := range st.Runners {
			sr := &st.Runners[ri]
			runnerName := strings.TrimSpace(sr.Name)
			if runnerName == "" || utf8.RuneCountInString(runnerName) > 32 {
				fail(w, newBizError(CodeRosterInvalid, "选手名不能为空且不超过 32 个字符"))
				return
			}
			if sr.No < 1 || sr.No > model.LegsPerTeam {
				fail(w, newBizError(CodeRosterInvalid, "棒次须为 1-6"))
				return
			}
			if seenNo[sr.No] {
				fail(w, newBizError(CodeRosterInvalid, "棒次重复（"+name+"）"))
				return
			}
			seenNo[sr.No] = true
			// 赛制：第一至五棒男生，第六棒女生
			expect := "M"
			if sr.No == model.LegsPerTeam {
				expect = "F"
			}
			if sr.Gender != "M" && sr.Gender != "F" {
				fail(w, newBizError(CodeRosterInvalid,
					"第"+itoa(sr.No)+"棒 "+runnerName+" 的性别无效（M/F）"))
				return
			}
			if sr.Gender != expect {
				fail(w, newBizError(CodeRosterInvalid,
					"按赛制第"+itoa(sr.No)+"棒应为"+genderLabel(expect)+"选手"))
				return
			}
			runners = append(runners, model.Runner{No: sr.No, Name: runnerName, Gender: sr.Gender})
		}
		input = append(input, dao.RosterTeamInput{Name: name, Runners: runners})
	}

	// 名单锁定（比赛开始后拒绝替换）
	if err := dao.ParticipantImp.ReplaceRoster(time.Now(), input); err != nil {
		fail(w, translateRosterError(err))
		return
	}
	// 读回并返回
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
	sendOk(w, buildParticipantsData(teams, runners))
}

func translateRosterError(err error) error {
	if errors.Is(err, dao.ErrRaceNotIdle) {
		return newBizError(CodeRosterLocked, err.Error())
	}
	if dao.IsDuplicateEntryError(err) {
		return newBizError(CodeRosterInvalid, "队伍名重复")
	}
	return internalError(err)
}

func genderLabel(g string) string {
	if g == "F" {
		return "女"
	}
	return "男"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
