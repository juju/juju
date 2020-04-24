// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	networktesting "github.com/juju/juju/core/network/testing"
	"github.com/juju/juju/state"
)

type stateSuite struct {
	uniterSuite
	networktesting.FirewallHelper
	*apitesting.APIAddresserTests
	*apitesting.ModelWatcherTests
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	waitForModelWatchersIdle := func(c *gc.C) {
		s.JujuConnSuite.WaitForModelWatchersIdle(c, s.BackingState.ModelUUID())
	}
	s.APIAddresserTests = apitesting.NewAPIAddresserTests(s.uniter, s.BackingState, waitForModelWatchersIdle)
	s.ModelWatcherTests = apitesting.NewModelWatcherTests(s.uniter, s.BackingState, s.Model)
}

func (s *stateSuite) TestProviderType(c *gc.C) {
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	providerType, err := s.uniter.ProviderType()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(providerType, gc.DeepEquals, cfg.Type())
}

func (s *stateSuite) TestAllMachinePorts(c *gc.C) {
	// Verify no ports are opened yet on the machine or unit.
	machinePorts, err := s.wordpressMachine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machinePorts, gc.HasLen, 0)
	unitPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPorts, gc.HasLen, 0)

	// Add another wordpress unit on the same machine.
	wordpressUnit1, err := s.wordpressApplication.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = wordpressUnit1.AssignToMachine(s.wordpressMachine)
	c.Assert(err, jc.ErrorIsNil)

	// Open some ports on both units.
	s.AssertOpenUnitPorts(c, s.wordpressUnit, "", "tcp", 100, 200)
	s.AssertOpenUnitPorts(c, s.wordpressUnit, "", "udp", 10, 20)
	s.AssertOpenUnitPorts(c, wordpressUnit1, "", "tcp", 201, 250)
	s.AssertOpenUnitPorts(c, wordpressUnit1, "", "udp", 1, 8)

	portsMap, err := s.uniter.AllMachinePorts(s.wordpressMachine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(portsMap, jc.DeepEquals, map[network.PortRange]params.RelationUnit{
		{100, 200, "tcp"}: {Unit: s.wordpressUnit.Tag().String()},
		{10, 20, "udp"}:   {Unit: s.wordpressUnit.Tag().String()},
		{201, 250, "tcp"}: {Unit: wordpressUnit1.Tag().String()},
		{1, 8, "udp"}:     {Unit: wordpressUnit1.Tag().String()},
	})
}
