package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// 业务错误码
const (
	CodeOK               = 0     // 成功
	CodeGeneric          = -1    // 通用业务错误
	CodeInternal         = 500   // 内部错误（不向前端透出细节）
	CodeUnauthorized     = 40100 // 未登录 / token 无效或过期
	CodeForbidden        = 40300 // 无权限
	CodeRaceNotRunning   = 40001 // 比赛未开始或已暂停
	CodeRecordDuplicated = 40002 // 该选手已记录交接时间
	CodeRunnerNotFound   = 40003 // 选手不存在
)

// JsonResult 返回结构（与模板保持一致的响应包）
type JsonResult struct {
	Code     int         `json:"code"`
	ErrorMsg string      `json:"errorMsg,omitempty"`
	Data     interface{} `json:"data"`
}

// bizError 业务错误：携带错误码与对外展示文案
type bizError struct {
	code int
	msg  string
}

func (e *bizError) Error() string { return e.msg }

func newBizError(code int, msg string) *bizError {
	return &bizError{code: code, msg: msg}
}

func genericError(msg string) *bizError {
	return newBizError(CodeGeneric, msg)
}

// internalError 记录详细日志后返回通用内部错误，避免泄漏 SQL 等细节
func internalError(err error) *bizError {
	log.Printf("internal error: %+v", err)
	return newBizError(CodeInternal, "服务内部错误，请稍后重试")
}

// writeJson 统一输出响应，HTTP 状态码恒为 200，业务结果由 code 表达
func writeJson(w http.ResponseWriter, res *JsonResult) {
	msg, err := json.Marshal(res)
	if err != nil {
		log.Println("marshal response error:", err)
		fmt.Fprint(w, "内部错误")
		return
	}
	w.Header().Set("content-type", "application/json")
	w.Write(msg)
}

func sendOk(w http.ResponseWriter, data interface{}) {
	writeJson(w, &JsonResult{Code: CodeOK, Data: data})
}

func sendError(w http.ResponseWriter, code int, msg string) {
	writeJson(w, &JsonResult{Code: code, ErrorMsg: msg})
}

// fail 将错误转换为响应：业务错误按码返回，其余按内部错误处理
func fail(w http.ResponseWriter, err error) {
	if be, ok := err.(*bizError); ok {
		sendError(w, be.code, be.msg)
		return
	}
	sendError(w, CodeInternal, internalError(err).msg)
}

// decodeBody 解析 JSON 请求体
func decodeBody(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// methodError 请求方法不支持
func methodError(w http.ResponseWriter, method string) {
	sendError(w, CodeGeneric, fmt.Sprintf("请求方法 %s 不支持", method))
}

// unixMilli 时间转毫秒时间戳（兼容 go1.16，不依赖较新的 UnixMilli）
func unixMilli(t time.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond)
}
