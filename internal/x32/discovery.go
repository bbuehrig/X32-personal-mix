package x32

import (
	"fmt"
	"net"
	"time"

	"X32-personal-mix/internal/osc"
)

// Mixer describes a discovered X32/M32 console.
type Mixer struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Name     string `json:"name"`
	Model    string `json:"model"`
	Firmware string `json:"firmware"`
}

// Discover broadcasts /xinfo and collects replies until timeout.
func Discover(client *osc.Client, timeout time.Duration) ([]Mixer, error) {
	items, err := client.Collect(timeout, func(msg osc.Message) bool {
		return msg.Address == "/xinfo"
	}, func() error {
		if err := client.EnableBroadcast(); err != nil {
			return fmt.Errorf("enable broadcast: %w", err)
		}
		dest := &net.UDPAddr{IP: net.IPv4bcast, Port: osc.DefaultPort}
		if err := client.Send(dest, osc.Message{Address: "/xinfo"}); err != nil {
			return fmt.Errorf("send /xinfo: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var mixers []Mixer
	for _, item := range items {
		m, ok := parseXinfo(item.Msg, item.From, osc.DefaultPort)
		if !ok {
			continue
		}
		if _, exists := seen[m.IP]; exists {
			continue
		}
		seen[m.IP] = struct{}{}
		mixers = append(mixers, m)
	}
	return mixers, nil
}

// QueryXinfo asks a known mixer for /xinfo on the given UDP port.
func QueryXinfo(client *osc.Client, host string, port int, timeout time.Duration) (Mixer, error) {
	addr, err := udpAddr(host, port)
	if err != nil {
		return Mixer{}, err
	}

	msg, err := client.Request(addr, "/xinfo", timeout)
	if err != nil {
		return Mixer{}, fmt.Errorf("no /xinfo reply from %s:%d: %w", host, port, err)
	}
	m, ok := parseXinfo(msg, addr, port)
	if !ok {
		return Mixer{}, fmt.Errorf("invalid /xinfo reply from %s:%d", host, port)
	}
	if m.IP == "" {
		m.IP = host
	}
	return m, nil
}

func parseXinfo(msg osc.Message, from *net.UDPAddr, port int) (Mixer, bool) {
	if len(msg.Arguments) < 4 {
		return Mixer{}, false
	}
	ip, ok1 := msg.Arguments[0].(string)
	name, ok2 := msg.Arguments[1].(string)
	model, ok3 := msg.Arguments[2].(string)
	fw, ok4 := msg.Arguments[3].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return Mixer{}, false
	}
	if ip == "" && from != nil {
		ip = from.IP.String()
	}
	return Mixer{
		IP:       ip,
		Port:     port,
		Name:     name,
		Model:    model,
		Firmware: fw,
	}, true
}
