// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionworker_test

import (
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/apiserver/charmrevisionupdater/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker/charmrevisionworker"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type RevisionUpdateSuite struct {
	testing.CharmSuite
	jujutesting.JujuConnSuite

	st             *api.State
	versionUpdater *charmrevisionworker.RevisionUpdateWorker
}

var _ = gc.Suite(&RevisionUpdateSuite{})

func (s *RevisionUpdateSuite) SetUpSuite(c *gc.C) {
	c.Assert(*charmrevisionworker.Interval, gc.Equals, 24*time.Hour)
	s.JujuConnSuite.SetUpSuite(c)
	s.CharmSuite.SetUpSuite(c, &s.JujuConnSuite)
}

func (s *RevisionUpdateSuite) TearDownSuite(c *gc.C) {
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *RevisionUpdateSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)

	machine, err := s.State.AddMachine("quantal", state.JobManageEnviron)
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

func (s *RevisionUpdateSuite) TearDownTest(c *gc.C) {
	s.JujuConnSuite.TearDownTest(c)
}

func (s *RevisionUpdateSuite) runUpdater(c *gc.C, updateInterval time.Duration) {
	s.PatchValue(charmrevisionworker.Interval, updateInterval)
	revisionUpdaterState := s.st.CharmRevisionUpdater()
	c.Assert(revisionUpdaterState, gc.NotNil)

	s.versionUpdater = charmrevisionworker.NewRevisionUpdateWorker(revisionUpdaterState)
	s.AddCleanup(func(c *gc.C) { s.versionUpdater.Stop() })
}

func (s *RevisionUpdateSuite) checkCharmRevision(c *gc.C, expectedRev int) bool {
	checkRevision := func() bool {
		curl := charm.MustParseURL("cs:quantal/mysql")
		placeholder, err := s.State.LatestPlaceholderCharm(curl)
		return err == nil && placeholder.String() == curl.WithRevision(expectedRev).String()
	}

	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if success = checkRevision(); success {
			break
		}
	}
	return success
}

func (s *RevisionUpdateSuite) TestVersionUpdateRunsInitially(c *gc.C) {
	s.SetupScenario(c)

	// Run the updater with a long update interval to ensure only the initial
	// update on startup is run.
	s.runUpdater(c, time.Hour)
	c.Assert(s.checkCharmRevision(c, 23), jc.IsTrue)
}

func (s *RevisionUpdateSuite) TestVersionUpdateRunsPeriodically(c *gc.C) {
	s.SetupScenario(c)

	// Start the updater and check the initial status.
	s.runUpdater(c, 5*time.Millisecond)
	c.Assert(s.checkCharmRevision(c, 23), jc.IsTrue)

	// Make some changes
	s.UpdateStoreRevision("cs:quantal/mysql", 24)
	// Check the results of the latest changes.
	c.Assert(s.checkCharmRevision(c, 24), jc.IsTrue)
}
