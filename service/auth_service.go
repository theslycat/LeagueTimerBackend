package service

import (
	"crypto/hmac"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"wxcloudrun-golang/db/dao"
	"wxcloudrun-golang/db/model"
)

// 按需求硬编码的唯一主裁判账号（全局唯一 chief，不落库）。
// 主裁判拥有全部权限：开始/暂停/终止/重置比赛、管理副裁判账号、记录交接时间。
// 副裁判（deputy）由主裁判通过接口创建并存 MySQL。
const (
	ChiefUsername    = "chief"
	ChiefPassword    = "league-timer-2026"
	chiefDisplayName = "主裁判"
)

// loginData POST /api/login 成功响应 data
type loginData struct {
	Token       string `json:"token"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	DisplayName string `json:"displayName"`
}

// LoginHandler POST /api/login 主裁判/副裁判登录
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodError(w, r.Method)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &req); err != nil {
		fail(w, genericError("请求参数格式错误"))
		return
	}
	username := strings.TrimSpace(req.Username)

	// 主裁判：硬编码账号，密码常数时间比较
	if username == ChiefUsername {
		if !hmac.Equal([]byte(req.Password), []byte(ChiefPassword)) {
			fail(w, genericError("用户名或密码错误"))
			return
		}
		token, err := GenerateToken(&Claims{
			Uid:         0,
			Username:    ChiefUsername,
			Role:        model.RoleChief,
			DisplayName: chiefDisplayName,
		})
		if err != nil {
			fail(w, internalError(err))
			return
		}
		sendOk(w, &loginData{
			Token:       token,
			Username:    ChiefUsername,
			Role:        model.RoleChief,
			DisplayName: chiefDisplayName,
		})
		return
	}

	// 副裁判：查库校验 PBKDF2 摘要
	referee, err := dao.RefereeImp.GetRefereeByUsername(username)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			fail(w, genericError("用户名或密码错误"))
			return
		}
		fail(w, internalError(err))
		return
	}
	if referee.Role != model.RoleDeputy || !VerifyPassword(req.Password, referee.PasswordHash) {
		fail(w, genericError("用户名或密码错误"))
		return
	}
	token, err := GenerateToken(&Claims{
		Uid:         referee.Id,
		Username:    referee.Username,
		Role:        model.RoleDeputy,
		DisplayName: referee.DisplayName,
	})
	if err != nil {
		fail(w, internalError(err))
		return
	}
	sendOk(w, &loginData{
		Token:       token,
		Username:    referee.Username,
		Role:        model.RoleDeputy,
		DisplayName: referee.DisplayName,
	})
}
