// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/component/all"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/process"
	"github.com/juju/juju/state"
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
	jujutesting.JujuConnSuite
}

func (s *processesBaseSuite) addUnit(c *gc.C, charmName, serviceName string) (names.CharmTag, *state.Unit) {
	ch := s.AddTestingCharm(c, charmName)

	svc := s.AddTestingService(c, serviceName, ch)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)

	charmTag := ch.Tag().(names.CharmTag)
	return charmTag, unit
}

type processesHookContextSuite struct {
	processesBaseSuite
}

func (s *processesHookContextSuite) TestHookLifecycle(c *gc.C) {
	// start: info, launch, info
	// config-changed: info, set-status, info
	// stop: info, destroy, info
	// TODO(ericsnow) At the moment, only launch happens...

	unitTag := names.NewUnitTag("a-service/0")
	s.checkState(c, unitTag, nil)
	s.checkPluginLog(c, []string{
		"...",
	})

	s.prepPlugin(c, "myproc", "xyz123", "running")
	s.checkPluginLog(c, []string{
		"...",
	})

	// Add/start the unit.

	_, unit := s.addUnit(c, "proc-hooks", "a-service")
	c.Assert(unit.UnitTag(), gc.Equals, unitTag)

	s.checkState(c, unitTag, []process.Info{{
		Process: charm.Process{
			Name: "myproc",
			Type: "myplugin",
		},
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				Label: "running",
			},
		},
	}})
	s.checkPluginLog(c, []string{
		"...",
	})

	// Change the config.
	s.setConfig(c, unitTag, "status", "okay")

	s.checkState(c, unitTag, []process.Info{{
		Process: charm.Process{
			Name: "myproc",
			Type: "myplugin",
		},
		Details: process.Details{
			ID: "xyz123",
			Status: process.PluginStatus{
				Label: "okay",
			},
		},
	}})
	s.checkPluginLog(c, []string{
		"...",
	})

	// Stop the unit.
	s.destroyUnit(c, unitTag)

	s.checkState(c, unitTag, nil)
	s.checkPluginLog(c, []string{
		"...",
	})
}

// TODO(ericsnow) Add a test specifically for each supported plugin
// (e.g. docker)?

func (s *processesHookContextSuite) TestRegister(c *gc.C) {
	//s.addUnit(c, "proc-actions", "a-service")
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
