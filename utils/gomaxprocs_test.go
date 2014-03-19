// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type gomaxprocsSuite struct {
	testbase.LoggingSuite
	setmaxprocs    chan int
	numCPUResponse int
}

var _ = gc.Suite(&gomaxprocsSuite{})

func (s *gomaxprocsSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	// always stub out GOMAXPROCS so we don't actually change anything
	s.numCPUResponse = 2
	s.setmaxprocs = make(chan int, 1)
	maxprocsfunc := func(n int) int {
		c.Logf("sending %d on setmaxprocs", n)
		s.setmaxprocs <- n
		return 1
	}
	numCPUFunc := func() int { return s.numCPUResponse }
	cleanup := utils.OverrideGOMAXPROCSFuncs(maxprocsfunc, numCPUFunc)
	s.AddCleanup(func(c *gc.C) { c.Logf("running cleanup"); cleanup() })
	s.PatchEnvironment("GOMAXPROCS", "")
}

func (s *gomaxprocsSuite) TestUseMultipleCPUsDoesNothingWhenGOMAXPROCSSet(c *gc.C) {
	os.Setenv("GOMAXPROCS", "1")
	utils.UseMultipleCPUs()
	c.Check(<-s.setmaxprocs, gc.Equals, 0)
}

func (s *gomaxprocsSuite) TestUseMultipleCPUsWhenEnabled(c *gc.C) {
	utils.UseMultipleCPUs()
	c.Check(<-s.setmaxprocs, gc.Equals, 2)
	s.numCPUResponse = 4
	utils.UseMultipleCPUs()
	c.Check(<-s.setmaxprocs, gc.Equals, 4)
}
