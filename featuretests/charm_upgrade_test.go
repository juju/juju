// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type CharmUpgradeSuite struct {
	testing.JujuConnSuite

	appOneName string
	appOne     *state.Application

	appTwoName string
}

func (s *CharmUpgradeSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.appOneName = "app1"
	charmOne := s.AddTestingCharm(c, "upgrade-charm1")

	var err error
	s.appOne, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:  s.appOneName,
		Charm: charmOne,
	})
	c.Assert(err, jc.ErrorIsNil)
	unitOne, err := s.appOne.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	unitOne.SetCharmURL(charmOne.URL())

	s.appTwoName = "app2"
	charmTwo := s.AddTestingCharm(c, "upgrade-charm2")
	appTwo, err := s.State.AddApplication(state.AddApplicationArgs{
		Name:  s.appTwoName,
		Charm: charmTwo,
	})
	c.Assert(err, jc.ErrorIsNil)
	unitTwo, err := appTwo.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	unitTwo.SetCharmURL(charmTwo.URL())

	runCommandExpectSuccess(c, "add-relation", s.appOneName, s.appTwoName)
}

// This test deploys 2 applications with 1 unit each and relates units to each other.
// It then updates charm for one of the application with various changes to relations.
// All upgrades should succeed since there is only one unit.
// The only update that should fail is with changes to relation
// that has already been established to another application's unit.
// Addresses lp#1510787 scenario.
func (s *CharmUpgradeSuite) TestUpgradeCharm(c *gc.C) {
	// should pass: peer relation name to one unit changed
	v2 := s.AddTestingCharm(c, "upgrade-charm1v2")
	cfg := state.SetCharmConfig{Charm: v2}
	err := s.appOne.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// should pass: peer relation interface name to one unit changed
	v3 := s.AddTestingCharm(c, "upgrade-charm1v3")
	cfg = state.SetCharmConfig{Charm: v3}
	err = s.appOne.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	// should fail: peer relation change to another app's unit exist
	v4 := s.AddTestingCharm(c, "upgrade-charm1v4")
	cfg = state.SetCharmConfig{Charm: v4}
	err = s.appOne.SetCharm(cfg)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf(`*would break relation "%v:server %v:server"*`, s.appTwoName, s.appOneName))
}
