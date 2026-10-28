//go:build windows

package render

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func platformEnableVirtualTerminal(w io.Writer) (bool, func() error) {
	f, ok := w.(*os.File)
	if !ok {
		return true, nil
	}
	handle := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Not a console. Nothing to restore.
		return true, nil
	}
	next := mode | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if err := windows.SetConsoleMode(handle, next); err != nil {
		return false, nil
	}
	if next == mode {
		// Already enabled. End is a no-op.
		return true, nil
	}
	return true, func() error {
		return windows.SetConsoleMode(handle, mode)
	}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	var mode uint32
	err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode)
	return err == nil
}

func terminalColumns(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	var info windows.ConsoleScreenBufferInfo
	err := windows.GetConsoleScreenBufferInfo(windows.Handle(f.Fd()), &info)
	if err != nil {
		return 0
	}
	cols := int(info.Window.Right - info.Window.Left + 1)
	if cols <= 0 {
		return 0
	}
	return cols
}
