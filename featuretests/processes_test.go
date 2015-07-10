// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/component/all"
	jujutesting "github.com/juju/juju/juju/testing"
)

func initProcessesSuites() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}

	gc.Suite(&processesHookContextSuite{})
	gc.Suite(&processesWorkerSuite{})
	gc.Suite(&processesCmdJujuSuite{})
}

type processesBaseSuite struct {
	jujutesting.RepoSuite
}

type processesHookContextSuite struct {
	processesBaseSuite
}

func (s *processesHookContextSuite) TestRegister(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesHookContextSuite) TestLaunch(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesHookContextSuite) TestInfo(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesHookContextSuite) TestUnregister(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesHookContextSuite) TestDestroy(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

type processesWorkerSuite struct {
	processesBaseSuite
}

func (s *processesWorkerSuite) TestSetStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

func (s *processesWorkerSuite) TestCleanUp(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}

type processesCmdJujuSuite struct {
	processesBaseSuite
}

func (s *processesCmdJujuSuite) TestStatus(c *gc.C) {
	// TODO(ericsnow) Finish!
	c.Skip("not finished")
}
