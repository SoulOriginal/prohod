// Package guard is a tiny watchdog process: if the app dies while connected, it restores the system proxy
// (from the state file) and kills the orphaned engine, so the user is never left offline.
package guard

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"prohod/internal/sysproxy"
)

// Flag is the argv[1] that switches the binary into guard mode.
const Flag = "--prohod-guard"

// Spawn starts a detached guard for the current process.
func Spawn(statePath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, Flag, strconv.Itoa(os.Getpid()), statePath)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin", "HOME=" + os.Getenv("HOME"),
		"DBUS_SESSION_BUS_ADDRESS=" + os.Getenv("DBUS_SESSION_BUS_ADDRESS"), "XDG_RUNTIME_DIR=" + os.Getenv("XDG_RUNTIME_DIR")}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // survives the app's process group
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// alive reports whether pid still exists.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// Run is the guard main loop. It returns when the parent exits.
func Run(args []string) {
	if len(args) < 2 {
		return
	}
	parent, err := strconv.Atoi(args[0])
	if err != nil || parent <= 1 {
		return
	}
	statePath := args[1]
	for alive(parent) {
		time.Sleep(time.Second)
	}
	// Parent is gone. If it was still connected, the state file is there: repair.
	_, _ = sysproxy.RepairStale(statePath)
}
