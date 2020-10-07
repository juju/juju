// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os/exec"
	"strings"

	"github.com/juju/errors"
)

// runCommand execs the provided command. It exists
// here so it can be overridden in export_test.go
func runCommand(cmd string, args ...string) error {
	logger.Tracef("runCommand cmd=%s, args=%s", cmd, args)
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()
	if err == nil {
		logger.Tracef("runCommand succeeded, output is:\n%s", out)
		return nil
	}
	logger.Tracef("runCommand error %v; output is:\n%s", out)
	if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
		return errors.Errorf(
			"error executing %q: %s",
			cmd,
			strings.Replace(string(out), "\n", "; ", -1),
		)
	}
	return errors.Annotatef(err, "error executing %q", cmd)
}
