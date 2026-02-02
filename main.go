//go:build !libtokenizers

package main

import (
	"embed"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/lengzhao/conf/autoload"
	"github.com/yincongcyincong/MuseBot/conf"
	"github.com/yincongcyincong/MuseBot/db"
	"github.com/yincongcyincong/MuseBot/http"
	"github.com/yincongcyincong/MuseBot/i18n"
	"github.com/yincongcyincong/MuseBot/logger"
	"github.com/yincongcyincong/MuseBot/metrics"
	"github.com/yincongcyincong/MuseBot/rag"
	"github.com/yincongcyincong/MuseBot/register"
	"github.com/yincongcyincong/MuseBot/robot"
)

//go:embed static/* conf/i18n/*.json conf/mcp/*.json
var StaticFiles embed.FS

func main() {
	logger.InitLogger()
	conf.InitConf()
	i18n.SetStaticFS(StaticFiles)
	i18n.InitI18n()
	db.InitTable()
	conf.InitTools()
	rag.InitRag()
	http.SetStaticFS(StaticFiles)
	http.InitHTTP()
	metrics.RegisterMetrics()
	robot.StartRobot()
	register.InitRegister()
	robot.InitCron()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
