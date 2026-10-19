package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnsureFiles creates a self-signed cert/key pair if missing (or if required
// DNS names are not covered) and returns their paths.
func EnsureFiles(dir string, domains ...string) (certPath, keyPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	need := normalizeDomains(domains)
	if fileOK(certPath) && fileOK(keyPath) && certCovers(certPath, need) {
		return certPath, keyPath, nil
	}

	certPEM, keyPEM, err := generate(need)
	if err != nil {
		return "", "", fmt.Errorf("generate: %w", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return "", "", fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", "", fmt.Errorf("write key: %w", err)
	}
	return certPath, keyPath, nil
}

func fileOK(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() > 0
}

func normalizeDomains(domains []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimSuffix(d, ".")
		if d == "" || seen[d] {
			continue
		}
		// Strip accidental scheme / port / path from user input.
		if strings.Contains(d, "://") {
			if u, err := parseLooseHost(d); err == nil && u != "" {
				d = u
			}
		}
		if host, _, err := net.SplitHostPort(d); err == nil {
			d = host
		}
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || seen[d] || net.ParseIP(d) != nil {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

func parseLooseHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
	}
	if i := strings.IndexAny(raw, "/?"); i >= 0 {
		raw = raw[:i]
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return strings.ToLower(host), nil
	}
	return strings.ToLower(strings.TrimSpace(raw)), nil
}

func certCovers(certPath string, domains []string) bool {
	if len(domains) == 0 {
		return true
	}
	raw, err := os.ReadFile(certPath)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	for _, d := range domains {
		if err := cert.VerifyHostname(d); err != nil {
			return false
		}
	}
	return true
}

func generate(domains []string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, nil, err
	}

	hosts := []string{"localhost"}
	seenHost := map[string]bool{"localhost": true}
	for _, d := range domains {
		if !seenHost[d] {
			seenHost[d] = true
			hosts = append(hosts, d)
		}
	}

	cn := "X32 Personal Mix LAN"
	if len(domains) > 0 {
		cn = domains[0]
	}

	seen := map[string]bool{"127.0.0.1": true, "::1": true}
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				ip = v4
			}
			k := ip.String()
			if seen[k] {
				continue
			}
			seen[k] = true
			ips = append(ips, ip)
		}
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"X32 Personal Mix"},
			CommonName:   cn,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              hosts,
		IPAddresses:           ips,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM, nil
}
