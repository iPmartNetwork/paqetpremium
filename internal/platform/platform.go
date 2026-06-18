// Package platform documents and enforces PaqetPremium deployment targets.
//
// PaqetPremium is Linux-only. It is designed for VPS tunnels between:
//   - Iran entry (client role)
//   - foreign exit (server role / Kharej)
package platform

import (
	"fmt"
	"runtime"
)

const (
	TargetOS = "linux"
	// Supported: amd64 (primary), arm64
)

func IsLinux() bool {
	return runtime.GOOS == TargetOS
}

func RequireLinux() error {
	if !IsLinux() {
		return fmt.Errorf(
			"%s requires Linux (Iran/foreign VPS). current OS: %s/%s",
			"PaqetPremium", runtime.GOOS, runtime.GOARCH,
		)
	}
	return nil
}
