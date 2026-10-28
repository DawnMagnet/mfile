//go:build !unix && !windows

package render

import "io"

func isTerminal(io.Writer) bool { return false }

func terminalColumns(io.Writer) int { return 0 }
