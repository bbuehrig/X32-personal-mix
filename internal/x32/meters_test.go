package x32

import (
	"encoding/binary"
	"math"
	"testing"

	"X32-personal-mix/internal/osc"
)

func TestParseMeters0(t *testing.T) {
	const n = 4
	blob := make([]byte, 4+n*4)
	binary.LittleEndian.PutUint32(blob[0:4], uint32(n))
	vals := []float32{0, 0.25, 0.5, 1.0}
	for i, v := range vals {
		binary.LittleEndian.PutUint32(blob[4+i*4:8+i*4], math.Float32bits(v))
	}
	got, ok := ParseMeters0(osc.Message{
		Address:   "/meters/0",
		Arguments: []any{blob},
	})
	if !ok {
		t.Fatal("expected ok")
	}
	if len(got) != n {
		t.Fatalf("len=%d", len(got))
	}
	for i, v := range vals {
		if got[i] != v {
			t.Fatalf("got[%d]=%v want %v", i, got[i], v)
		}
	}
}

func TestParseMeters0RejectsOther(t *testing.T) {
	if _, ok := ParseMeters0(osc.Message{Address: "/ch/01/mix/01/level"}); ok {
		t.Fatal("expected reject")
	}
}
