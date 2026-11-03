//go:build !linux

package engine

import "syscall"

// No parent-death signal outside Linux; the guard process kills an orphaned engine.
func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
