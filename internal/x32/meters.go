package x32

import (
	"encoding/binary"
	"math"
	"net"
	"time"

	"X32-personal-mix/internal/osc"
)

// ParseMeters0 extracts linear meter floats from a /meters/0 blob reply.
// Blob layout: little-endian int32 count, then count little-endian float32 values.
// The first 32 values are input channels 1–32.
func ParseMeters0(msg osc.Message) ([]float32, bool) {
	if msg.Address != "/meters/0" {
		return nil, false
	}
	if len(msg.Arguments) < 1 {
		return nil, false
	}
	blob, ok := msg.Arguments[0].([]byte)
	if !ok || len(blob) < 4 {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(blob[0:4]))
	if n <= 0 || 4+n*4 > len(blob) {
		return nil, false
	}
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := binary.LittleEndian.Uint32(blob[4+i*4 : 8+i*4])
		out[i] = math.Float32frombits(bits)
	}
	return out, true
}

// StartMeters renews a /meters/0 subscription until stop is closed.
// timeFactor multiplies the 50ms base interval (2 ≈ 100ms updates). Subscription lasts ~10s.
func StartMeters(client *osc.Client, dest *net.UDPAddr, renewEvery time.Duration, timeFactor int32, stop <-chan struct{}) {
	if timeFactor < 1 || timeFactor > 99 {
		timeFactor = 2
	}
	send := func() {
		_ = client.SendMsg(dest, osc.Message{
			Address: "/meters",
			Arguments: []any{
				"/meters/0",
				int32(0),
				int32(0),
				timeFactor,
			},
		})
	}
	send()
	t := time.NewTicker(renewEvery)
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
