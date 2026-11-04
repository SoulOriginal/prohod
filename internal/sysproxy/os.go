package sysproxy

import (
	"errors"
	"runtime"
	"syscall"
	"time"
)

var goos = runtime.GOOS

// Capture reads the current OS proxy configuration.
func Capture(port int) (*State, error) {
	st := &State{Port: port}
	switch goos {
	case "darwin":
		svcs, err := macCapture()
		if err != nil {
			return nil, err
		}
		st.Mac = svcs
	case "linux":
		st.KDE = kdeCapture()
		st.GNOME = gnomeCapture()
		if st.KDE == nil && st.GNOME == nil {
			return nil, errors.New("no supported desktop proxy settings (KDE or GNOME) found")
		}
	default:
		return nil, errors.New("system proxy is not supported on this platform yet")
	}
	return st, nil
}

// Apply points the OS proxy at 127.0.0.1:st.Port. On error the caller must call Restore(st).
func Apply(st *State) error {
	st.AppliedAt = time.Now()
	switch goos {
	case "darwin":
		return macApply(st.Mac, st.Port)
	case "linux":
		if st.KDE != nil {
			if err := kdeApply(st.Port); err != nil {
				return err
			}
		}
		if st.GNOME != nil {
			if err := gnomeApply(st.Port); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("unsupported platform")
}

// Restore puts back the originals saved in st.
func Restore(st *State) error {
	var errs []error
	if len(st.Mac) > 0 {
		errs = append(errs, macRestore(st.Mac))
	}
	if st.KDE != nil {
		errs = append(errs, kdeRestore(st.KDE))
	}
	if st.GNOME != nil {
		errs = append(errs, gnomeRestore(st.GNOME))
	}
	return errors.Join(errs...)
}

// killOrphan kills a leftover engine process from a crashed session.
func killOrphan(pid int) {
	if pid > 1 {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
