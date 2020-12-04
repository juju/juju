// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v2/exec"
)

type ScriptResult struct {
	Stdout   []byte
	Stderr   []byte
	Code     int
	TimedOut bool
}

func runCommand(command string, environ []string, clock clock.Clock, timeout time.Duration) (*ScriptResult, error) {
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
	timedOut := false

	if timeout != 0 {
		cancel = make(chan struct{})
		go func() {
			<-clock.After(timeout)
			timedOut = true
			close(cancel)
		}()
	}

	result, err := cmd.WaitWithCancel(cancel)

	return &ScriptResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		Code:     result.Code,
		TimedOut: timedOut,
	}, errors.Trace(err)
}
