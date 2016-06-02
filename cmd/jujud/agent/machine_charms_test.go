// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	charmtesting "github.com/juju/juju/apiserver/charmrevisionupdater/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// MachineWithCharmsSuite provides infrastructure for tests which need to
// work with charms.
type MachineWithCharmsSuite struct {
	commonMachineSuite
	charmtesting.CharmSuite

	machine *state.Machine
}

var _ = gc.Suite(&MachineWithCharmsSuite{})

func (s *MachineWithCharmsSuite) SetUpSuite(c *gc.C) {
	s.commonMachineSuite.SetUpSuite(c)
	s.CharmSuite.SetUpSuite(c, &s.commonMachineSuite.JujuConnSuite)
}

func (s *MachineWithCharmsSuite) TearDownSuite(c *gc.C) {
	s.CharmSuite.TearDownSuite(c)
	s.commonMachineSuite.TearDownSuite(c)
}

func (s *MachineWithCharmsSuite) SetUpTest(c *gc.C) {
	s.commonMachineSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
}

func (s *MachineWithCharmsSuite) TearDownTest(c *gc.C) {
	s.CharmSuite.TearDownTest(c)
	s.commonMachineSuite.TearDownTest(c)
}

func (s *MachineWithCharmsSuite) TestManageModelRunsCharmRevisionUpdater(c *gc.C) {
	m, _, _ := s.primeAgent(c, state.JobManageModel)

	s.SetupScenario(c)

	a := s.newAgent(c, m)
	go func() {
		c.Check(a.Run(nil), jc.ErrorIsNil)
	}()
	defer func() { c.Check(a.Stop(), jc.ErrorIsNil) }()

	checkRevision := func() bool {
		curl := charm.MustParseURL("cs:quantal/mysql")
		placeholder, err := s.State.LatestPlaceholderCharm(curl)
		return err == nil && placeholder.String() == curl.WithRevision(23).String()
	}
	success := false
	for attempt := coretesting.LongAttempt.Start(); attempt.Next(); {
		if success = checkRevision(); success {
			break
		}
	}
	c.Assert(success, jc.IsTrue)
}
