package tlsutil

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrGenerateCertPersists(t *testing.T) {
	dir := t.TempDir()

	cert1, err := LoadOrGenerateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrGenerateCert (first call): %v", err)
	}
	if len(cert1.Certificate) == 0 {
		t.Fatal("expected a non-empty certificate chain")
	}

	// A second call must load the same persisted key pair, not generate a
	// new one - the fingerprint the controller pins on first connect would
	// otherwise stop matching after every target restart.
	cert2, err := LoadOrGenerateCert(dir)
	if err != nil {
		t.Fatalf("LoadOrGenerateCert (second call): %v", err)
	}
	if Fingerprint(cert1.Certificate[0]) != Fingerprint(cert2.Certificate[0]) {
		t.Error("second LoadOrGenerateCert produced a different certificate than the first")
	}
}

func TestFingerprintFormat(t *testing.T) {
	der := []byte("not a real certificate, just some bytes to hash")
	fp := Fingerprint(der)

	parts := strings.Split(fp, ":")
	if len(parts) != 32 { // sha256 = 32 bytes
		t.Fatalf("Fingerprint produced %d colon-separated parts, want 32", len(parts))
	}
	for _, p := range parts {
		if len(p) != 2 {
			t.Errorf("fingerprint part %q is not 2 hex chars", p)
		}
	}

	if got := Fingerprint(der); got != fp {
		t.Error("Fingerprint is not deterministic for the same input")
	}
	if got := Fingerprint([]byte("different bytes")); got == fp {
		t.Error("Fingerprint produced the same output for different input")
	}
}

func TestKnownHostsTrustAndLookup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts.json")

	kh, err := OpenKnownHosts(path)
	if err != nil {
		t.Fatalf("OpenKnownHosts: %v", err)
	}
	if _, ok := kh.Lookup("192.168.1.16:7777"); ok {
		t.Fatal("Lookup on a fresh store should find nothing")
	}

	if err := kh.Trust("192.168.1.16:7777", "AA:BB:CC"); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	got, ok := kh.Lookup("192.168.1.16:7777")
	if !ok || got != "AA:BB:CC" {
		t.Fatalf("Lookup after Trust = (%q, %v), want (\"AA:BB:CC\", true)", got, ok)
	}

	// Trust must persist to disk: a fresh KnownHosts opened from the same
	// path should see it too.
	kh2, err := OpenKnownHosts(path)
	if err != nil {
		t.Fatalf("OpenKnownHosts (reopen): %v", err)
	}
	got, ok = kh2.Lookup("192.168.1.16:7777")
	if !ok || got != "AA:BB:CC" {
		t.Fatalf("Lookup after reopening = (%q, %v), want (\"AA:BB:CC\", true)", got, ok)
	}
}

func TestKnownHostsMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist", "known_hosts.json")
	kh, err := OpenKnownHosts(path)
	if err != nil {
		t.Fatalf("OpenKnownHosts on a missing file: %v", err)
	}
	if _, ok := kh.Lookup("anything"); ok {
		t.Fatal("Lookup on a store backed by a missing file should find nothing")
	}
}
