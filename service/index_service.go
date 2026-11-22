package service

import (
	"fmt"
	"net/http"
)

// IndexHandler 健康检查：仅对根路径 / 返回服务信息。
// Go 的 "/" 是 catch-all，如果对所有路径都回 200，未知接口会拿到健康检查 JSON，
// 前端只会看到「响应格式异常」，掩盖真正的问题（如路径拼写错误/版本未更新）。
// 因此这里对非根路径显式返回 404，让问题一眼可见。
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("content-type", "application/json")
	fmt.Fprint(w, `{"service":"league-timer-backend","status":"ok"}`)
}
