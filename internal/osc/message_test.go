package osc

import (
	"testing"
)

func TestMarshalUnmarshalXinfo(t *testing.T) {
	original := Message{
		Address: "/xinfo",
		Arguments: []any{
			"192.168.1.10",
			"FOH",
			"X32",
			"4.06-8",
		},
	}
	data, err := original.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Address != original.Address {
		t.Fatalf("address: got %q want %q", got.Address, original.Address)
	}
	if len(got.Arguments) != 4 {
		t.Fatalf("args len: got %d", len(got.Arguments))
	}
	for i, want := range original.Arguments {
		if got.Arguments[i] != want {
			t.Fatalf("arg %d: got %v want %v", i, got.Arguments[i], want)
		}
	}
}

func TestEmptyTypetag(t *testing.T) {
	msg := Message{Address: "/xinfo"}
	data, err := msg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != "/xinfo" || len(got.Arguments) != 0 {
		t.Fatalf("unexpected: %+v", got)
	}
}
