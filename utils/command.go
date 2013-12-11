// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os/exec"
)

// RunCommand executes the command and return the combined output.
func RunCommand(command string, args ...string) (output string, err error) {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	output = string(out)
	if err != nil {
		return output, err
	}
	return output, nil
}
