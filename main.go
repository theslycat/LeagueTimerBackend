package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"wxcloudrun-golang/db"
	"wxcloudrun-golang/service"
)

func main() {
	if err := db.Init(); err != nil {
		panic(fmt.Sprintf("mysql init failed with %+v", err))
	}

	// 建表 + 幂等种子数据（race 单例、演示队伍与选手）
	if err := db.Migrate(); err != nil {
		panic(fmt.Sprintf("db migrate failed with %+v", err))
	}

	http.HandleFunc("/", service.IndexHandler)
	http.HandleFunc("/api/login", service.LoginHandler)
	http.HandleFunc("/api/referees", service.RefereesHandler)
	http.HandleFunc("/api/race", service.RaceHandler)
	http.HandleFunc("/api/race/start", service.RaceStartHandler)
	http.HandleFunc("/api/race/pause", service.RacePauseHandler)
	http.HandleFunc("/api/race/terminate", service.RaceTerminateHandler)
	http.HandleFunc("/api/race/reset", service.RaceResetHandler)
	http.HandleFunc("/api/participants", service.ParticipantsHandler)
	http.HandleFunc("/api/events", service.EventsHandler)
	http.HandleFunc("/api/ranking", service.RankingHandler)

	// 云托管容器监听 80；本地调试可用 PORT 环境变量覆盖（如 PORT=8080）
	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
