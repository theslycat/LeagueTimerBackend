package service

import (
	"errors"
	"net/http"
	"time"

	"gorm.io/gorm"

	"wxcloudrun-golang/db/dao"
)

// recordVO 交接记录对外视图
type recordVO struct {
	Id         int64  `json:"id"`
	RunnerId   int64  `json:"runnerId"`
	PassMs     int64  `json:"passMs"`
	ServerMs   int64  `json:"serverMs"`
	RecordedBy string `json:"recordedBy"`
	CreatedAt  int64  `json:"createdAt"`
}

type recordCreateData struct {
	Record *recordVO `json:"record"`
}

type recordListData struct {
	Records []*recordVO `json:"records"`
}

// RecordsHandler POST 记录交接时间 / GET 查询全部记录（chief 或 deputy）
func RecordsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		authMiddleware("", createRecord)(w, r)
	case http.MethodGet:
		authMiddleware("", listRecords)(w, r)
	default:
		methodError(w, r.Method)
	}
}

// createRecord 记录交接时间：
// 40003 选手不存在 / 40001 比赛非 running / 40002 重复记录
func createRecord(w http.ResponseWriter, r *http.Request, claims *Claims) {
	var req struct {
		RunnerId *int64 `json:"runnerId"`
		PassMs   *int64 `json:"passMs"`
		ClientAt *int64 `json:"clientAt"`
	}
	if err := decodeBody(r, &req); err != nil {
		fail(w, genericError("请求参数格式错误"))
		return
	}
	if req.RunnerId == nil || *req.RunnerId <= 0 {
		fail(w, genericError("参数 runnerId 无效"))
		return
	}
	if req.PassMs == nil || *req.PassMs < 0 {
		fail(w, genericError("参数 passMs 无效"))
		return
	}
	// 选手存在性校验
	if _, err := dao.RunnerImp.GetRunnerById(*req.RunnerId); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(w, newBizError(CodeRunnerNotFound, "选手不存在"))
			return
		}
		fail(w, internalError(err))
		return
	}
	record, err := dao.RecordImp.CreateRecord(time.Now(), *req.RunnerId, *req.PassMs,
		claims.DisplayName, req.ClientAt)
	if err != nil {
		fail(w, translateRecordError(err))
		return
	}
	sendOk(w, &recordCreateData{Record: &recordVO{
		Id:         record.Id,
		RunnerId:   record.RunnerId,
		PassMs:     record.PassMs,
		ServerMs:   record.ServerMs,
		RecordedBy: record.RecordedBy,
		CreatedAt:  unixMilli(record.CreatedAt),
	}})
}

// listRecords 全部交接记录（按 pass_ms 升序）
func listRecords(w http.ResponseWriter, r *http.Request, claims *Claims) {
	records, err := dao.RecordImp.ListRecords()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	list := make([]*recordVO, 0, len(records))
	for i := range records {
		record := &records[i]
		list = append(list, &recordVO{
			Id:         record.Id,
			RunnerId:   record.RunnerId,
			PassMs:     record.PassMs,
			ServerMs:   record.ServerMs,
			RecordedBy: record.RecordedBy,
			CreatedAt:  unixMilli(record.CreatedAt),
		})
	}
	sendOk(w, &recordListData{Records: list})
}

func translateRecordError(err error) error {
	switch {
	case errors.Is(err, dao.ErrRaceNotRunning):
		return newBizError(CodeRaceNotRunning, err.Error())
	case errors.Is(err, dao.ErrRecordDuplicated):
		return newBizError(CodeRecordDuplicated, err.Error())
	default:
		return internalError(err)
	}
}
