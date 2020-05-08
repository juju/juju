// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/charm/v7"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	charmtesting "github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater/testing"
	"github.com/juju/juju/controller"
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

func (s *MachineWithCharmsSuite) SetUpTest(c *gc.C) {
	s.ControllerConfigAttrs = map[string]interface{}{
		// TODO(raftlease): setting this temporarily until the startup
		// issue is resolved.
		controller.Features: []interface{}{"legacy-leases"},
	}
	s.commonMachineSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
}

func (s *MachineWithCharmsSuite) TestManageModelRunsCharmRevisionUpdater(c *gc.C) {
	c.Skip("Update this test to work with raft leases")
	m, _, _ := s.primeAgent(c, state.JobManageModel)

	s.SetupScenario(c)

	a := s.newAgent(c, m)
	go func() {
		ctx := cmdtesting.Context(c)
		c.Check(a.Run(ctx), jc.ErrorIsNil)
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
