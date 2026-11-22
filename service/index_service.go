package service

import (
	"fmt"
	"net/http"
)

// IndexHandler 健康检查/主页：极简化，不再读取 index.html
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	fmt.Fprint(w, `{"service":"league-timer-backend","status":"ok"}`)
}
