package x32

import (
	"fmt"
	"net"
	"time"

	"X32-personal-mix/internal/osc"
)

const ChannelCount = 32

// Channel is one input channel's send into a mix bus (submix).
type Channel struct {
	Number    int     `json:"number"`
	Name      string  `json:"name"`
	Level     float32 `json:"level"` // 0.0 … 1.0
	Pan       float32 `json:"pan"`   // 0.0 = L … 0.5 = C … 1.0 = R
	On        bool    `json:"on"`
	Icon     int    `json:"icon"`
	IconSrc  string `json:"iconSrc"`
	Color    int    `json:"color"`
	ColorCSS string `json:"colorCss"`
}

// FetchSubmix loads channel names, icons and send level/mute/pan for the bus.
func FetchSubmix(client *osc.Client, ip string, port, bus int, perQuery time.Duration) ([]Channel, error) {
	addr, err := udpAddr(ip, port)
	if err != nil {
		return nil, err
	}
	if bus < 1 || bus > BusCount {
		return nil, fmt.Errorf("invalid bus %d", bus)
	}

	out := make([]Channel, 0, ChannelCount)
	for ch := 1; ch <= ChannelCount; ch++ {
		namePath := fmt.Sprintf("/ch/%02d/config/name", ch)
		iconPath := fmt.Sprintf("/ch/%02d/config/icon", ch)
		colorPath := fmt.Sprintf("/ch/%02d/config/color", ch)
		levelPath := fmt.Sprintf("/ch/%02d/mix/%02d/level", ch, bus)
		onPath := fmt.Sprintf("/ch/%02d/mix/%02d/on", ch, bus)
		panPath := fmt.Sprintf("/ch/%02d/mix/%02d/pan", ch, bus)

		nameMsg, err := client.Request(addr, namePath, perQuery)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", namePath, err)
		}
		iconMsg, err := client.Request(addr, iconPath, perQuery)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", iconPath, err)
		}
		colorMsg, err := client.Request(addr, colorPath, perQuery)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", colorPath, err)
		}
		levelMsg, err := client.Request(addr, levelPath, perQuery)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", levelPath, err)
		}
		onMsg, err := client.Request(addr, onPath, perQuery)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", onPath, err)
		}
		panMsg, err := client.Request(addr, panPath, perQuery)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", panPath, err)
		}

		name := stringArg(nameMsg, 0)
		if name == "" {
			name = fmt.Sprintf("Ch %d", ch)
		}
		icon := intArg(iconMsg, 0)
		if icon < 1 {
			icon = 1
		}
		color := intArg(colorMsg, 0)
		out = append(out, Channel{
			Number:    ch,
			Name:      name,
			Level:     floatArg(levelMsg, 0),
			Pan:       floatArg(panMsg, 0),
			On:        boolArg(onMsg, 0),
			Icon:     icon,
			IconSrc:  IconSrc(icon),
			Color:    color,
			ColorCSS: ColorCSS(color),
		})
	}
	return out, nil
}

// SetSendLevel sets /ch/XX/mix/BB/level.
func SetSendLevel(client *osc.Client, ip string, port, ch, bus int, level float32) error {
	addr, err := udpAddr(ip, port)
	if err != nil {
		return err
	}
	level = clamp01(level)
	path := fmt.Sprintf("/ch/%02d/mix/%02d/level", ch, bus)
	return client.SendMsg(addr, osc.Message{Address: path, Arguments: []any{level}})
}

// SetSendPan sets /ch/XX/mix/BB/pan (0…1, center 0.5).
func SetSendPan(client *osc.Client, ip string, port, ch, bus int, pan float32) error {
	addr, err := udpAddr(ip, port)
	if err != nil {
		return err
	}
	pan = clamp01(pan)
	path := fmt.Sprintf("/ch/%02d/mix/%02d/pan", ch, bus)
	return client.SendMsg(addr, osc.Message{Address: path, Arguments: []any{pan}})
}

// SetSendOn sets /ch/XX/mix/BB/on (1=ON, 0=OFF).
func SetSendOn(client *osc.Client, ip string, port, ch, bus int, on bool) error {
	addr, err := udpAddr(ip, port)
	if err != nil {
		return err
	}
	var v int32
	if on {
		v = 1
	}
	path := fmt.Sprintf("/ch/%02d/mix/%02d/on", ch, bus)
	return client.SendMsg(addr, osc.Message{Address: path, Arguments: []any{v}})
}

// StartXremote renews /xremote on dest every interval until stop is closed.
func StartXremote(client *osc.Client, dest *net.UDPAddr, interval time.Duration, stop <-chan struct{}) {
	send := func() {
		_ = client.SendMsg(dest, osc.Message{Address: "/xremote"})
	}
	send()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			send()
		}
	}
}

func udpAddr(host string, port int) (*net.UDPAddr, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return nil, fmt.Errorf("resolve %s:%d: %w", host, port, err)
	}
	return addr, nil
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func stringArg(msg osc.Message, i int) string {
	if i >= len(msg.Arguments) {
		return ""
	}
	s, _ := msg.Arguments[i].(string)
	return s
}

func floatArg(msg osc.Message, i int) float32 {
	if i >= len(msg.Arguments) {
		return 0
	}
	switch v := msg.Arguments[i].(type) {
	case float32:
		return v
	case float64:
		return float32(v)
	case int32:
		return float32(v)
	default:
		return 0
	}
}

func intArg(msg osc.Message, i int) int {
	if i >= len(msg.Arguments) {
		return 0
	}
	switch v := msg.Arguments[i].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func boolArg(msg osc.Message, i int) bool {
	if i >= len(msg.Arguments) {
		return false
	}
	switch v := msg.Arguments[i].(type) {
	case int32:
		return v != 0
	case string:
		return v == "ON" || v == "on" || v == "1"
	case float32:
		return v != 0
	default:
		return false
	}
}
