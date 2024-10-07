// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/highavailability"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/blockcommand"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type clientSuite struct {
	testing.ApiServerSuite

	authorizer apiservertesting.FakeAuthorizer
	haServer   *highavailability.HighAvailabilityAPI

	store objectstore.ObjectStore
}

var _ = gc.Suite(&clientSuite{})

var (
	emptyCons      = constraints.Value{}
	controllerCons = constraints.MustParse("mem=16G cores=16")
)

// modelConfigService is a convenience function to get the controller model's
// model config service inside a test.
func (s *clientSuite) modelConfigService(c *gc.C) common.ModelConfigService {
	return s.ControllerDomainServices(c).Config()
}

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        testing.AdminUser,
		Controller: true,
	}
	st := s.ControllerModel(c).State()
	var err error
	s.haServer, err = highavailability.NewHighAvailabilityAPI(facadetest.ModelContext{
		State_:          st,
		Auth_:           s.authorizer,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	// We have to ensure the agents are alive, or EnableHA will create more to
	// replace them.
	_, err = st.AddMachines(
		s.modelConfigService(c),
		state.MachineTemplate{
			Base:        state.UbuntuBase("12.10"),
			Jobs:        []state.MachineJob{state.JobManageModel},
			Constraints: controllerCons,
			Addresses: []network.SpaceAddress{
				network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
				network.NewSpaceAddress("cloud-local0.internal", network.WithScope(network.ScopeCloudLocal)),
				network.NewSpaceAddress("fc00::0", network.WithScope(network.ScopePublic)),
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.store = testing.NewObjectStore(c, s.ControllerModelUUID())
}

func (s *clientSuite) enableS3(c *gc.C) {
	// HA Requires S3 to be setup, the default is to use file backed storage.
	// Forcing this to be s3 here, allows HA to be enabled.
	attrs := controller.Config{
		controller.ObjectStoreType:           string(objectstore.S3Backend),
		controller.ObjectStoreS3Endpoint:     "http://localhost:1234",
		controller.ObjectStoreS3StaticKey:    "deadbeef",
		controller.ObjectStoreS3StaticSecret: "shhh....",
	}
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	err := controllerConfigService.UpdateControllerConfig(context.Background(), attrs, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) setMachineAddresses(c *gc.C, machineId string) {
	st := s.ControllerModel(c).State()
	m, err := st.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMachineAddresses(
		controllerConfig,
		network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress(fmt.Sprintf("cloud-local%s.internal", machineId), network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress(fmt.Sprintf("fc0%s::1", machineId), network.WithScope(network.ScopePublic)),
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) enableHA(
	c *gc.C, numControllers int, cons constraints.Value, placement []string,
) (params.ControllersChanges, error) {
	s.enableS3(c)
	return enableHA(c, s.haServer, numControllers, cons, placement)
}

func (s *clientSuite) enableHANoS3(
	c *gc.C, numControllers int, cons constraints.Value, placement []string,
) (params.ControllersChanges, error) {
	return enableHA(c, s.haServer, numControllers, cons, placement)
}

func enableHA(
	c *gc.C,
	haServer *highavailability.HighAvailabilityAPI,
	numControllers int,
	cons constraints.Value,
	placement []string,
) (params.ControllersChanges, error) {
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{{
			NumControllers: numControllers,
			Constraints:    cons,
			Placement:      placement,
		}}}
	results, err := haServer.EnableHA(context.Background(), arg)
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

func (s *clientSuite) TestEnableHAErrorForMultiCloudLocal(c *gc.C) {
	s.enableS3(c)
	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Base().DisplayString(), gc.Equals, "ubuntu@12.10")

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	err = machines[0].SetMachineAddresses(
		controllerConfig,
		network.NewSpaceAddress("cloud-local2.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("cloud-local22.internal", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, gc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 0"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHAErrorForNoCloudLocal(c *gc.C) {
	st := s.ControllerModel(c).State()
	m0, err := st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.Base().DisplayString(), gc.Equals, "ubuntu@12.10")

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	// remove the extra provider addresses, so we have no valid CloudLocal addresses
	c.Assert(m0.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
	), jc.ErrorIsNil)

	_, err = s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, gc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 0"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHANoErrorForNoAddresses(c *gc.C) {
	enableHAResult, err := s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	s.setMachineAddresses(c, "0")
	s.setMachineAddresses(c, "1")
	// 0 and 1 are up, but 2 hasn't finished booting yet, so has no addresses set

	_, err = s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestEnableHAAddMachinesErrorForMultiCloudLocal(c *gc.C) {
	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Base().String(), gc.Equals, "ubuntu@12.10/stable")

	enableHAResult, err := s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})

	s.setMachineAddresses(c, "1")

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	m, err := st.Machine("2")
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMachineAddresses(
		controllerConfig,
		network.NewSpaceAddress("cloud-local2.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("cloud-local22.internal", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.enableHA(c, 5, emptyCons, nil)
	c.Assert(err, gc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 2"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHAConstraints(c *gc.C) {
	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G"), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.ControllerModel(c).State().AllMachines()
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
	enableHAResult, err := s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.ControllerModel(c).State().AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)
	for _, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(cons, gc.DeepEquals, controllerCons)
	}
}

func (s *clientSuite) TestEnableHAControllerConfigConstraints(c *gc.C) {
	attrs := controller.Config{controller.JujuHASpace: "ha-space"}
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	err := controllerConfigService.UpdateControllerConfig(context.Background(), attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("spaces=random-space"), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
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

func (s *clientSuite) TestEnableHAControllerConfigWithFileBackedObjectStore(c *gc.C) {
	attrs := controller.Config{controller.ObjectStoreType: "file"}
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	err := controllerConfigService.UpdateControllerConfig(context.Background(), attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.enableHANoS3(c, 3, emptyCons, nil)
	c.Assert(err, gc.ErrorMatches, `cannot enable-ha with filesystem backed object store`)
}

func (s *clientSuite) TestBlockMakeHA(c *gc.C) {
	// Block all changes.
	domainServices := s.ControllerDomainServices(c)
	blockCommandService := domainServices.BlockCommand()
	err := blockCommandService.SwitchBlockOn(context.Background(), blockcommand.ChangeBlock, "TestBlockEnableHA")
	c.Assert(err, jc.ErrorIsNil)

	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G"), nil)
	c.Assert(err, gc.ErrorMatches, "TestBlockEnableHA")

	c.Check(enableHAResult.Maintained, gc.HasLen, 0)
	c.Check(enableHAResult.Added, gc.HasLen, 0)
	c.Check(enableHAResult.Removed, gc.HasLen, 0)
	c.Check(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.ControllerModel(c).State().AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)
}

func (s *clientSuite) TestEnableHAPlacement(c *gc.C) {
	placement := []string{"valid"}
	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G tags=foobar"), placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err := s.ControllerModel(c).State().AllMachines()
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
	st := s.ControllerModel(c).State()
	machine1Cons := constraints.MustParse("mem=8G")
	_, err := st.AddMachines(
		s.modelConfigService(c),
		state.MachineTemplate{
			Base:        state.UbuntuBase("12.10"),
			Jobs:        []state.MachineJob{state.JobHostUnits},
			Constraints: machine1Cons,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.AddMachine(s.modelConfigService(c), state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	placement := []string{"1", "2"}
	enableHAResult, err := s.enableHA(c, 3, emptyCons, placement)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.HasLen, 0)
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.DeepEquals, []string{"machine-1", "machine-2"})

	machines, err := st.AllMachines()
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

func (s *clientSuite) TestEnableHA0Preserves(c *gc.C) {
	// A value of 0 says either "if I'm not HA, make me HA" or "preserve my
	// current HA settings".
	enableHAResult, err := s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 3)

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")

	// Now, we keep agent 1 alive, but not agent 2, calling
	// EnableHA(0) again will cause us to start another machine
	c.Assert(machines[2].Destroy(s.store), jc.ErrorIsNil)
	c.Assert(machines[2].Refresh(), jc.ErrorIsNil)
	node, err := st.ControllerNode(machines[2].Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), jc.ErrorIsNil)
	c.Assert(node.Refresh(), jc.ErrorIsNil)
	c.Assert(st.RemoveControllerReference(node), jc.ErrorIsNil)
	c.Assert(machines[2].EnsureDead(), jc.ErrorIsNil)
	enableHAResult, err = s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0", "machine-1"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-3"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err = st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 4)
}

func (s *clientSuite) TestEnableHA0Preserves5(c *gc.C) {
	// Start off with 5 servers
	enableHAResult, err := s.enableHA(c, 5, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2", "machine-3", "machine-4"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 5)
	nodes, err := st.ControllerNodes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nodes, gc.HasLen, 5)
	for _, n := range nodes {
		c.Assert(n.SetHasVote(true), jc.ErrorIsNil)
	}

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")
	s.setMachineAddresses(c, "3")
	s.setMachineAddresses(c, "4")
	c.Assert(machines[4].Destroy(s.store), jc.ErrorIsNil)
	c.Assert(machines[4].Refresh(), jc.ErrorIsNil)
	node, err := st.ControllerNode(machines[4].Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), jc.ErrorIsNil)
	c.Assert(node.Refresh(), jc.ErrorIsNil)
	c.Assert(st.RemoveControllerReference(node), jc.ErrorIsNil)
	c.Assert(machines[4].EnsureDead(), jc.ErrorIsNil)

	// Keeping all alive but one, will bring up 1 more server to preserve 5
	enableHAResult, err = s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0", "machine-1",
		"machine-2", "machine-3"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-5"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	machines, err = st.AllMachines()
	c.Assert(machines, gc.HasLen, 6)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestEnableHAErrors(c *gc.C) {
	_, err := s.enableHA(c, -1, emptyCons, nil)
	c.Assert(err, gc.ErrorMatches, "number of controllers must be odd and non-negative")

	enableHAResult, err := s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")

	_, err = s.enableHA(c, 1, emptyCons, nil)
	c.Assert(err, gc.ErrorMatches, "failed to enable HA with 1 controllers: cannot remove controllers with enable-ha, use remove-machine and chose the controller\\(s\\) to remove")
}

func (s *clientSuite) TestEnableHAHostedModelErrors(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	st2 := f.MakeModel(c, &factory.ModelParams{ConfigAttrs: coretesting.Attrs{"controller": false}})
	defer st2.Close()

	haServer, err := highavailability.NewHighAvailabilityAPI(facadetest.ModelContext{
		State_:          st2,
		Auth_:           s.authorizer,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, jc.ErrorIsNil)

	enableHAResult, err := enableHA(c, haServer, 3, constraints.MustParse("mem=4G"), nil)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "unsupported with workload models")

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
	results, err := s.haServer.EnableHA(context.Background(), arg)
	c.Check(err, gc.ErrorMatches, "only one controller spec is supported")
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *clientSuite) TestEnableHANoSpecs(c *gc.C) {
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{},
	}
	results, err := s.haServer.EnableHA(context.Background(), arg)
	c.Check(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 0)
}

func (s *clientSuite) TestEnableHABootstrap(c *gc.C) {
	// Testing based on lp:1748275 - Juju HA fails due to demotion of Machine 0
	machines, err := s.ControllerModel(c).State().AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 1)

	enableHAResult, err := s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, gc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, gc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, gc.HasLen, 0)
	c.Assert(enableHAResult.Converted, gc.HasLen, 0)
}

func (s *clientSuite) TestHighAvailabilityCAASFails(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	st := f.MakeCAASModel(c, nil)
	defer st.Close()

	_, err := highavailability.NewHighAvailabilityAPI(facadetest.ModelContext{
		State_:          st,
		Auth_:           s.authorizer,
		DomainServices_: s.ControllerDomainServices(c),
	})
	c.Assert(err, gc.ErrorMatches, "high availability on kubernetes controllers not supported")
}

func (s *clientSuite) TestControllerDetails(c *gc.C) {
	cfg, err := s.ControllerDomainServices(c).ControllerConfig().ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	apiPort := cfg.APIPort()

	_, err = s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, jc.ErrorIsNil)

	d, err := s.haServer.ControllerDetails(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d, jc.DeepEquals, params.ControllerDetailsResults{
		Results: []params.ControllerDetails{
			{ControllerId: "0", APIAddresses: []string{
				fmt.Sprintf("cloud-local0.internal:%d", apiPort),
				fmt.Sprintf("[fc00::0]:%d", apiPort)}},
			// No addresses for machines 1 and 2 yet.
			{ControllerId: "1"},
			{ControllerId: "2"},
		},
	})
}
