// Package sysproxy points the OS proxy settings at our loopback SOCKS port and restores the exact originals.
//
// Flow: Capture() reads the originals into a State, the caller persists it (SaveState) BEFORE Apply(), so that a
// crash at any point can be repaired from the state file by Restore().
package sysproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// State holds everything needed to undo Apply().
type State struct {
	Port      int         `json:"port"`
	EnginePID int         `json:"enginePid,omitempty"`
	Mac       []MacSvc    `json:"mac,omitempty"`
	KDE       *KDEState   `json:"kde,omitempty"`
	GNOME     *GNOMEState `json:"gnome,omitempty"`
	AppliedAt time.Time   `json:"appliedAt"`
}

// Bypass hosts/nets that must never go through the proxy (local nets, CGNAT / mesh-VPN ranges, loopback).
var bypass = []string{"localhost", "127.0.0.1", "::1", "*.local", "169.254/16", "10.0.0.0/8", "172.16.0.0/12",
	"192.168.0.0/16", "100.64.0.0/10"}

// run executes a command with a timeout; replaced in tests.
var run = func(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

var lookPath = exec.LookPath

// SaveState writes the state file atomically with 0600 permissions.
func SaveState(path string, st *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", " ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadState reads a state file; (nil, nil) if it does not exist.
func LoadState(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// RepairStale restores and removes a leftover state file (app crashed while connected). Returns true if it did.
func RepairStale(path string) (bool, error) {
	st, err := LoadState(path)
	if err != nil || st == nil {
		if err != nil {
			_ = os.Remove(path) // unreadable: nothing we can restore from
		}
		return false, err
	}
	if st.EnginePID > 0 {
		killOrphan(st.EnginePID)
	}
	err = Restore(st)
	if err == nil {
		_ = os.Remove(path)
	}
	return true, err
}
