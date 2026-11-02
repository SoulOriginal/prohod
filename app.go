package main

import (
	"context"
	"os/exec"
	"runtime"

	"prohod/internal/core"
	"prohod/internal/logx"
	"prohod/internal/stats"
)

const Version = "0.1.0"

// App is the thin binding layer exposed to the UI. It only returns neutral, user-facing data.
type App struct {
	ctx context.Context
	ctl *core.Controller
}

func NewApp(ctl *core.Controller) *App { return &App{ctl: ctl} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.ctl.Settings().AutoConnect {
		go func() { _ = a.ctl.Connect() }()
	}
}

func (a *App) shutdown(ctx context.Context) { a.ctl.Shutdown() }

func (a *App) Connect() string {
	if err := a.ctl.Connect(); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) Disconnect() string {
	if err := a.ctl.Disconnect(); err != nil {
		return err.Error()
	}
	return ""
}

func (a *App) Status() core.Status     { return a.ctl.Status() }
func (a *App) Stats() stats.Snapshot   { return a.ctl.Stats.Snapshot() }
func (a *App) Logs(n int) []logx.Entry { return a.ctl.Log.Recent(n) }
func (a *App) Settings() core.Settings { return a.ctl.Settings() }
func (a *App) Version() string         { return Version }
func (a *App) SaveSettings(s core.Settings) string {
	if err := a.ctl.SaveSettings(s); err != nil {
		return err.Error()
	}
	return ""
}

// OpenLogs opens the log folder in the file manager.
func (a *App) OpenLogs() {
	dir := a.ctl.LogDir()
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", dir).Start()
	} else {
		_ = exec.Command("xdg-open", dir).Start()
	}
}
