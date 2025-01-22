// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager_test

import (
	"os"
	"os/exec"

	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/manager"
)

var _ = gc.Suite(&RunSuite{})

type RunSuite struct {
	testing.IsolationSuite
}

func (s *RunSuite) TestRunCommandWithRetryAttemptsExceeded(c *gc.C) {
	calls := 0
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		// Call the real cmd.CombinedOutput to simulate better what
		// happens in production. See also http://pad.lv/1394524.
		output, err := cmd.CombinedOutput()
		if _, ok := err.(*exec.Error); err != nil && !ok {
			c.Check(err, gc.ErrorMatches, "exec: Stdout already set")
			c.Fatalf("CommandOutput called twice unexpectedly")
		}
		return output, cmdError
	})

	_, _, err := manager.RunCommandWithRetry("ls -la", alwaysRetryable{}, manager.RetryPolicy{
		Attempts: 3,
		Delay:    testing.ShortWait,
	})

	c.Check(err, gc.ErrorMatches, "packaging command failed: attempt count exceeded: exit status.*")
	c.Check(calls, gc.Equals, 3)
}

func (s *RunSuite) TestRunCommandWithRetryStopsWithFatalError(c *gc.C) {
	calls := 0
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		// Call the real cmd.CombinedOutput to simulate better what
		// happens in production. See also http://pad.lv/1394524.
		output, err := cmd.CombinedOutput()
		if _, ok := err.(*exec.Error); err != nil && !ok {
			c.Check(err, gc.ErrorMatches, "exec: Stdout already set")
			c.Fatalf("CommandOutput called twice unexpectedly")
		}
		return output, cmdError
	})

	_, _, err := manager.RunCommandWithRetry("ls -la", alwaysFatal{}, manager.RetryPolicy{
		Attempts: 3,
		Delay:    testing.ShortWait,
	})

	c.Check(err, gc.ErrorMatches, "packaging command failed: encountered fatal error: boom!")
	c.Check(calls, gc.Equals, 1)
}

type mockExitStatuser int

func (es mockExitStatuser) ExitStatus() int {
	return int(es)
}

type alwaysRetryable struct{}

func (alwaysRetryable) IsRetryable(int, string) bool {
	return true
}

func (alwaysRetryable) MaskError(int, string) error {
	return errors.Errorf("boom!")
}

type alwaysFatal struct{}

func (alwaysFatal) IsRetryable(int, string) bool {
	return false
}

func (alwaysFatal) MaskError(int, string) error {
	return errors.Errorf("boom!")
}
