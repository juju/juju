// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptrunner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v4/exec"
)

// ScriptResult holds the result of running a script.
type ScriptResult struct {
	Stdout []byte
	Stderr []byte
	Code   int
}

// RunCommand runs the given command with the given environment and
// returns the result. If timeout is non-zero, the command will be
// killed after that duration.
func RunCommand(command string, environ []string, clock clock.Clock, timeout time.Duration) (*ScriptResult, error) {
	cmd := exec.RunParams{
		Commands:    command,
		Environment: environ,
		Clock:       clock,
	}

	err := cmd.Run()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cancel chan struct{}

	if timeout != 0 {
		cancel = make(chan struct{})
		go func() {
			<-clock.After(timeout)
			close(cancel)
		}()
	}

	result, err := cmd.WaitWithCancel(cancel)
	if err != nil {
		return nil, errors.Annotatef(err, "running command %q", command)
	}

	return &ScriptResult{
		Stdout: result.Stdout,
		Stderr: result.Stderr,
		Code:   result.Code,
	}, err
}
