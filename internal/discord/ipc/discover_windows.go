//go:build windows

package ipc

import (
	"net"
	"strconv"
	"time"

	npipe "gopkg.in/natefinch/npipe.v2"
)

func dialAddr(addr string, timeout time.Duration) (net.Conn, error) {
	return npipe.DialTimeout(addr, timeout)
}

// socketCandidates lists the well-known Discord named pipes.
func Candidates() []string {
	var out []string
	for i := 0; i < 10; i++ {
		out = append(out, `\\.\pipe\discord-ipc-`+strconv.Itoa(i))
	}
	return out
}
