//go:build amd64 && !windows

package cpuguard

import (
	"os"
	"syscall"
)

// reexecSelf replaces this process. It returns only on failure.
func reexecSelf(env []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, env)
}
