// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionworker_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/charmrevisionupdater/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/charmrevisionworker"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type RevisionUpdateSuite struct {
	testing.CharmSuite
	jujutesting.JujuConnSuite

	st             api.Connection
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
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("i-manager", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
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
	id := charm.MustParseReference("~who/quantal/mysql-24")
	ch := testcharms.Repo.CharmArchive(c.MkDir(), id.Name)
	s.Server.UploadCharm(c, ch, id, true)
	// Check the results of the latest changes.
	c.Assert(s.checkCharmRevision(c, 24), jc.IsTrue)
}

func (s *RevisionUpdateSuite) TestDiesOnError(c *gc.C) {
	mockUpdate := func(ruw *charmrevisionworker.RevisionUpdateWorker) error {
		return errors.New("boo")
	}
	s.PatchValue(&charmrevisionworker.UpdateVersions, mockUpdate)

	revisionUpdaterState := s.st.CharmRevisionUpdater()
	c.Assert(revisionUpdaterState, gc.NotNil)

	versionUpdater := charmrevisionworker.NewRevisionUpdateWorker(revisionUpdaterState)
	err := versionUpdater.Stop()
	c.Assert(errors.Cause(err), gc.ErrorMatches, "boo")
}
