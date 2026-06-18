//go:build !linux

package iptables

import (
	"fmt"

	"github.com/paqetpremium/paqetpremium/internal/config"
)

func Apply(net *config.NetworkRuntime) error {
	return fmt.Errorf("iptables helper requires Linux")
}

func ApplyTCPPort(port int) error {
	return fmt.Errorf("iptables helper requires Linux")
}

func Remove() error {
	return fmt.Errorf("iptables helper requires Linux")
}
