//go:build unix

package osc

import "golang.org/x/sys/unix"

func setBroadcast(fd uintptr) error {
	return unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
}
