// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmversionworker_test

import (
	stdtesting "testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/apiserver/charmversionupdater/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker/charmversionworker"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type VersionUpdaterSuite struct {
	testing.CharmSuite

	st             *api.State
	versionUpdater *charmversionworker.VersionUpdateWorker
}

var _ = gc.Suite(&VersionUpdaterSuite{})

func (s *VersionUpdaterSuite) SetUpSuite(c *gc.C) {
	c.Assert(*charmversionworker.Interval, gc.Equals, 6*time.Hour)
	s.CharmSuite.SetUpSuite(c)
}

func (s *VersionUpdaterSuite) SetUpTest(c *gc.C) {
	s.CharmSuite.SetUpTest(c)

	machine, err := s.State.AddMachine("quantal", state.JobManageState)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAsMachine(c, machine.Tag(), password, "fake_nonce")
	c.Assert(s.st, gc.NotNil)
}

func (s *VersionUpdaterSuite) TearDownTest(c *gc.C) {
	c.Assert(s.versionUpdater.Stop(), gc.IsNil)
	s.CharmSuite.TearDownTest(c)
}

func (s *VersionUpdaterSuite) runUpdater(c *gc.C, updateInterval time.Duration) {
	s.PatchValue(charmversionworker.Interval, updateInterval)
	versionUpdaterState := s.st.CharmVersionUpdater()
	c.Assert(versionUpdaterState, gc.NotNil)

	s.versionUpdater = charmversionworker.NewVersionUpdateWorker(versionUpdaterState)
}

func (s *VersionUpdaterSuite) checkStatus(c *gc.C, expected string) bool {
	svc, err := s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	if !c.Check(svc.RevisionStatus(), gc.Equals, expected) {
		return false
	}
	u, err := s.State.Unit("mysql/0")
	c.Assert(err, gc.IsNil)
	return c.Check(u.RevisionStatus(), gc.Equals, "unknown")
}

func (s *VersionUpdaterSuite) checkServiceRevisionStatus(c *gc.C, expected string) bool {
	checkStatus := func() bool {
		svc, err := s.State.Service("mysql")
		c.Assert(err, gc.IsNil)
		unit, err := s.State.Unit("mysql/0")
		c.Assert(err, gc.IsNil)
		if svc.RevisionStatus() != expected {
			return false
		}
		return unit.RevisionStatus() == "unknown"
	}

	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if success = checkStatus(); success {
			break
		}
	}
	return success
}

func (s *VersionUpdaterSuite) TestVersionUpdateRunsInitially(c *gc.C) {
	s.SetupScenario(c)

	// Run the updater with a long update interval to ensure only the initial
	// update on startup is run.
	s.runUpdater(c, time.Hour)
	c.Assert(s.checkServiceRevisionStatus(c, "out of date (available: 23)"), jc.IsTrue)
}

func (s *VersionUpdaterSuite) TestVersionUpdateRunsPeriodically(c *gc.C) {
	s.SetupScenario(c)

	// Start the updater and check the initial status.
	s.runUpdater(c, 5*time.Millisecond)
	c.Assert(s.checkServiceRevisionStatus(c, "out of date (available: 23)"), jc.IsTrue)

	// Make some changes
	ch := s.AddCharmWithRevision(c, "mysql", 23)
	svc, err := s.State.Service("mysql")
	c.Assert(err, gc.IsNil)
	err = svc.SetCharm(ch, true)
	c.Assert(err, gc.IsNil)

	// Check the results of the latest changes.
	c.Assert(s.checkServiceRevisionStatus(c, ""), jc.IsTrue)
}
