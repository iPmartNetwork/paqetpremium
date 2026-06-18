package transport

import (
	"crypto/sha256"
	"fmt"

	"github.com/xtaci/kcp-go/v5"
	"golang.org/x/crypto/pbkdf2"
)

// pbkdf2 salt matches the paqet family for wire compatibility with existing tunnels.
const kcpSalt = "paqet"

func NewBlockCrypt(block, key string) (kcp.BlockCrypt, error) {
	derived := pbkdf2.Key([]byte(key), []byte(kcpSalt), 100_000, 32, sha256.New)

	type spec struct {
		keyLen int
		build  func([]byte) (kcp.BlockCrypt, error)
	}

	table := map[string]spec{
		"aes":         {0, kcp.NewAESBlockCrypt},
		"aes-128":     {16, kcp.NewAESBlockCrypt},
		"aes-128-gcm": {16, kcp.NewAESGCMCrypt},
		"aes-192":     {24, kcp.NewAESBlockCrypt},
		"salsa20":     {0, kcp.NewSalsa20BlockCrypt},
		"aes-256":     {32, kcp.NewAESBlockCrypt},
		"none":        {0, kcp.NewNoneBlockCrypt},
		"null":        {0, func([]byte) (kcp.BlockCrypt, error) { return nil, nil }},
	}

	s, ok := table[block]
	if !ok {
		return nil, fmt.Errorf("unsupported kcp block %q", block)
	}

	material := derived
	if s.keyLen > 0 && len(material) >= s.keyLen {
		material = material[:s.keyLen]
	}
	return s.build(material)
}

func ApplyKCP(session *kcp.UDPSession, opt Options) {
	var noDelay, interval, resend, noCongestion int
	var wDelay, ackNoDelay bool
	switch opt.KCP.Mode {
	case "normal":
		noDelay, interval, resend, noCongestion = 0, 40, 2, 1
		wDelay, ackNoDelay = true, false
	case "fast2":
		noDelay, interval, resend, noCongestion = 1, 20, 2, 1
		wDelay, ackNoDelay = false, true
	case "fast3":
		noDelay, interval, resend, noCongestion = 1, 10, 2, 1
		wDelay, ackNoDelay = false, true
	default: // fast
		noDelay, interval, resend, noCongestion = 0, 30, 2, 1
		wDelay, ackNoDelay = true, false
	}

	session.SetNoDelay(noDelay, interval, resend, noCongestion)
	session.SetWindowSize(opt.KCP.SndWnd, opt.KCP.RcvWnd)
	session.SetMtu(opt.KCP.MTU)
	session.SetWriteDelay(wDelay)
	session.SetACKNoDelay(ackNoDelay)
	session.SetDSCP(46)
}
