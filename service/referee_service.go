package service

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"wxcloudrun-golang/db/dao"
	"wxcloudrun-golang/db/model"
)

// refereeUsernamePattern 副裁判用户名规则：2-32 位字母、数字或下划线
var refereeUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{2,32}$`)

// refereeVO 副裁判对外视图（不含密码摘要）
type refereeVO struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	CreatedAt   int64  `json:"createdAt"`
}

type refereeListData struct {
	Referees []*refereeVO `json:"referees"`
}

type refereeCreateData struct {
	Referee *refereeVO `json:"referee"`
}

// RefereesHandler 副裁判管理（仅主裁判）：
// GET 列表 / POST 创建 / DELETE?username=xxx 删除
func RefereesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		authMiddleware(model.RoleChief, listReferees)(w, r)
	case http.MethodPost:
		authMiddleware(model.RoleChief, createReferee)(w, r)
	case http.MethodDelete:
		authMiddleware(model.RoleChief, deleteReferee)(w, r)
	default:
		methodError(w, r.Method)
	}
}

func toRefereeVO(referee *model.Referee) *refereeVO {
	return &refereeVO{
		Username:    referee.Username,
		DisplayName: referee.DisplayName,
		CreatedAt:   unixMilli(referee.CreatedAt),
	}
}

// listReferees 副裁判列表（按创建时间升序）
func listReferees(w http.ResponseWriter, r *http.Request, claims *Claims) {
	referees, err := dao.RefereeImp.ListReferees()
	if err != nil {
		fail(w, internalError(err))
		return
	}
	list := make([]*refereeVO, 0, len(referees))
	for i := range referees {
		list = append(list, toRefereeVO(&referees[i]))
	}
	sendOk(w, &refereeListData{Referees: list})
}

// createReferee 创建副裁判：username 2-32 位字母数字下划线、
// password >= 6 位、displayName 非空且 <= 32 字符；不允许创建 chief
func createReferee(w http.ResponseWriter, r *http.Request, claims *Claims) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
	}
	if err := decodeBody(r, &req); err != nil {
		fail(w, genericError("请求参数格式错误"))
		return
	}
	username := strings.TrimSpace(req.Username)
	displayName := strings.TrimSpace(req.DisplayName)

	if !refereeUsernamePattern.MatchString(username) {
		fail(w, genericError("用户名需为 2-32 位字母、数字或下划线"))
		return
	}
	if strings.EqualFold(username, ChiefUsername) {
		fail(w, genericError("不允许使用该用户名"))
		return
	}
	if utf8.RuneCountInString(req.Password) < 6 {
		fail(w, genericError("密码长度至少 6 位"))
		return
	}
	if displayName == "" || utf8.RuneCountInString(displayName) > 32 {
		fail(w, genericError("姓名不能为空且不超过 32 个字符"))
		return
	}

	// 重名预检查（创建时仍有唯一索引兜底）
	if _, err := dao.RefereeImp.GetRefereeByUsername(username); err == nil {
		fail(w, genericError("用户名已存在"))
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		fail(w, internalError(err))
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		fail(w, internalError(err))
		return
	}
	referee := &model.Referee{
		Username:     username,
		PasswordHash: hash,
		DisplayName:  displayName,
		Role:         model.RoleDeputy,
	}
	if err := dao.RefereeImp.CreateReferee(referee); err != nil {
		if dao.IsDuplicateEntryError(err) {
			fail(w, genericError("用户名已存在"))
			return
		}
		fail(w, internalError(err))
		return
	}
	sendOk(w, &refereeCreateData{Referee: toRefereeVO(referee)})
}

// deleteReferee 删除副裁判（DELETE /api/referees?username=xxx）
func deleteReferee(w http.ResponseWriter, r *http.Request, claims *Claims) {
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if username == "" {
		fail(w, genericError("缺少 username 参数"))
		return
	}
	rows, err := dao.RefereeImp.DeleteRefereeByUsername(username)
	if err != nil {
		fail(w, internalError(err))
		return
	}
	if rows == 0 {
		fail(w, genericError("裁判不存在"))
		return
	}
	sendOk(w, struct{}{})
}
