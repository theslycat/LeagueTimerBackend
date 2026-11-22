package service

import (
	"errors"
	"net/http"
	"time"

	"wxcloudrun-golang/db/dao"
	"wxcloudrun-golang/db/model"
)

// raceVO 比赛状态对外视图
type raceVO struct {
	Status    string `json:"status"`
	StartedAt *int64 `json:"startedAt"`
	ElapsedMs int64  `json:"elapsedMs"`
	UpdatedAt int64  `json:"updatedAt"`
}

type raceData struct {
	Race *raceVO `json:"race"`
}

// raceElapsedMs elapsed = base_ms + (running ? now - started_at : 0)
func raceElapsedMs(race *model.Race) int64 {
	if race.Status == model.StatusRunning && race.StartedAt != nil {
		elapsed := race.BaseMs + time.Since(*race.StartedAt).Milliseconds()
		if elapsed < 0 {
			elapsed = 0
		}
		return elapsed
	}
	return race.BaseMs
}

func buildRaceVO(race *model.Race) *raceVO {
	vo := &raceVO{
		Status:    race.Status,
		ElapsedMs: raceElapsedMs(race),
		UpdatedAt: unixMilli(race.UpdatedAt),
	}
	if race.StartedAt != nil {
		started := unixMilli(*race.StartedAt)
		vo.StartedAt = &started
	}
	return vo
}

// RaceHandler GET /api/race 查询比赛状态（无需登录）
func RaceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodError(w, r.Method)
		return
	}
	race, err := dao.RaceImp.GetRace()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	sendOk(w, &raceData{Race: buildRaceVO(race)})
}

// raceTransition 比赛状态流转接口的统一封装（仅主裁判、仅 POST）：
// 按钮合法但状态不满足时返回 code -1 与具体文案
func raceTransition(action func(now time.Time) (*model.Race, error), confirm string, confirmTitle string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodError(w, r.Method)
			return
		}
		authMiddleware(model.RoleChief, func(w http.ResponseWriter, r *http.Request, claims *Claims) {
			race, err := action(time.Now())
			if err != nil {
				fail(w, translateRaceError(err))
				return
			}
			sendOk(w, &raceData{Race: buildRaceVO(race)})
		})(w, r)
	}
}

// RaceStartHandler POST /api/race/start 发令开始比赛（仅主裁判，8:00 统一发令）
func RaceStartHandler(w http.ResponseWriter, r *http.Request) {
	raceTransition(dao.RaceImp.StartRace, "", "")(w, r)
}

// RaceTerminateHandler POST /api/race/terminate 终止比赛（仅主裁判）。
// 按赛制无「暂停」：发令后计时不可中止，误发令用终止 + 重置补救。
func RaceTerminateHandler(w http.ResponseWriter, r *http.Request) {
	raceTransition(dao.RaceImp.TerminateRace, "", "")(w, r)
}

// RaceResetHandler POST /api/race/reset 重置比赛并清空事件（仅主裁判）
func RaceResetHandler(w http.ResponseWriter, r *http.Request) {
	raceTransition(dao.RaceImp.ResetRace, "", "")(w, r)
}

// RacePauseHandler 暂停接口已按赛制移除：发令后比赛计时不可中止
func RacePauseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodError(w, r.Method)
		return
	}
	sendError(w, CodeGeneric, "按赛制比赛不支持暂停：误发令请用「终止 + 重置」补救")
}

// isRaceStateError 是否属于状态机层面可对外展示的错误
func isRaceStateError(err error) bool {
	return errors.Is(err, dao.ErrRaceAlreadyRunning) ||
		errors.Is(err, dao.ErrRaceNotStarted) ||
		errors.Is(err, dao.ErrRaceFinished)
}

func translateRaceError(err error) error {
	if isRaceStateError(err) {
		return genericError(err.Error())
	}
	return internalError(err)
}
