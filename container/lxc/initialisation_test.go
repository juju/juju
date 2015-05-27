// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/packaging/commands"
	"github.com/juju/utils/packaging/manager"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type InitialiserSuite struct {
	testing.BaseSuite
	calledCmds []string
}

var _ = gc.Suite(&InitialiserSuite{})

// getMockRunCommandWithRetry is a helper function which returns a function
// with an identical signature to manager.RunCommandWithRetry which saves each
// command it recieves in a slice and always returns no output, error code 0
// and a nil error.
func getMockRunCommandWithRetry(calledCmds *[]string) func(string) (string, int, error) {
	return func(cmd string) (string, int, error) {
		*calledCmds = append(*calledCmds, cmd)
		return "", 0, nil
	}
}

func (s *InitialiserSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.calledCmds = []string{}
	s.PatchValue(&manager.RunCommandWithRetry, getMockRunCommandWithRetry(&s.calledCmds))
}

func (s *InitialiserSuite) TestLTSSeriesPackages(c *gc.C) {
	// Momentarily, the only series with a dedicated cloud archive is precise,
	// which we will use for the following test:
	paccmder, err := commands.NewPackageCommander("precise")
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(&version.Current.Series, "precise")
	container := NewContainerInitialiser("precise")

	err = container.Initialise()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.calledCmds, gc.DeepEquals, []string{
		paccmder.InstallCmd("--target-release", "precise-updates/cloud-tools", "lxc"),
		paccmder.InstallCmd("--target-release", "precise-updates/cloud-tools", "cloud-image-utils"),
	})
}

func (s *InitialiserSuite) TestNoSeriesPackages(c *gc.C) {
	// Here we want to test for any other series whilst avoiding the
	// possibility of hitting a cloud archive-requiring release.
	// As such, we simply pass an empty series.
	paccmder, err := commands.NewPackageCommander("")
	c.Assert(err, jc.ErrorIsNil)

	container := NewContainerInitialiser("")

	err = container.Initialise()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.calledCmds, gc.DeepEquals, []string{
		paccmder.InstallCmd("lxc"),
		paccmder.InstallCmd("cloud-image-utils"),
	})
}
