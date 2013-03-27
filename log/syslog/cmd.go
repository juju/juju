package syslog

import (
	"bytes"
	"fmt"
	"os/exec"
)

func Restart() error {
	return runCommand("restart", "rsyslog")
}

func runCommand(args ...string) error {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err == nil {
		return nil
	}
	out = bytes.TrimSpace(out)
	if len(out) > 0 {
		return fmt.Errorf("exec %q: %v (%s)", args, err, out)
	}
	return fmt.Errorf("exec %q: %v", args, err)
}
