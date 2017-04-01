// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debinterfaces

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/exec"
)

type scriptResult struct {
	Stdout []byte
	Stderr []byte
	Code   int
}

func runCommand(command string, environ []string, clock clock.Clock, timeout time.Duration) (*scriptResult, error) {
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
		err = errors.Trace(err)
	}

	return &scriptResult{
		Stdout: result.Stdout,
		Stderr: result.Stderr,
		Code:   result.Code,
	}, err
}
