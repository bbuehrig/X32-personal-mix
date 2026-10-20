package x32

import (
	"fmt"
	"time"

	"X32-personal-mix/internal/osc"
)

// BusMaster is the selected mix-bus output strip (master for that submix).
type BusMaster struct {
	Number   int     `json:"number"`
	Name     string  `json:"name"`
	Level    float32 `json:"level"`
	Pan      float32 `json:"pan"`
	On       bool    `json:"on"`
	Color    int     `json:"color"`
	ColorCSS string  `json:"colorCss"`
}

// FetchBusMaster loads fader/mute/pan/color for mix bus n (1–16).
func FetchBusMaster(client *osc.Client, ip string, port, bus int, name string, perQuery time.Duration) (*BusMaster, error) {
	addr, err := udpAddr(ip, port)
	if err != nil {
		return nil, err
	}
	if bus < 1 || bus > BusCount {
		return nil, fmt.Errorf("invalid bus %d", bus)
	}

	faderPath := fmt.Sprintf("/bus/%02d/mix/fader", bus)
	onPath := fmt.Sprintf("/bus/%02d/mix/on", bus)
	panPath := fmt.Sprintf("/bus/%02d/mix/pan", bus)
	colorPath := fmt.Sprintf("/bus/%02d/config/color", bus)

	faderMsg, err := client.Request(addr, faderPath, perQuery)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", faderPath, err)
	}
	onMsg, err := client.Request(addr, onPath, perQuery)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", onPath, err)
	}
	panMsg, err := client.Request(addr, panPath, perQuery)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", panPath, err)
	}
	colorMsg, err := client.Request(addr, colorPath, perQuery)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", colorPath, err)
	}

	if name == "" {
		name = fmt.Sprintf("Bus %d", bus)
	}
	color := intArg(colorMsg, 0)
	return &BusMaster{
		Number:   bus,
		Name:     name,
		Level:    floatArg(faderMsg, 0),
		Pan:      floatArg(panMsg, 0),
		On:       boolArg(onMsg, 0),
		Color:    color,
		ColorCSS: ColorCSS(color),
	}, nil
}

// SetBusFader sets /bus/XX/mix/fader.
func SetBusFader(client *osc.Client, ip string, port, bus int, level float32) error {
	addr, err := udpAddr(ip, port)
	if err != nil {
		return err
	}
	level = clamp01(level)
	path := fmt.Sprintf("/bus/%02d/mix/fader", bus)
	return client.SendMsg(addr, osc.Message{Address: path, Arguments: []any{level}})
}

// SetBusPan sets /bus/XX/mix/pan.
func SetBusPan(client *osc.Client, ip string, port, bus int, pan float32) error {
	addr, err := udpAddr(ip, port)
	if err != nil {
		return err
	}
	pan = clamp01(pan)
	path := fmt.Sprintf("/bus/%02d/mix/pan", bus)
	return client.SendMsg(addr, osc.Message{Address: path, Arguments: []any{pan}})
}

// SetBusOn sets /bus/XX/mix/on.
func SetBusOn(client *osc.Client, ip string, port, bus int, on bool) error {
	addr, err := udpAddr(ip, port)
	if err != nil {
		return err
	}
	var v int32
	if on {
		v = 1
	}
	path := fmt.Sprintf("/bus/%02d/mix/on", bus)
	return client.SendMsg(addr, osc.Message{Address: path, Arguments: []any{v}})
}

// BusMeterIndex returns the /meters/0 float index for mix bus n (1–16), or -1.
// Layout: 32 ch + 8 aux + 8 FX returns + 16 buses.
func BusMeterIndex(bus int) int {
	if bus < 1 || bus > BusCount {
		return -1
	}
	return 32 + 8 + 8 + (bus - 1)
}
