//go:build aix || linux || solaris || zos || darwin || dragonfly || freebsd || netbsd || openbsd

package render

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func platformEnableVirtualTerminal(io.Writer) (bool, func() error) { return true, nil }

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlReadTermios)
	return err == nil
}

func terminalColumns(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 0
	}
	return int(ws.Col)
}
