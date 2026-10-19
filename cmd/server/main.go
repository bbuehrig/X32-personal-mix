package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"X32-personal-mix/internal/api"
	"X32-personal-mix/internal/hub"
	"X32-personal-mix/internal/osc"
	"X32-personal-mix/internal/tlscert"
	"X32-personal-mix/web"
)

func main() {
	mixerFlag := flag.String("mixer", "", "Mixer address host[:port] – skips discovery")
	httpsFlag := flag.String("https", ":8443", "HTTPS listen address (default :8443). Disable: -https=off")
	httpFlag := flag.String("http", ":8080", "HTTP listen address – always redirects to HTTPS (default :8080)")
	tlsDirFlag := flag.String("tls-dir", "", "Directory for self-signed certificate (default: .x32-personal-mix-tls)")
	domainFlag := flag.String("domain", "", "Hostname for certificate/URL (e.g. mixer.tcy) – optional")
	flag.Parse()

	httpsAddr := strings.TrimSpace(*httpsFlag)
	if v := strings.TrimSpace(os.Getenv("HTTPS")); v != "" {
		httpsAddr = v
	}
	if httpsAddr == "off" || httpsAddr == "0" || httpsAddr == "false" {
		httpsAddr = ""
	}

	httpAddr := strings.TrimSpace(*httpFlag)
	if v := strings.TrimSpace(os.Getenv("ADDR")); v != "" {
		httpAddr = v
	}
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	tlsDir := strings.TrimSpace(*tlsDirFlag)
	if tlsDir == "" {
		tlsDir = strings.TrimSpace(os.Getenv("TLS_DIR"))
	}
	if tlsDir == "" {
		tlsDir = ".x32-personal-mix-tls"
	}
	if !filepath.IsAbs(tlsDir) {
		if wd, err := os.Getwd(); err == nil {
			tlsDir = filepath.Join(wd, tlsDir)
		}
	}

	domain := strings.TrimSpace(*domainFlag)
	if domain == "" {
		domain = strings.TrimSpace(os.Getenv("DOMAIN"))
	}

	mixerAddr := strings.TrimSpace(*mixerFlag)
	if mixerAddr == "" {
		mixerAddr = strings.TrimSpace(os.Getenv("MIXER"))
	}

	var fixed *api.MixerAddr
	if mixerAddr != "" {
		host, port, err := parseMixerAddr(mixerAddr)
		if err != nil {
			log.Fatalf("mixer address: %v", err)
		}
		fixed = &api.MixerAddr{IP: host, Port: port}
	}

	oscClient, err := osc.NewClient()
	if err != nil {
		log.Fatalf("OSC client: %v", err)
	}
	defer oscClient.Close()

	h := hub.New()
	srvAPI := api.New(h, oscClient, web.FS, fixed)
	appHandler := srvAPI.Handler()

	var httpsServer *http.Server
	var httpServer *http.Server

	lanIPs := localIPv4s()

	if httpsAddr == "" {
		httpServer = &http.Server{
			Addr:              httpAddr,
			Handler:           appHandler,
			ReadHeaderTimeout: 5 * time.Second,
		}
		httpPort := portFromAddr(httpAddr)
		log.Print("X32 Personal Mix HTTP (HTTPS off)")
		logListenURLs("http", httpPort, domain, lanIPs)
		if fixed != nil {
			log.Printf("Mixer %s:%d", fixed.IP, fixed.Port)
		} else {
			log.Print("Mixer: auto-discovery")
		}
		go serveHTTP(httpServer)
	} else {
		var domains []string
		if domain != "" {
			domains = []string{domain}
		}
		certPath, keyPath, err := tlscert.EnsureFiles(tlsDir, domains...)
		if err != nil {
			log.Fatalf("tls cert: %v", err)
		}
		log.Printf("TLS certificate ready: %s", certPath)
		if domain != "" {
			log.Printf("Certificate DNS: %s (+ localhost + LAN IPs)", domain)
		}

		httpsServer = &http.Server{
			Addr:              httpsAddr,
			Handler:           appHandler,
			ReadHeaderTimeout: 5 * time.Second,
			// Self-signed + aborted probes spam "TLS handshake error"; keep real faults.
			ErrorLog: log.New(&tlsNoiseFilter{out: os.Stderr}, "", 0),
		}
		httpsPort := portFromAddr(httpsAddr)
		go func() {
			log.Print("X32 Personal Mix HTTPS")
			logListenURLs("https", httpsPort, domain, lanIPs)
			if fixed != nil {
				log.Printf("Mixer %s:%d", fixed.IP, fixed.Port)
			} else {
				log.Print("Mixer: auto-discovery")
			}
			log.Print("Self-signed – accept the certificate once on the device (no Let's Encrypt for pure LAN names).")
			if err := httpsServer.ListenAndServeTLS(certPath, keyPath); err != nil && err != http.ErrServerClosed {
				log.Fatalf("https: %v", err)
			}
		}()

		httpServer = &http.Server{
			Addr:              httpAddr,
			Handler:           httpsRedirectHandler(httpsPort),
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			httpPort := portFromAddr(httpAddr)
			log.Printf("HTTP redirect :%s → HTTPS :%s", httpPort, httpsPort)
			for _, host := range listenHosts(domain, lanIPs) {
				log.Printf("  http://%s → https://%s", displayHostPort(host, httpPort), displayHostPort(host, httpsPort))
			}
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("http redirect: %v (HTTPS still running)", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if httpServer != nil {
		_ = httpServer.Shutdown(ctx)
	}
	if httpsServer != nil {
		_ = httpsServer.Shutdown(ctx)
	}
}

// tlsNoiseFilter drops expected self-signed / probe handshake noise from net/http.
type tlsNoiseFilter struct{ out *os.File }

func (f *tlsNoiseFilter) Write(p []byte) (int, error) {
	s := string(p)
	if strings.Contains(s, "TLS handshake error") {
		switch {
		case strings.Contains(s, "unknown certificate"),
			strings.Contains(s, "certificate unknown"),
			strings.Contains(s, "EOF"),
			strings.Contains(s, "connection reset by peer"),
			strings.Contains(s, "client didn't provide a certificate"),
			strings.Contains(s, "first record does not look like a TLS handshake"):
			return len(p), nil
		}
	}
	return f.out.Write(p)
}

func serveHTTP(srv *http.Server) {
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http: %v", err)
	}
}

// localIPv4s returns non-loopback IPv4 addresses on up interfaces (LAN reachability).
func localIPv4s() []string {
	seen := map[string]bool{}
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			s := ip.String()
			if seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func listenHosts(domain string, lanIPs []string) []string {
	hosts := []string{"localhost"}
	hosts = append(hosts, lanIPs...)
	if domain != "" {
		hosts = append(hosts, domain)
	}
	return hosts
}

func logListenURLs(scheme, port, domain string, lanIPs []string) {
	for _, host := range listenHosts(domain, lanIPs) {
		log.Printf("  %s://%s", scheme, displayHostPort(host, port))
	}
	if len(lanIPs) == 0 {
		log.Print("  (no LAN IPv4 found – check network interfaces)")
	}
}

func httpsRedirectHandler(httpsPort string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		var target string
		if httpsPort == "443" {
			target = "https://" + host + r.URL.RequestURI()
		} else {
			target = "https://" + net.JoinHostPort(host, httpsPort) + r.URL.RequestURI()
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

func portFromAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "443"
	}
	if port == "" {
		return "443"
	}
	return port
}

func displayHostPort(host, port string) string {
	if port == "" || port == "443" {
		return host
	}
	return net.JoinHostPort(host, port)
}

func parseMixerAddr(s string) (string, int, error) {
	if host, portStr, err := net.SplitHostPort(s); err == nil {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return "", 0, fmt.Errorf("invalid port %q", portStr)
		}
		host = strings.TrimSpace(host)
		if host == "" {
			return "", 0, fmt.Errorf("empty host")
		}
		return host, port, nil
	}

	host := strings.TrimSpace(s)
	if host == "" {
		return "", 0, fmt.Errorf("empty address")
	}
	if strings.Contains(host, ":") && net.ParseIP(host) == nil {
		return "", 0, fmt.Errorf("invalid address %q (use host:port or [ipv6]:port)", s)
	}
	return host, osc.DefaultPort, nil
}
