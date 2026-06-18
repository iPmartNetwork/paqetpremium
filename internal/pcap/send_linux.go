//go:build linux

package pcap

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/netutil"
)

type sendHandle struct {
	handle       *pcap.Handle
	net          *config.NetworkRuntime
	srcIP        net.IP
	routerMAC    net.HardwareAddr
	srcPort      uint16
	ipv6SrcIP    net.IP
	ipv6RouterMAC net.HardwareAddr
	baseTime     uint32
	tsCounter    uint32
	localFlags   *netutil.FlagCycle
	remoteByPeer map[uint64]*netutil.FlagCycle
	mu           sync.RWMutex
}

func newSendHandle(rt *config.NetworkRuntime) (*sendHandle, error) {
	handle, err := openHandle(rt, pcap.BlockForever)
	if err != nil {
		return nil, err
	}
	if err := handle.SetDirection(pcap.DirectionOut); err != nil {
		handle.Close()
		return nil, fmt.Errorf("pcap direction out: %w", err)
	}

	sh := &sendHandle{
		handle:       handle,
		net:          rt,
		srcIP:        append(net.IP(nil), rt.IPv4.IP...),
		routerMAC:    append(net.HardwareAddr(nil), rt.RouterMAC...),
		srcPort:      uint16(rt.Port),
		baseTime:     uint32(time.Now().UnixNano() / int64(time.Millisecond)),
		localFlags:   netutil.NewFlagCycle(rt.LocalFlags),
		remoteByPeer: make(map[uint64]*netutil.FlagCycle),
	}
	if rt.IPv6 != nil {
		sh.ipv6SrcIP = append(net.IP(nil), rt.IPv6.IP...)
		sh.ipv6RouterMAC = append(net.HardwareAddr(nil), rt.IPv6RouterMAC...)
	}
	return sh, nil
}

func (h *sendHandle) setRemoteFlags(addr net.Addr, flags []netutil.TCPFlagSet) {
	ua, ok := addr.(*net.UDPAddr)
	if !ok {
		return
	}
	h.mu.Lock()
	h.remoteByPeer[netutil.AddrKey(ua.IP, ua.Port)] = netutil.NewFlagCycle(flags)
	h.mu.Unlock()
}

func (h *sendHandle) deleteRemoteFlags(addr net.Addr) {
	ua, ok := addr.(*net.UDPAddr)
	if !ok {
		return
	}
	h.mu.Lock()
	delete(h.remoteByPeer, netutil.AddrKey(ua.IP, ua.Port))
	h.mu.Unlock()
}

func (h *sendHandle) flagsFor(dst *net.UDPAddr) netutil.TCPFlagSet {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if c, ok := h.remoteByPeer[netutil.AddrKey(dst.IP, dst.Port)]; ok {
		return c.Next()
	}
	return h.localFlags.Next()
}

func (h *sendHandle) write(payload []byte, dst *net.UDPAddr) error {
	if dst.IP.To4() == nil {
		return h.writeIPv6(payload, dst)
	}
	return h.writeIPv4(payload, dst)
}

func (h *sendHandle) writeIPv4(payload []byte, dst *net.UDPAddr) error {
	f := h.flagsFor(dst)

	eth := &layers.Ethernet{
		SrcMAC:       h.net.Interface.HardwareAddr,
		DstMAC:       h.routerMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TOS:      184,
		TTL:      64,
		Flags:    layers.IPv4DontFragment,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    h.srcIP,
		DstIP:    dst.IP.To4(),
	}
	tcp := h.buildTCP(uint16(dst.Port), f)
	tcp.SetNetworkLayerForChecksum(ip)

	buf := gopacket.NewSerializeBuffer()
	defer buf.Clear()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, tcp, gopacket.Payload(payload)); err != nil {
		return err
	}
	return h.handle.WritePacketData(buf.Bytes())
}

func (h *sendHandle) writeIPv6(payload []byte, dst *net.UDPAddr) error {
	if h.ipv6SrcIP == nil {
		return fmt.Errorf("ipv6 not configured")
	}
	f := h.flagsFor(dst)

	eth := &layers.Ethernet{
		SrcMAC:       h.net.Interface.HardwareAddr,
		DstMAC:       h.ipv6RouterMAC,
		EthernetType: layers.EthernetTypeIPv6,
	}
	ip6 := &layers.IPv6{
		Version:    6,
		TrafficClass: 184,
		HopLimit:   64,
		NextHeader: layers.IPProtocolTCP,
		SrcIP:      h.ipv6SrcIP,
		DstIP:      dst.IP,
	}
	tcp := h.buildTCP(uint16(dst.Port), f)
	tcp.SetNetworkLayerForChecksum(ip6)

	buf := gopacket.NewSerializeBuffer()
	defer buf.Clear()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip6, tcp, gopacket.Payload(payload)); err != nil {
		return err
	}
	return h.handle.WritePacketData(buf.Bytes())
}

func (h *sendHandle) buildTCP(dstPort uint16, f netutil.TCPFlagSet) *layers.TCP {
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(h.srcPort),
		DstPort: layers.TCPPort(dstPort),
		FIN:     f.FIN,
		SYN:     f.SYN,
		RST:     f.RST,
		PSH:     f.PSH,
		ACK:     f.ACK,
		URG:     f.URG,
		ECE:     f.ECE,
		CWR:     f.CWR,
		NS:      f.NS,
		Window:  65535,
	}

	counter := atomic.AddUint32(&h.tsCounter, 1)
	tsVal := h.baseTime + (counter >> 3)
	if f.SYN {
		tcp.Options = []layers.TCPOption{
			{OptionType: layers.TCPOptionKindMSS, OptionLength: 4, OptionData: []byte{0x05, 0xb4}},
			{OptionType: layers.TCPOptionKindSACKPermitted, OptionLength: 2},
			{OptionType: layers.TCPOptionKindTimestamps, OptionLength: 10, OptionData: make([]byte, 8)},
			{OptionType: layers.TCPOptionKindNop},
			{OptionType: layers.TCPOptionKindWindowScale, OptionLength: 3, OptionData: []byte{8}},
		}
		binary.BigEndian.PutUint32(tcp.Options[2].OptionData[0:4], tsVal)
		tcp.Seq = 1 + (counter & 0x7)
		if f.ACK {
			tcp.Ack = tcp.Seq + 1
		}
	} else {
		tcp.Options = []layers.TCPOption{
			{OptionType: layers.TCPOptionKindNop},
			{OptionType: layers.TCPOptionKindNop},
			{OptionType: layers.TCPOptionKindTimestamps, OptionLength: 10, OptionData: make([]byte, 8)},
		}
		tsEcr := tsVal - (counter%200 + 50)
		binary.BigEndian.PutUint32(tcp.Options[2].OptionData[0:4], tsVal)
		binary.BigEndian.PutUint32(tcp.Options[2].OptionData[4:8], tsEcr)
		seq := h.baseTime + (counter << 7)
		tcp.Seq = seq
		tcp.Ack = seq - (counter & 0x3ff) + 1400
	}
	return tcp
}

func (h *sendHandle) close() {
	if h.handle != nil {
		h.handle.Close()
	}
}
