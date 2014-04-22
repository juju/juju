// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

func Restart() error {
	if os.Geteuid() == 0 {
		return runCommand("restart", "rsyslog")
	}
	return fmt.Errorf("must be root")
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
