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

var RetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 250 * time.Millisecond,
}

// TODO(ericsnow) Combine RunCommand and RunPsCommand.

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

func RunCommand(args ...string) error {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err == nil {
		return nil
	}

	out = bytes.TrimSpace(out)
	if len(out) > 0 {
		return errors.Annotatef(err, "exec %q: (%s)", args, out)
	}
	return errors.Annotatef(err, "exec %q", args)
}
