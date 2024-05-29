// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager_test

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/manager"
)

var _ = gc.Suite(&RunSuite{})

type RunSuite struct {
	testing.IsolationSuite
}

func (s *RunSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
}

func (s *RunSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *RunSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
}

func (s *RunSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)
}

type mockExitStatuser int

func (es mockExitStatuser) ExitStatus() int {
	return int(es)
}

func (s *RunSuite) TestRunCommandWithRetryDoesOnPackageLocationFailure(c *gc.C) {
	const minRetries = 3
	var calls int
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.Attempts, minRetries)
	s.PatchValue(&manager.Delay, testing.ShortWait)
	s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
		return mockExitStatuser(100) // retry each time.
	})
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		// Replace the command path and args so it's a no-op.
		cmd.Path = ""
		cmd.Args = []string{"version"}
		// Call the real cmd.CombinedOutput to simulate better what
		// happens in production. See also http://pad.lv/1394524.
		output, err := cmd.CombinedOutput()
		if _, ok := err.(*exec.Error); err != nil && !ok {
			c.Check(err, gc.ErrorMatches, "exec: Stdout already set")
			c.Fatalf("CommandOutput called twice unexpectedly")
		}
		return output, cmdError
	})

	calls = 0
	apt := manager.NewAptPackageManager()
	err := apt.Install(testedPackageName)
	c.Check(err, gc.ErrorMatches, "packaging command failed: attempt count exceeded: exit status.*")
	c.Check(calls, gc.Equals, minRetries)

	// reset calls and re-test for Yum calls:
	calls = 0
	yum := manager.NewYumPackageManager()
	err = yum.Install(testedPackageName)
	c.Check(err, gc.ErrorMatches, "packaging command failed: attempt count exceeded: exit status.*")
	c.Check(calls, gc.Equals, minRetries)
}

func (s *RunSuite) TestRunCommandWithRetryStopsWithFatalError(c *gc.C) {
	const minRetries = 3
	var calls int
	state := os.ProcessState{}
	cmdError := &exec.ExitError{ProcessState: &state}
	s.PatchValue(&manager.Attempts, minRetries)
	s.PatchValue(&manager.Delay, testing.ShortWait)
	s.PatchValue(&manager.ProcessStateSys, func(*os.ProcessState) interface{} {
		return mockExitStatuser(100) // retry each time.
	})
	s.PatchValue(&manager.CommandOutput, func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		cmdOutput := fmt.Sprintf("Reading state information...\nE: Unable to locate package %s",

			testedPackageName)
		return []byte(cmdOutput), cmdError
	})

	apt := manager.NewAptPackageManager()
	err := apt.Install(testedPackageName)
	c.Check(err, gc.ErrorMatches, "packaging command failed: encountered fatal error: unable to locate package")
	c.Check(calls, gc.Equals, 1)
}
