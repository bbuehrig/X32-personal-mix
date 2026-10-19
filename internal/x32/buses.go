package x32

import (
	"fmt"
	"net"
	"time"

	"X32-personal-mix/internal/osc"
)

const BusCount = 16

// Bus is a mix bus as shown in the UI (odd numbers for stereo pairs).
type Bus struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

// FetchStereoBuses reads names for buses 01–16 and returns the odd-numbered
// buses (1,3,…,15), assuming pairs are stereo-linked.
func FetchStereoBuses(client *osc.Client, ip string, port int, perBusTimeout time.Duration) ([]Bus, error) {
	addr := &net.UDPAddr{IP: net.ParseIP(ip), Port: port}
	if addr.IP == nil {
		return nil, fmt.Errorf("invalid ip %q", ip)
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}

	names := make([]string, BusCount)
	for i := 1; i <= BusCount; i++ {
		path := busNamePath(i)
		msg, err := client.Request(addr, path, perBusTimeout)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		name := ""
		if len(msg.Arguments) > 0 {
			if s, ok := msg.Arguments[0].(string); ok {
				name = s
			}
		}
		names[i-1] = name
	}

	out := make([]Bus, 0, BusCount/2)
	for i := 1; i <= BusCount; i += 2 {
		name := names[i-1]
		if name == "" {
			name = fmt.Sprintf("Bus %d", i)
		}
		out = append(out, Bus{Number: i, Name: name})
	}
	return out, nil
}

func busNamePath(n int) string {
	return fmt.Sprintf("/bus/%02d/config/name", n)
}
