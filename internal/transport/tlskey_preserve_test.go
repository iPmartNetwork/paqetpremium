package transport

import (
	"testing"
)

// This is a PRESERVATION test for Bug 5 (Property 8, handshake portion). It locks
// in the baseline behavior that must remain unchanged by the fix: a QUIC
// client/server pair configured with the SAME shared secret completes the TLS 1.3
// handshake.
//
// Like the Bug 5 exploration test, the authentication outcome is fully determined
// by the *tls.Config values produced by tlsConfigFromSecret, so we exercise the
// real behavior with a standard crypto/tls 1.3 handshake over net.Pipe — pure Go,
// no pcap/QUIC/CGO and no Linux build guard. It reuses the tlsHandshake helper
// defined in tlskey_bug_test.go (same package).
//
// On the UNFIXED code this passes because the client's InsecureSkipVerify accepts
// any server cert. After the Bug 5 fix it must STILL pass because both endpoints
// derive their certificate from the same shared secret, so the pinned verifier
// accepts the peer.

// TestQUICAuth_MatchingSecret_Preservation asserts that two endpoints sharing the
// same secret successfully complete the handshake.
//
// We only require the CLIENT side to complete without error. Over a synchronous
// net.Pipe the server may report a benign post-handshake error when it writes the
// TLS 1.3 session ticket (the pipe write blocks with no reader draining it), which
// is unrelated to authentication — so we ignore serverErr and assert only on the
// client. As a defensive measure we also disable session tickets on this local
// copy of the server config only; this is a local mutation of the returned config
// and does not change production behavior, which uses real QUIC connections rather
// than a blocking in-memory pipe. (The shared tlsHandshake helper still bounds the
// handshake with its own deadline, so a stalled server side cannot hang the test.)
//
// **Validates: Requirements 3.5**
func TestQUICAuth_MatchingSecret_Preservation(t *testing.T) {
	serverCfg, err := tlsConfigFromSecret("sharedsecret", "paqetpremium", true)
	if err != nil {
		t.Fatalf("building server TLS config: %v", err)
	}
	// SAME secret on both sides — this is the matching-secret (non-bug) path.
	clientCfg, err := tlsConfigFromSecret("sharedsecret", "paqetpremium", false)
	if err != nil {
		t.Fatalf("building client TLS config: %v", err)
	}

	// Disable TLS 1.3 session tickets for THIS test only to avoid the server
	// blocking on a post-handshake ticket write over the synchronous net.Pipe.
	// This is a local mutation of the returned config and does not change
	// production behavior.
	serverCfg.SessionTicketsDisabled = true

	clientErr, serverErr := tlsHandshake(t, serverCfg, clientCfg)

	// The matching-secret handshake must complete from the client's perspective.
	// We intentionally do NOT assert serverErr == nil: a benign post-handshake
	// pipe write error on the server side is acceptable and unrelated to auth.
	if clientErr != nil {
		t.Fatalf("matching-secret handshake failed on client side: clientErr=%v (serverErr=%v); "+
			"expected the client to complete the handshake because both endpoints derive their "+
			"certificate from the same shared secret", clientErr, serverErr)
	}
}
