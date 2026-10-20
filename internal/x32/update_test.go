package x32

import (
	"testing"

	"X32-personal-mix/internal/osc"
)

func TestParseSubmixUpdateLevel(t *testing.T) {
	upd, ok := ParseSubmixUpdate(osc.Message{
		Address:   "/ch/03/mix/01/level",
		Arguments: []any{float32(0.6)},
	})
	if !ok || upd.Channel != 3 || upd.Bus != 1 || upd.Kind != "level" || upd.Level != 0.6 {
		t.Fatalf("unexpected: %+v ok=%v", upd, ok)
	}
}

func TestParseSubmixUpdatePan(t *testing.T) {
	upd, ok := ParseSubmixUpdate(osc.Message{
		Address:   "/ch/08/mix/05/pan",
		Arguments: []any{float32(0.25)},
	})
	if !ok || upd.Kind != "pan" || upd.Channel != 8 || upd.Bus != 5 {
		t.Fatalf("unexpected: %+v", upd)
	}
}

func TestParseSubmixUpdateUnpadded(t *testing.T) {
	upd, ok := ParseSubmixUpdate(osc.Message{
		Address:   "/ch/1/mix/1/level",
		Arguments: []any{float32(0.4)},
	})
	if !ok || upd.Channel != 1 || upd.Bus != 1 || upd.Level != 0.4 {
		t.Fatalf("unexpected: %+v ok=%v", upd, ok)
	}
}

func TestParseSubmixUpdateBusMaster(t *testing.T) {
	upd, ok := ParseSubmixUpdate(osc.Message{
		Address:   "/bus/05/mix/fader",
		Arguments: []any{float32(0.75)},
	})
	if !ok || upd.Channel != 0 || upd.Bus != 5 || upd.Kind != "level" || upd.Level != 0.75 {
		t.Fatalf("bus fader: %+v ok=%v", upd, ok)
	}
	upd, ok = ParseSubmixUpdate(osc.Message{
		Address:   "/bus/05/mix/on",
		Arguments: []any{int32(0)},
	})
	if !ok || upd.Channel != 0 || upd.Bus != 5 || upd.Kind != "on" || upd.On {
		t.Fatalf("bus on: %+v ok=%v", upd, ok)
	}
}

func TestBusMeterIndex(t *testing.T) {
	if got := BusMeterIndex(1); got != 48 {
		t.Fatalf("bus 1: %d", got)
	}
	if got := BusMeterIndex(16); got != 63 {
		t.Fatalf("bus 16: %d", got)
	}
}
