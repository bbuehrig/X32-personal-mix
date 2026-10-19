//go:build windows

package osc

import "golang.org/x/sys/windows"

func setBroadcast(fd uintptr) error {
	return windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_BROADCAST, 1)
}
