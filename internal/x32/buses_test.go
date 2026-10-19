package x32

import "testing"

func TestBusNamePath(t *testing.T) {
	if got := busNamePath(1); got != "/bus/01/config/name" {
		t.Fatalf("got %q", got)
	}
	if got := busNamePath(16); got != "/bus/16/config/name" {
		t.Fatalf("got %q", got)
	}
}
