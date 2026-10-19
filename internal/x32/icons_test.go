package x32

import "testing"

func TestIconSrc(t *testing.T) {
	if IconSrc(1) != "" {
		t.Fatal("blank should be empty")
	}
	if got := IconSrc(2); got != "/icons/2.png" {
		t.Fatalf("kick: %q", got)
	}
	if got := IconSrc(20); got != "/icons/20.png" {
		t.Fatalf("guitar: %q", got)
	}
	if got := IconSrc(48); got != "/icons/48.png" {
		t.Fatalf("oh L: %q", got)
	}
	if IconSrc(0) != "" || IconSrc(75) != "" {
		t.Fatal("oob")
	}
}
