// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"bytes"
	"os/exec"

	"github.com/juju/deputy"
)

const executable = "docker"

var execCommand = exec.Command

func runDocker(command string, args ...string) ([]byte, error) {
	d := deputy.Deputy{
		Errors: deputy.FromStderr,
	}
	cmd := execCommand(executable, append([]string{command}, args...)...)
	out := &bytes.Buffer{}
	cmd.Stdout = out
	if err := d.Run(cmd); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
