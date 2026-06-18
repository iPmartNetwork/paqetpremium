//go:build linux

package pcap

import (
	"fmt"
	"time"

	"github.com/gopacket/gopacket/pcap"
	"github.com/paqetpremium/paqetpremium/internal/config"
)

func openHandle(net *config.NetworkRuntime, timeout time.Duration) (*pcap.Handle, error) {
	inactive, err := pcap.NewInactiveHandle(net.Interface.Name)
	if err != nil {
		return nil, fmt.Errorf("pcap inactive handle on %s: %w", net.Interface.Name, err)
	}
	defer inactive.CleanUp()

	if err := inactive.SetBufferSize(net.Sockbuf); err != nil {
		return nil, fmt.Errorf("pcap buffer size: %w", err)
	}
	if err := inactive.SetSnapLen(65536); err != nil {
		return nil, fmt.Errorf("pcap snaplen: %w", err)
	}
	if err := inactive.SetPromisc(true); err != nil {
		return nil, fmt.Errorf("pcap promisc: %w", err)
	}
	if err := inactive.SetTimeout(timeout); err != nil {
		return nil, fmt.Errorf("pcap timeout: %w", err)
	}
	if err := inactive.SetImmediateMode(true); err != nil {
		return nil, fmt.Errorf("pcap immediate mode: %w", err)
	}

	handle, err := inactive.Activate()
	if err != nil {
		return nil, fmt.Errorf("pcap activate on %s: %w", net.Interface.Name, err)
	}
	return handle, nil
}
