package metrics

import (
	"fmt"
	"io"

	"github.com/paqetpremium/paqetpremium/internal/upstream"
)

func WritePrometheus(w io.Writer, snap Snapshot, sessions int, upstreams any) {
	_, _ = fmt.Fprintf(w, "# PaqetPremium metrics\n")
	if sessions > 0 {
		_, _ = fmt.Fprintf(w, "paqetpremium_tunnel_sessions %d\n", sessions)
	}
	_, _ = fmt.Fprintf(w, "paqetpremium_bytes_in_total %d\n", snap.BytesIn)
	_, _ = fmt.Fprintf(w, "paqetpremium_bytes_out_total %d\n", snap.BytesOut)
	_, _ = fmt.Fprintf(w, "paqetpremium_tcp_accepted_total %d\n", snap.TCPAccepted)
	_, _ = fmt.Fprintf(w, "paqetpremium_tcp_active %d\n", snap.TCPActive)
	_, _ = fmt.Fprintf(w, "paqetpremium_udp_packets_total %d\n", snap.UDPPackets)
	_, _ = fmt.Fprintf(w, "paqetpremium_relay_tcp_total %d\n", snap.RelayTCP)
	_, _ = fmt.Fprintf(w, "paqetpremium_relay_udp_total %d\n", snap.RelayUDP)
	_, _ = fmt.Fprintf(w, "paqetpremium_udp_dgram_flows_total %d\n", snap.UDPDgramFlows)
	_, _ = fmt.Fprintf(w, "paqetpremium_udp_dgram_in_total %d\n", snap.UDPDgramIn)
	_, _ = fmt.Fprintf(w, "paqetpremium_udp_dgram_out_total %d\n", snap.UDPDgramOut)
	_, _ = fmt.Fprintf(w, "paqetpremium_udp_dgram_dropped_total %d\n", snap.UDPDgramDropped)
	_, _ = fmt.Fprintf(w, "paqetpremium_errors_total %d\n", snap.Errors)
	WriteUpstream(w, upstreams)
}

func WriteUpstream(w io.Writer, raw any) {
	ups, ok := raw.([]upstream.ServerStatus)
	if !ok {
		return
	}
	for _, u := range ups {
		healthy := 0
		if u.Healthy {
			healthy = 1
		}
		active := 0
		if u.Active {
			active = 1
		}
		_, _ = fmt.Fprintf(w, "paqetpremium_upstream_healthy{name=%q,addr=%q} %d\n", u.Name, u.Addr, healthy)
		_, _ = fmt.Fprintf(w, "paqetpremium_upstream_active{name=%q} %d\n", u.Name, active)
		_, _ = fmt.Fprintf(w, "paqetpremium_upstream_rtt_ms{name=%q} %g\n", u.Name, u.RTTMs)
		_, _ = fmt.Fprintf(w, "paqetpremium_upstream_sessions{name=%q} %d\n", u.Name, u.Sessions)
	}
}
