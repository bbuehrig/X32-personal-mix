package tlscert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureFiles(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, err := EnsureFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		t.Fatalf("load pair: %v", err)
	}
	// second call reuses
	c2, k2, err := EnsureFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c2 != certPath || k2 != keyPath {
		t.Fatalf("paths changed")
	}
	if filepath.Dir(certPath) != dir {
		t.Fatalf("unexpected dir %s", certPath)
	}
}

func TestEnsureFilesDomain(t *testing.T) {
	dir := t.TempDir()
	certPath, _, err := EnsureFiles(dir, "mixer.tcy")
	if err != nil {
		t.Fatal(err)
	}
	cert := mustParseCert(t, certPath)
	if err := cert.VerifyHostname("mixer.tcy"); err != nil {
		t.Fatalf("hostname mixer.tcy: %v", err)
	}
	if err := cert.VerifyHostname("localhost"); err != nil {
		t.Fatalf("hostname localhost: %v", err)
	}

	// reuse when domain already covered
	before, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureFiles(dir, "mixer.tcy"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("cert regenerated unexpectedly")
	}

	// regenerate when a new domain is required
	if _, _, err := EnsureFiles(dir, "mixer.tcy", "mix.local"); err != nil {
		t.Fatal(err)
	}
	cert2 := mustParseCert(t, certPath)
	if err := cert2.VerifyHostname("mix.local"); err != nil {
		t.Fatalf("after regenerate mix.local: %v", err)
	}
}

func TestNormalizeDomains(t *testing.T) {
	got := normalizeDomains([]string{" Mixer.TCY ", "https://mixer.tcy:8443/", "192.168.1.1", ""})
	if len(got) != 1 || got[0] != "mixer.tcy" {
		t.Fatalf("got %#v", got)
	}
}

func mustParseCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatal("no PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
