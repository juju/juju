// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os/exec"
	"strings"

	"github.com/juju/errors"
)

func RunCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
		return errors.Errorf(
			"error executing %q: %s",
			cmd,
			strings.Replace(string(out), "\n", "; ", -1),
		)
	}
	return errors.Annotatef(err, "error executing %q", cmd)
}
