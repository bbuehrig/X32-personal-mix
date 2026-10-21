package x32

import (
	"fmt"
	"regexp"
	"strconv"

	"X32-personal-mix/internal/osc"
)

var (
	reMixLevel = regexp.MustCompile(`^/ch/(\d{1,2})/mix/(\d{1,2})/level$`)
	reMixOn    = regexp.MustCompile(`^/ch/(\d{1,2})/mix/(\d{1,2})/on$`)
	reMixPan   = regexp.MustCompile(`^/ch/(\d{1,2})/mix/(\d{1,2})/pan$`)
	reBusFader = regexp.MustCompile(`^/bus/(\d{1,2})/mix/fader$`)
	reBusOn    = regexp.MustCompile(`^/bus/(\d{1,2})/mix/on$`)
	reBusPan   = regexp.MustCompile(`^/bus/(\d{1,2})/mix/pan$`)
	reBusName  = regexp.MustCompile(`^/bus/(\d{1,2})/config/name$`)
	reBusIcon  = regexp.MustCompile(`^/bus/(\d{1,2})/config/icon$`)
	reBusColor = regexp.MustCompile(`^/bus/(\d{1,2})/config/color$`)
	reChName   = regexp.MustCompile(`^/ch/(\d{1,2})/config/name$`)
	reChIcon   = regexp.MustCompile(`^/ch/(\d{1,2})/config/icon$`)
	reChColor  = regexp.MustCompile(`^/ch/(\d{1,2})/config/color$`)
)

// SubmixUpdate is a parsed live change relevant to a submix view.
// Channel 0 means the bus master strip (Bus set).
type SubmixUpdate struct {
	Channel int
	Bus     int    // 0 if channel-global (name/icon)
	Kind    string // level | on | pan | name | icon | color
	Level   float32
	Pan     float32
	On      bool
	Name    string
	Icon    int
	Color   int
}

// ParseSubmixUpdate extracts a submix-relevant change from an OSC message.
func ParseSubmixUpdate(msg osc.Message) (SubmixUpdate, bool) {
	if m := reMixLevel.FindStringSubmatch(msg.Address); m != nil {
		return SubmixUpdate{
			Channel: atoi(m[1]),
			Bus:     atoi(m[2]),
			Kind:    "level",
			Level:   floatArg(msg, 0),
		}, true
	}
	if m := reMixOn.FindStringSubmatch(msg.Address); m != nil {
		return SubmixUpdate{
			Channel: atoi(m[1]),
			Bus:     atoi(m[2]),
			Kind:    "on",
			On:      boolArg(msg, 0),
		}, true
	}
	if m := reMixPan.FindStringSubmatch(msg.Address); m != nil {
		return SubmixUpdate{
			Channel: atoi(m[1]),
			Bus:     atoi(m[2]),
			Kind:    "pan",
			Pan:     floatArg(msg, 0),
		}, true
	}
	if m := reBusFader.FindStringSubmatch(msg.Address); m != nil {
		return SubmixUpdate{
			Channel: 0,
			Bus:     atoi(m[1]),
			Kind:    "level",
			Level:   floatArg(msg, 0),
		}, true
	}
	if m := reBusOn.FindStringSubmatch(msg.Address); m != nil {
		return SubmixUpdate{
			Channel: 0,
			Bus:     atoi(m[1]),
			Kind:    "on",
			On:      boolArg(msg, 0),
		}, true
	}
	if m := reBusPan.FindStringSubmatch(msg.Address); m != nil {
		return SubmixUpdate{
			Channel: 0,
			Bus:     atoi(m[1]),
			Kind:    "pan",
			Pan:     floatArg(msg, 0),
		}, true
	}
	if m := reBusName.FindStringSubmatch(msg.Address); m != nil {
		name := stringArg(msg, 0)
		if name == "" {
			name = fmt.Sprintf("Bus %d", atoi(m[1]))
		}
		return SubmixUpdate{
			Channel: 0,
			Bus:     atoi(m[1]),
			Kind:    "name",
			Name:    name,
		}, true
	}
	if m := reBusIcon.FindStringSubmatch(msg.Address); m != nil {
		icon := intArg(msg, 0)
		if icon < 1 {
			icon = 1
		}
		return SubmixUpdate{
			Channel: 0,
			Bus:     atoi(m[1]),
			Kind:    "icon",
			Icon:    icon,
		}, true
	}
	if m := reBusColor.FindStringSubmatch(msg.Address); m != nil {
		return SubmixUpdate{
			Channel: 0,
			Bus:     atoi(m[1]),
			Kind:    "color",
			Color:   intArg(msg, 0),
		}, true
	}
	if m := reChName.FindStringSubmatch(msg.Address); m != nil {
		name := stringArg(msg, 0)
		if name == "" {
			name = fmt.Sprintf("Ch %d", atoi(m[1]))
		}
		return SubmixUpdate{
			Channel: atoi(m[1]),
			Kind:    "name",
			Name:    name,
		}, true
	}
	if m := reChIcon.FindStringSubmatch(msg.Address); m != nil {
		icon := intArg(msg, 0)
		if icon < 1 {
			icon = 1
		}
		return SubmixUpdate{
			Channel: atoi(m[1]),
			Kind:    "icon",
			Icon:    icon,
		}, true
	}
	if m := reChColor.FindStringSubmatch(msg.Address); m != nil {
		return SubmixUpdate{
			Channel: atoi(m[1]),
			Kind:    "color",
			Color:   intArg(msg, 0),
		}, true
	}
	return SubmixUpdate{}, false
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
