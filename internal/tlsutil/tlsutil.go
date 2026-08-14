// Package tlsutil handles the self-signed certificate used to encrypt the
// controller<->target connection, plus a trust-on-first-use (TOFU) store so
// the controller can pin a target's certificate fingerprint across runs,
// the same way SSH pins host keys.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LoadOrGenerateCert returns a TLS certificate persisted under dir,
// generating a new self-signed ECDSA P-256 certificate on first use.
func LoadOrGenerateCert(dir string) (tls.Certificate, error) {
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		return cert, nil
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return tls.Certificate{}, fmt.Errorf("tlsutil: creating %s: %w", dir, err)
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "kbs-reverse-kvm"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// Fingerprint returns a human-readable SHA-256 fingerprint of a DER
// certificate, formatted as colon-separated hex pairs (à la SSH/TLS
// tooling), e.g. "AB:CD:EF:...".
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// KnownHosts is a simple JSON-backed trust store mapping "host:port" to a
// pinned certificate fingerprint, analogous to ~/.ssh/known_hosts.
type KnownHosts struct {
	path    string
	entries map[string]string
}

// OpenKnownHosts loads (or initializes) the trust store at path.
func OpenKnownHosts(path string) (*KnownHosts, error) {
	kh := &KnownHosts{path: path, entries: map[string]string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return kh, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return kh, nil
	}
	if err := json.Unmarshal(data, &kh.entries); err != nil {
		return nil, err
	}
	return kh, nil
}

// Path returns the filesystem path backing this trust store.
func (kh *KnownHosts) Path() string {
	return kh.path
}

// Lookup returns the pinned fingerprint for addr, if any.
func (kh *KnownHosts) Lookup(addr string) (fingerprint string, ok bool) {
	fp, ok := kh.entries[addr]
	return fp, ok
}

// Trust pins fingerprint for addr and persists the store to disk.
func (kh *KnownHosts) Trust(addr, fingerprint string) error {
	kh.entries[addr] = fingerprint
	if dir := filepath.Dir(kh.path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(kh.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(kh.path, data, 0600)
}
