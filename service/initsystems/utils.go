// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"bytes"
	"os/exec"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	uexec "github.com/juju/utils/exec"
)

// RetryAttempts provides a uniform retry strategy that may be shared
// across InitSystem implementations.
var RetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 250 * time.Millisecond,
}

// TODO(ericsnow) Combine RunCommand and RunPsCommand.

// RunPsCommand runs the provided shell command on the local host. This
// is useful for InitSystem implementations that cannot make another
// mechanism (e.g. Go bindings).
func RunPsCommand(cmd string) (*uexec.ExecResponse, error) {
	com := uexec.RunParams{
		Commands: cmd,
	}
	out, err := uexec.RunCommands(com)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if out.Code != 0 {
		return nil, errors.Errorf("running %s: %s", cmd, string(out.Stderr))
	}
	return out, nil
}

// RunCommand runs the provided shell command and args on the local
// host. This is useful for InitSystem implementations that cannot make
// another mechanism (e.g. Go bindings).
func RunCommand(cmdAndArgs ...string) error {
	cmd := cmdAndArgs[0]
	args := cmdAndArgs[1:]
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err == nil {
		return nil
	}

	out = bytes.TrimSpace(out)
	if len(out) > 0 {
		return errors.Annotatef(err, "exec %q: (%s)", cmdAndArgs, out)
	}
	return errors.Annotatef(err, "exec %q", cmdAndArgs)
}
