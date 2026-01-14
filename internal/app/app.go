package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/likaia/nginxpulse/internal/analytics"
	"github.com/likaia/nginxpulse/internal/cli"
	"github.com/likaia/nginxpulse/internal/config"
	"github.com/likaia/nginxpulse/internal/enrich"
	"github.com/likaia/nginxpulse/internal/ingest"
	"github.com/likaia/nginxpulse/internal/logging"
	"github.com/likaia/nginxpulse/internal/server"
	"github.com/likaia/nginxpulse/internal/store"
	"github.com/likaia/nginxpulse/internal/version"
	"github.com/likaia/nginxpulse/internal/worker"
	"github.com/sirupsen/logrus"
)

// Run wires the application dependencies and blocks until shutdown.
func Run() error {
	if cli.ProcessCliCommands() {
		return nil
	}

	logging.ConfigureLogging()
	defer logging.CloseLogFile()

	logrus.Info("------ 服务启动成功 ------")
	logrus.Infof("构建时间: %s, Git提交: %s", version.BuildTime, version.GitCommit)
	defer logrus.Info("------ 服务已安全关闭 ------")

	if err := enrich.InitIPGeoLocation(); err != nil {
		return err
	}

	repository, err := initRepository()
	if err != nil {
		return err
	}
	defer repository.Close()

	logParser := ingest.NewLogParser(repository)
	statsFactory := analytics.NewStatsFactory(repository)

	cfg := config.ReadConfig()
	serverHandle := server.StartHTTPServer(statsFactory, logParser, cfg.Server.Port)

	go worker.InitialScan(logParser)

	interval := config.ParseInterval(cfg.System.TaskInterval, 5*time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.RunScheduler(ctx, logParser, interval)

	return waitForShutdown(cancel, serverHandle)
}

func initRepository() (*store.Repository, error) {
	logrus.Info("****** 1 初始化数据 ******")
	repository, err := store.NewRepository()
	if err != nil {
		logrus.WithField("error", err).Error("Failed to create database file")
		return repository, err
	}

	if err := repository.Init(); err != nil {
		logrus.WithField("error", err).Error("Failed to create tables")
		return repository, err
	}

	return repository, nil
}

func waitForShutdown(cancel context.CancelFunc, serverHandle *http.Server) error {
	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)
	<-shutdownSignal

	logrus.Info("开始关闭服务 ......")

	cancel()

	ctx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if serverHandle != nil {
		if err := serverHandle.Shutdown(ctx); err != nil {
			logrus.WithError(err).Warn("HTTP 服务器关闭异常")
		}
	}

	return nil
}
