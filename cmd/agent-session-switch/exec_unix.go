//go:build unix

package main

import (
	"fmt"
	"os"
	"syscall"
)

// execAgent replaces the current process with the agent, so the shell sees
// the agent itself and signals and the terminal behave as if it was run directly.
func execAgent(path string, argv []string) error {
	if err := syscall.Exec(path, argv, os.Environ()); err != nil {
		return fmt.Errorf("exec 失败: %w", err)
	}
	return nil
}
