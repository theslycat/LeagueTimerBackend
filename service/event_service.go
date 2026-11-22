package service

import (
	"errors"
	"net/http"
	"strconv"

	"gorm.io/gorm"

	"wxcloudrun-golang/db/dao"
	"wxcloudrun-golang/db/model"
)

// eventCreateData POST /api/events 成功响应 data
type eventCreateData struct {
	Event *eventVO `json:"event"`
}

// EventsHandler POST /api/events 记录比赛事件（chief 或 deputy）
// DELETE /api/events?id= 撤销事件（仅 chief，按依赖规则）
func EventsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		authMiddleware("", createEvent)(w, r)
	case http.MethodDelete:
		authMiddleware(model.RoleChief, deleteEvent)(w, r)
	default:
		methodError(w, r.Method)
	}
}

// createEvent 记录比赛事件：
// exchange     交接（含第 6 棒冲线），自动派生下一棒出发
// backup_start 备用黑色接力带出发（早于前棒出发+50min 需确认放行）
// solo_finish  备用带已发出后的补录到达
// withdraw     退赛
func createEvent(w http.ResponseWriter, r *http.Request, claims *Claims) {
	var req struct {
		Type         string `json:"type"`
		RunnerId     *int64 `json:"runnerId"`
		Ms           *int64 `json:"ms"`
		ClientAt     *int64 `json:"clientAt"`
		ConfirmEarly bool   `json:"confirmEarly"`
	}
	if err := decodeBody(r, &req); err != nil {
		fail(w, genericError("请求参数格式错误"))
		return
	}
	switch req.Type {
	case model.EventExchange, model.EventBackupStart, model.EventSoloFinish, model.EventWithdraw:
	default:
		fail(w, genericError("事件类型无效"))
		return
	}
	if req.RunnerId == nil || *req.RunnerId <= 0 {
		fail(w, genericError("参数 runnerId 无效"))
		return
	}
	if req.Ms == nil || *req.Ms < 0 {
		fail(w, genericError("参数 ms 无效"))
		return
	}

	runnerId := *req.RunnerId
	ms := *req.Ms
	var saved *model.RaceEvent
	var vo *eventVO

	err := dao.WithRaceTx(func(tx *gorm.DB, race *model.Race) error {
		if race.Status != model.StatusRunning {
			return dao.ErrRaceNotRunning
		}
		teams, err := dao.LoadTeams(tx)
		if err != nil {
			return err
		}
		runners, err := dao.LoadRunners(tx)
		if err != nil {
			return err
		}
		events, err := dao.LoadEvents(tx)
		if err != nil {
			return err
		}
		snap := BuildSnapshot(teams, runners, events, true)

		var nextRunnerId *int64
		early := false
		switch req.Type {
		case model.EventExchange:
			next, rerr := snap.CheckExchange(runnerId)
			if rerr != nil {
				return rerr
			}
			if next != nil {
				id := next.RunnerId
				nextRunnerId = &id
			}
		case model.EventBackupStart:
			if _, rerr := snap.CheckBackupStart(runnerId, ms); rerr != nil {
				if !(rerr.kind == ruleKindEarly && req.ConfirmEarly) {
					return rerr
				}
				early = true // 早于规则时刻，经确认放行
			}
		case model.EventSoloFinish:
			if rerr := snap.CheckSoloFinish(runnerId); rerr != nil {
				return rerr
			}
		case model.EventWithdraw:
			if rerr := snap.CheckWithdraw(runnerId); rerr != nil {
				return rerr
			}
		}

		saved = &model.RaceEvent{
			RaceId:       model.SingleRaceId,
			Type:         req.Type,
			RunnerId:     runnerId,
			NextRunnerId: nextRunnerId,
			Ms:           ms,
			Early:        early,
			ServerMs:     raceElapsedMs(race),
			RecordedBy:   claims.DisplayName,
			ClientAt:     req.ClientAt,
		}
		if err := tx.Create(saved).Error; err != nil {
			return err
		}
		vo = buildEventVO(saved, teams, runners)
		return nil
	})
	if err != nil {
		fail(w, translateEventError(err))
		return
	}
	sendOk(w, &eventCreateData{Event: vo})
}

// deleteEvent 撤销事件（chief）：依赖检查后物理删除
func deleteEvent(w http.ResponseWriter, r *http.Request, claims *Claims) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, genericError("参数 id 无效"))
		return
	}
	err = dao.WithRaceTx(func(tx *gorm.DB, race *model.Race) error {
		events, err := dao.LoadEvents(tx)
		if err != nil {
			return err
		}
		if rerr := CheckDeleteEvent(events, id); rerr != nil {
			return rerr
		}
		res := tx.Where("id = ?", id).Delete(&model.RaceEvent{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return dao.ErrEventNotFound
		}
		return nil
	})
	if err != nil {
		fail(w, translateEventError(err))
		return
	}
	sendOk(w, struct{}{})
}

// translateEventError 事件相关错误 → 业务错误码
func translateEventError(err error) error {
	var rerr *ruleError
	if errors.As(err, &rerr) {
		switch rerr.kind {
		case ruleKindEarly:
			return &bizError{
				code: CodeEventEarly,
				msg:  rerr.msg + "。确认提前放行吗？",
				data: map[string]int64{"earliestMs": rerr.earliestMs},
			}
		case ruleKindConflict:
			return newBizError(CodeEventConflict, rerr.msg)
		default:
			return newBizError(CodeEventInvalid, rerr.msg)
		}
	}
	if errors.Is(err, dao.ErrRaceNotRunning) {
		return newBizError(CodeRaceNotRunning, err.Error())
	}
	if errors.Is(err, dao.ErrEventNotFound) {
		return genericError(err.Error())
	}
	return internalError(err)
}
