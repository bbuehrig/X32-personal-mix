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
	Number   int    `json:"number"`
	Name     string `json:"name"`
	Icon     int    `json:"icon"`
	IconSrc  string `json:"iconSrc"`
	Color    int    `json:"color"`
	ColorCSS string `json:"colorCss"`
}

// FetchStereoBuses reads name/icon/color for odd buses (1,3,…,15).
func FetchStereoBuses(client *osc.Client, ip string, port int, perBusTimeout time.Duration) ([]Bus, error) {
	addr := &net.UDPAddr{IP: net.ParseIP(ip), Port: port}
	if addr.IP == nil {
		return nil, fmt.Errorf("invalid ip %q", ip)
	}
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}

	out := make([]Bus, 0, BusCount/2)
	for i := 1; i <= BusCount; i += 2 {
		nameMsg, err := client.Request(addr, busNamePath(i), perBusTimeout)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", busNamePath(i), err)
		}
		iconMsg, err := client.Request(addr, busIconPath(i), perBusTimeout)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", busIconPath(i), err)
		}
		colorMsg, err := client.Request(addr, busColorPath(i), perBusTimeout)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", busColorPath(i), err)
		}

		name := stringArg(nameMsg, 0)
		if name == "" {
			name = fmt.Sprintf("Bus %d", i)
		}
		icon := intArg(iconMsg, 0)
		if icon < 1 {
			icon = 1
		}
		color := intArg(colorMsg, 0)
		out = append(out, Bus{
			Number:   i,
			Name:     name,
			Icon:     icon,
			IconSrc:  IconSrc(icon),
			Color:    color,
			ColorCSS: ColorCSS(color),
		})
	}
	return out, nil
}

func busNamePath(n int) string {
	return fmt.Sprintf("/bus/%02d/config/name", n)
}

func busIconPath(n int) string {
	return fmt.Sprintf("/bus/%02d/config/icon", n)
}

func busColorPath(n int) string {
	return fmt.Sprintf("/bus/%02d/config/color", n)
}
