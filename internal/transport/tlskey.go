package transport

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// constReader is a deterministic, position-independent randomness source: every
// byte it yields is identical. This matters because Go's crypto stack
// (ecdsa.GenerateKey and the ECDSA signer used by x509.CreateCertificate) calls
// crypto/internal/randutil.MaybeReadByte, which non-deterministically consumes 0
// or 1 byte from the reader to discourage callers from relying on determinism.
// A positional stream would shift after that optional byte and yield different
// bytes on each call; a constant stream delivers the same bytes whether or not
// the optional byte was consumed, keeping certificate derivation byte-identical
// across the two endpoints.
type constReader byte

func (c constReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(c)
	}
	return len(p), nil
}

// deriveScalar deterministically maps the seed to a valid ECDSA private scalar d
// in [1, N-1] for the given curve. This avoids ecdsa.GenerateKey (which calls
// randutil.MaybeReadByte and is therefore non-deterministic for a fixed reader).
func deriveScalar(seed [32]byte, curve elliptic.Curve) *big.Int {
	n := curve.Params().N
	nMinus1 := new(big.Int).Sub(n, big.NewInt(1))
	d := new(big.Int).SetBytes(seed[:])
	d.Mod(d, nMinus1)
	d.Add(d, big.NewInt(1))
	return d
}

// deriveCert deterministically derives a TLS certificate and its raw leaf DER
// bytes from the shared secret. Both endpoints feed the same secret through the
// same key-derivation and template, so they compute byte-identical certificates.
// The returned der is the leaf certificate's raw DER, used for mutual pinning.
//
// Determinism is load-bearing: trust is established purely by byte-exact pinning
// (pinnedVerify), so the client and server MUST derive byte-identical DER. To
// achieve that we (a) derive the ECDSA private key directly from the seed instead
// of using ecdsa.GenerateKey, (b) sign with a constReader so the signature is
// deterministic, and (c) use fixed certificate timestamps. All three avoid the
// wall-clock / MaybeReadByte non-determinism that would otherwise diverge the DER.
func deriveCert(secret string) (tls.Certificate, []byte, error) {
	seed := sha256.Sum256([]byte(secret + ":paqetpremium-quic-v1"))

	curve := elliptic.P256()
	key := new(ecdsa.PrivateKey)
	key.Curve = curve
	key.D = deriveScalar(seed, curve)
	key.PublicKey.X, key.PublicKey.Y = curve.ScalarBaseMult(key.D.Bytes())

	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "paqetpremium"},
		// Validity window MUST use fixed, deterministic timestamps (never
		// time.Now()): the client and server derive this self-signed cert
		// independently and trust is established purely by byte-exact pinning.
		// Wall-clock timestamps would differ between the two derivations and
		// produce non-identical DER bytes, breaking the pin. The window itself
		// is not used for trust decisions.
		NotBefore:   time.Unix(0, 0).UTC(),
		NotAfter:    time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(constReader(seed[0]), &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}

	return tlsCert, der, nil
}

// pinnedVerify returns a tls.Config.VerifyPeerCertificate callback that accepts
// the peer only when it presents exactly the certificate derived from the shared
// secret (byte-identical leaf DER). Any other or absent certificate is rejected.
func pinnedVerify(expectedDER []byte) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("paqetpremium: peer presented no certificate")
		}
		if !bytes.Equal(rawCerts[0], expectedDER) {
			return fmt.Errorf("paqetpremium: peer certificate not derived from shared secret")
		}
		return nil
	}
}

func tlsConfigFromSecret(secret, alpn string, server bool) (*tls.Config, error) {
	tlsCert, der, err := deriveCert(secret)
	if err != nil {
		return nil, err
	}

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{alpn},
		// Both sides present the shared-secret-derived certificate (mutual auth).
		Certificates: []tls.Certificate{tlsCert},
	}
	if server {
		// Require a client certificate and pin it to the derived cert.
		cfg.ClientAuth = tls.RequireAnyClientCert
		cfg.VerifyPeerCertificate = pinnedVerify(der)
	} else {
		// Bypass CA-chain/hostname validation for the self-derived cert, but pin
		// the server's leaf certificate to the shared-secret-derived one.
		cfg.InsecureSkipVerify = true
		cfg.ServerName = "paqetpremium"
		cfg.VerifyPeerCertificate = pinnedVerify(der)
	}
	return cfg, nil
}
