// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"os"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type gomaxprocsSuite struct {
	testing.IsolationSuite
	setmaxprocs    chan int
	numCPUResponse int
	setMaxProcs    int
}

var _ = gc.Suite(&gomaxprocsSuite{})

func (s *gomaxprocsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	// always stub out GOMAXPROCS so we don't actually change anything
	s.numCPUResponse = 2
	s.setMaxProcs = -1
	maxProcsFunc := func(n int) int {
		s.setMaxProcs = n
		return 1
	}
	numCPUFunc := func() int { return s.numCPUResponse }
	s.PatchValue(utils.GOMAXPROCS, maxProcsFunc)
	s.PatchValue(utils.NumCPU, numCPUFunc)
	s.PatchEnvironment("GOMAXPROCS", "")
}

func (s *gomaxprocsSuite) TestUseMultipleCPUsDoesNothingWhenGOMAXPROCSSet(c *gc.C) {
	os.Setenv("GOMAXPROCS", "1")
	utils.UseMultipleCPUs()
	c.Check(s.setMaxProcs, gc.Equals, 0)
}

func (s *gomaxprocsSuite) TestUseMultipleCPUsWhenEnabled(c *gc.C) {
	utils.UseMultipleCPUs()
	c.Check(s.setMaxProcs, gc.Equals, 2)
	s.numCPUResponse = 4
	utils.UseMultipleCPUs()
	c.Check(s.setMaxProcs, gc.Equals, 4)
}
