package ioext

import (
	"io"

	"github.com/paqetpremium/paqetpremium/internal/metrics"
)

const copyBuffer = 32 * 1024

func Copy(dst io.Writer, src io.Reader) error {
	buf := make([]byte, copyBuffer)
	_, err := io.CopyBuffer(dst, src, buf)
	return err
}

// CopyMetered copies and records bytes on the collector (outbound from tunnel perspective per call).
func CopyMetered(dst io.Writer, src io.Reader, m *metrics.Collector, outbound bool) error {
	if m != nil {
		dst = m.MeterWriter(dst, outbound)
	}
	return Copy(dst, src)
}
