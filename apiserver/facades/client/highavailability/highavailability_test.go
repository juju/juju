// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/client/highavailability"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type clientSuite struct {
	testing.JujuConnSuite

	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
	haServer   *highavailability.HighAvailabilityAPI

	commontesting.BlockHelper
}

var _ = gc.Suite(&clientSuite{})

var (
	emptyCons      = constraints.Value{}
	controllerCons = constraints.MustParse("mem=16G cores=16")
	defaultSeries  = ""
)

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	err := s.resources.RegisterNamed("machineID", common.StringResource("0"))
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag:        s.AdminUserTag(c),
		Controller: true,
	}

	s.haServer, err = highavailability.NewHighAvailabilityAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachines(state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobManageModel},
		Constraints: controllerCons,
		Addresses: []network.SpaceAddress{
			network.NewScopedSpaceAddress("127.0.0.1", network.ScopeMachineLocal),
			network.NewScopedSpaceAddress("cloud-local0.internal", network.ScopeCloudLocal),
			network.NewScopedSpaceAddress("fc00::0", network.ScopePublic),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// We have to ensure the agents are alive, or EnableHA will
	// create more to replace them.
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })
}

func (s *clientSuite) setMachineAddresses(c *gc.C, machineId string) {
	m, err := s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMachineAddresses(
		network.NewScopedSpaceAddress("127.0.0.1", network.ScopeMachineLocal),
		network.NewScopedSpaceAddress(fmt.Sprintf("cloud-local%s.internal", machineId), network.ScopeCloudLocal),
		network.NewScopedSpaceAddress(fmt.Sprintf("fc0%s::1", machineId), network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) enableHA(
	c *gc.C, numControllers int, cons constraints.Value, series string, placement []string,
) (params.ControllersChanges, error) {
	return enableHA(c, s.haServer, numControllers, cons, series, placement)
}

func enableHA(
	c *gc.C,
	haServer *highavailability.HighAvailabilityAPI,
	numControllers int,
	cons constraints.Value,
	series string,
	placement []string,
) (params.ControllersChanges, error) {
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{{
			NumControllers: numControllers,
			Constraints:    cons,
			Series:         series,
			Placement:      placement,
		}}}
	results, err := haServer.EnableHA(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	// We explicitly return nil here so we can do typed nil checking
	// of the result like normal.
	err = nil
	if result.Error != nil {
		err = result.Error
	}
	return result.Result, err
}

func (s *clientSuite) TestEnableHASeries(c *gc.C) {
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")

	enableHAResult, err := s.enableHA(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err = s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")

	enableHAResult, err = s.enableHA(c, 5, emptyCons, "non-default", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0", "machine-1", "machine-2"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-3", "machine-4"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	c.Assert(err, jc.ErrorIsNil)
	machines, err = s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 5)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")
	c.Assert(machines[1].Series(), gc.Equals, "quantal")
	c.Assert(machines[2].Series(), gc.Equals, "quantal")
	c.Assert(machines[3].Series(), gc.Equals, "non-default")
	c.Assert(machines[4].Series(), gc.Equals, "non-default")
}

func (s *clientSuite) TestEnableHAErrorForMultiCloudLocal(c *gc.C) {
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")

	err = machines[0].SetMachineAddresses(
		network.NewScopedSpaceAddress("cloud-local2.internal", network.ScopeCloudLocal),
		network.NewScopedSpaceAddress("cloud-local22.internal", network.ScopeCloudLocal),
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.enableHA(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 0"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHAErrorForNoCloudLocal(c *gc.C) {
	m0, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.Series(), gc.Equals, "quantal")

	// remove the extra provider addresses, so we have no valid CloudLocal addresses
	c.Assert(m0.SetProviderAddresses(
		network.NewScopedSpaceAddress("127.0.0.1", network.ScopeMachineLocal),
	), jc.ErrorIsNil)

	_, err = s.enableHA(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 0"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHANoErrorForNoAddresses(c *gc.C) {
	enableHAResult, err := s.enableHA(c, 0, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	s.setMachineAddresses(c, "0")
	s.setMachineAddresses(c, "1")
	// 0 and 1 are up, but 2 hasn't finished booting yet, so has no addresses set

	_, err = s.enableHA(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestEnableHAAddMachinesErrorForMultiCloudLocal(c *gc.C) {
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Series(), gc.Equals, "quantal")

	enableHAResult, err := s.enableHA(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})

	s.setMachineAddresses(c, "1")

	m, err := s.State.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMachineAddresses(
		network.NewScopedSpaceAddress("cloud-local2.internal", network.ScopeCloudLocal),
		network.NewScopedSpaceAddress("cloud-local22.internal", network.ScopeCloudLocal),
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.enableHA(c, 5, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 2"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHAConstraints(c *gc.C) {
	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G"), defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	expectedCons := []constraints.Value{
		controllerCons,
		constraints.MustParse("mem=4G"),
		constraints.MustParse("mem=4G"),
	}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(cons, gc.DeepEquals, expectedCons[i])
	}
}

func (s *clientSuite) TestEnableHAEmptyConstraints(c *gc.C) {
	enableHAResult, err := s.enableHA(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	for _, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(cons, gc.DeepEquals, controllerCons)
	}
}

func (s *clientSuite) TestEnableHAControllerConfigConstraints(c *gc.C) {
	controllerSettings, _ := s.State.ReadSettings("controllers", "controllerSettings")
	controllerSettings.Set(controller.JujuHASpace, "ha-space")
	controllerSettings.Write()

	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("spaces=random-space"), defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	expectedCons := []constraints.Value{
		controllerCons,
		constraints.MustParse("spaces=ha-space,random-space"),
		constraints.MustParse("spaces=ha-space,random-space"),
	}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(cons, gc.DeepEquals, expectedCons[i])
	}
}

func (s *clientSuite) TestBlockMakeHA(c *gc.C) {
	// Block all changes.
	s.BlockAllChanges(c, "TestBlockEnableHA")

	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G"), defaultSeries, nil)
	s.AssertBlocked(c, err, "TestBlockEnableHA")

	c.Assert(enableHAResult.Maintained, gc.HasLen, 0)
	c.Assert(enableHAResult.Added, gc.HasLen, 0)
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
}

func (s *clientSuite) TestEnableHAPlacement(c *gc.C) {
	placement := []string{"valid"}
	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G tags=foobar"), defaultSeries, placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	expectedCons := []constraints.Value{
		controllerCons,
		{},
		constraints.MustParse("mem=4G tags=foobar"),
	}
	expectedPlacement := []string{"", "valid", ""}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(cons, gc.DeepEquals, expectedCons[i])
		c.Check(m.Placement(), gc.Equals, expectedPlacement[i])
	}
}

func (s *clientSuite) TestEnableHAPlacementTo(c *gc.C) {
	machine1Cons := constraints.MustParse("mem=8G")
	_, err := s.State.AddMachines(state.MachineTemplate{
		Series:      "quantal",
		Jobs:        []state.MachineJob{state.JobHostUnits},
		Constraints: machine1Cons,
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	placement := []string{"1", "2"}
	enableHAResult, err := s.enableHA(c, 3, emptyCons, defaultSeries, placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.HasLen, 0)
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.DeepEquals, []string{"machine-1", "machine-2"})

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	expectedCons := []constraints.Value{
		controllerCons,
		machine1Cons,
		{},
	}
	expectedPlacement := []string{"", "", ""}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(cons, gc.DeepEquals, expectedCons[i])
		c.Check(m.Placement(), gc.Equals, expectedPlacement[i])
	}
}

func (s *clientSuite) TestEnableHAPlacementToWithAddressInSpace(c *gc.C) {
	sp, err := s.State.AddSpace("ha-space", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	controllerSettings, _ := s.State.ReadSettings("controllers", "controllerSettings")
	controllerSettings.Set(controller.JujuHASpace, "ha-space")
	_, err = controllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)

	m1, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	a1 := network.NewSpaceAddress("192.168.6.6")
	a1.SpaceID = sp.Id()
	err = m1.SetProviderAddresses(a1)
	c.Assert(err, jc.ErrorIsNil)

	m2, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	a2 := network.NewSpaceAddress("192.168.6.7")
	a2.SpaceID = sp.Id()
	err = m2.SetProviderAddresses(a1)
	c.Assert(err, jc.ErrorIsNil)

	placement := []string{"1", "2"}
	_, err = s.enableHA(c, 3, emptyCons, defaultSeries, placement)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestEnableHAPlacementToErrorForInaccessibleSpace(c *gc.C) {
	_, err := s.State.AddSpace("ha-space", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	controllerSettings, _ := s.State.ReadSettings("controllers", "controllerSettings")
	controllerSettings.Set(controller.JujuHASpace, "ha-space")
	_, err = controllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	placement := []string{"1", "2"}
	_, err = s.enableHA(c, 3, emptyCons, defaultSeries, placement)
	c.Assert(err, gc.ErrorMatches, `machine "1" has no addresses in space "ha-space"`)
}

func (s *clientSuite) TestEnableHA0Preserves(c *gc.C) {
	// A value of 0 says either "if I'm not HA, make me HA" or "preserve my
	// current HA settings".
	enableHAResult, err := s.enableHA(c, 0, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")

	// Now, we keep agent 1 alive, but not agent 2, calling
	// EnableHA(0) again will cause us to start another machine
	c.Assert(machines[2].Destroy(), jc.ErrorIsNil)
	c.Assert(machines[2].Refresh(), jc.ErrorIsNil)
	node, err := s.State.ControllerNode(machines[2].Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), jc.ErrorIsNil)
	c.Assert(node.Refresh(), jc.ErrorIsNil)
	c.Assert(s.State.RemoveControllerReference(node), jc.ErrorIsNil)
	c.Assert(machines[2].EnsureDead(), jc.ErrorIsNil)
	enableHAResult, err = s.enableHA(c, 0, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0", "machine-1"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-3"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err = s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 4)
}

func (s *clientSuite) TestEnableHA0Preserves5(c *gc.C) {
	// Start off with 5 servers
	enableHAResult, err := s.enableHA(c, 5, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2", "machine-3", "machine-4"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 5)
	nodes, err := s.State.ControllerNodes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodes, gc.HasLen, 5)
	for _, n := range nodes {
		c.Assert(n.SetHasVote(true), jc.ErrorIsNil)
	}

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")
	s.setMachineAddresses(c, "3")
	s.setMachineAddresses(c, "4")
	c.Assert(machines[4].Destroy(), jc.ErrorIsNil)
	c.Assert(machines[4].Refresh(), jc.ErrorIsNil)
	node, err := s.State.ControllerNode(machines[4].Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), jc.ErrorIsNil)
	c.Assert(node.Refresh(), jc.ErrorIsNil)
	c.Assert(s.State.RemoveControllerReference(node), jc.ErrorIsNil)
	c.Assert(machines[4].EnsureDead(), jc.ErrorIsNil)

	// Keeping all alive but one, will bring up 1 more server to preserve 5
	enableHAResult, err = s.enableHA(c, 0, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0", "machine-1",
		"machine-2", "machine-3"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-5"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err = s.State.AllMachines()
	c.Assert(machines, gc.HasLen, 6)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestEnableHAErrors(c *gc.C) {
	enableHAResult, err := s.enableHA(c, -1, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.ErrorMatches, "number of controllers must be odd and non-negative")

	enableHAResult, err = s.enableHA(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")

	_, err = s.enableHA(c, 1, emptyCons, defaultSeries, nil)
	c.Assert(err, gc.ErrorMatches, "failed to create new controller machines: cannot reduce controller count")
}

func (s *clientSuite) TestEnableHAHostedModelErrors(c *gc.C) {
	st2 := s.Factory.MakeModel(c, &factory.ModelParams{ConfigAttrs: coretesting.Attrs{"controller": false}})
	defer st2.Close()

	haServer, err := highavailability.NewHighAvailabilityAPI(st2, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)

	enableHAResult, err := enableHA(c, haServer, 3, constraints.MustParse("mem=4G"), defaultSeries, nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "unsupported with hosted models")

	c.Assert(enableHAResult.Maintained, gc.HasLen, 0)
	c.Assert(enableHAResult.Added, gc.HasLen, 0)
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := st2.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 0)
}

func (s *clientSuite) TestEnableHAMultipleSpecs(c *gc.C) {
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{
			{NumControllers: 3},
			{NumControllers: 5},
		},
	}
	results, err := s.haServer.EnableHA(arg)
	c.Check(err, gc.ErrorMatches, "only one controller spec is supported")
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *clientSuite) TestEnableHANoSpecs(c *gc.C) {
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{},
	}
	results, err := s.haServer.EnableHA(arg)
	c.Check(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *clientSuite) TestEnableHABootstrap(c *gc.C) {
	// Testing based on lp:1748275 - Juju HA fails due to demotion of Machine 0
	machines, err := s.State.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)

	enableHAResult, err := s.enableHA(c, 3, emptyCons, defaultSeries, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)
}

func (s *clientSuite) TestHighAvailabilityCAASFails(c *gc.C) {
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()

	_, err := highavailability.NewHighAvailabilityAPI(st, s.resources, s.authoriser)
	c.Assert(err, gc.ErrorMatches, "high availability on kubernetes controllers not supported")
}
