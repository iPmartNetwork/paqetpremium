//go:build linux

package pcap

import (
	"fmt"
	"net"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/paqetpremium/paqetpremium/internal/config"
)

const readTimeout = 100 * time.Millisecond

type recvHandle struct {
	handle *pcap.Handle
}

func newRecvHandle(net *config.NetworkRuntime) (*recvHandle, error) {
	handle, err := openHandle(net, readTimeout)
	if err != nil {
		return nil, err
	}
	if err := handle.SetDirection(pcap.DirectionIn); err != nil {
		handle.Close()
		return nil, fmt.Errorf("pcap direction in: %w", err)
	}
	filter := fmt.Sprintf("(tcp and dst port %d) or (ip6 and tcp and dst port %d)", net.Port, net.Port)
	if net.IPv6 != nil && net.IPv6.Port != net.Port {
		filter = fmt.Sprintf("%s or (ip6 and tcp and dst port %d)", filter, net.IPv6.Port)
	}
	if err := handle.SetBPFFilter(filter); err != nil {
		handle.Close()
		return nil, fmt.Errorf("pcap bpf filter: %w", err)
	}
	return &recvHandle{handle: handle}, nil
}

func (h *recvHandle) read() ([]byte, net.Addr, error) {
	data, _, err := h.handle.ReadPacketData()
	if err != nil {
		return nil, nil, err
	}

	pkt := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.NoCopy)
	netLayer := pkt.NetworkLayer()
	if netLayer == nil {
		return nil, nil, nil
	}

	addr := &net.UDPAddr{}
	switch nl := netLayer.(type) {
	case *layers.IPv4:
		addr.IP = nl.SrcIP
	case *layers.IPv6:
		addr.IP = nl.SrcIP
	default:
		return nil, nil, nil
	}

	tr := pkt.TransportLayer()
	if tr == nil {
		return nil, nil, nil
	}
	if tcp, ok := tr.(*layers.TCP); ok {
		addr.Port = int(tcp.SrcPort)
	} else {
		return nil, nil, nil
	}

	app := pkt.ApplicationLayer()
	if app == nil {
		return nil, nil, nil
	}
	return app.Payload(), addr, nil
}

func (h *recvHandle) close() {
	if h.handle != nil {
		h.handle.Close()
	}
}
