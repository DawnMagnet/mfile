//go:build windows

package render

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func platformEnableVirtualTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return true
	}
	handle := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return true
	}
	err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	return err == nil
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
