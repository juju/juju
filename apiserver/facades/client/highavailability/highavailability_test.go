// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

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

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

var (
	emptyCons      = constraints.Value{}
	controllerCons = constraints.MustParse("mem=16G cores=16")
)

func (s *clientSuite) SetUpTest(c *tc.C) {
	ctx := c.Context()
	s.ApiServerSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        testing.AdminUser,
		Controller: true,
	}
	st := s.ControllerModel(c).State()
	var err error
	s.haServer, err = highavailability.NewHighAvailabilityAPI(ctx, facadetest.ModelContext{
		State_:          st,
		Auth_:           s.authorizer,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	// We have to ensure the agents are alive, or EnableHA will create more to
	// replace them.
	_, err = st.AddMachines(
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
	c.Assert(err, tc.ErrorIsNil)

	s.store = testing.NewObjectStore(c, s.ControllerModelUUID())
}

func (s *clientSuite) enableS3(c *tc.C) {
	// HA Requires S3 to be setup, the default is to use file backed storage.
	// Forcing this to be s3 here, allows HA to be enabled.
	attrs := controller.Config{
		controller.ObjectStoreType:           string(objectstore.S3Backend),
		controller.ObjectStoreS3Endpoint:     "http://localhost:1234",
		controller.ObjectStoreS3StaticKey:    "deadbeef",
		controller.ObjectStoreS3StaticSecret: "shhh....",
	}
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	err := controllerConfigService.UpdateControllerConfig(c.Context(), attrs, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *clientSuite) setMachineAddresses(c *tc.C, machineId string) {
	st := s.ControllerModel(c).State()
	m, err := st.Machine(machineId)
	c.Assert(err, tc.ErrorIsNil)

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	err = m.SetMachineAddresses(
		controllerConfig,
		network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
		network.NewSpaceAddress(fmt.Sprintf("cloud-local%s.internal", machineId), network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress(fmt.Sprintf("fc0%s::1", machineId), network.WithScope(network.ScopePublic)),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *clientSuite) enableHA(
	c *tc.C, numControllers int, cons constraints.Value, placement []string,
) (params.ControllersChanges, error) {
	s.enableS3(c)
	return enableHA(c, s.haServer, numControllers, cons, placement)
}

func (s *clientSuite) enableHANoS3(
	c *tc.C, numControllers int, cons constraints.Value, placement []string,
) (params.ControllersChanges, error) {
	return enableHA(c, s.haServer, numControllers, cons, placement)
}

func enableHA(
	c *tc.C,
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
	results, err := haServer.EnableHA(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	// We explicitly return nil here so we can do typed nil checking
	// of the result like normal.
	err = nil
	if result.Error != nil {
		err = result.Error
	}
	return result.Result, err
}

func (s *clientSuite) TestEnableHAErrorForMultiCloudLocal(c *tc.C) {
	s.enableS3(c)
	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	c.Assert(machines[0].Base().DisplayString(), tc.Equals, "ubuntu@12.10")

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	err = machines[0].SetMachineAddresses(
		controllerConfig,
		network.NewSpaceAddress("cloud-local2.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("cloud-local22.internal", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 0"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHAErrorForNoCloudLocal(c *tc.C) {
	st := s.ControllerModel(c).State()
	m0, err := st.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m0.Base().DisplayString(), tc.Equals, "ubuntu@12.10")

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// remove the extra provider addresses, so we have no valid CloudLocal addresses
	c.Assert(m0.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("127.0.0.1", network.WithScope(network.ScopeMachineLocal)),
	), tc.ErrorIsNil)

	_, err = s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 0"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHANoErrorForNoAddresses(c *tc.C) {
	enableHAResult, err := s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	s.setMachineAddresses(c, "0")
	s.setMachineAddresses(c, "1")
	// 0 and 1 are up, but 2 hasn't finished booting yet, so has no addresses set

	_, err = s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnableHANoErrorVirtualAddresses verifies that virtual IPv4 addresses doesn't prevent enabling HA
// (see https://bugs.launchpad.net/juju/+bug/2073986)
func (s *clientSuite) TestEnableHANoErrorVirtualAddressesIpV4(c *tc.C) {
	st := s.ControllerModel(c).State()

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Add a virtual address to machine 0
	m, err := st.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	fakeIP := fmt.Sprintf("cloud-local-virtual%s.internal", "0")
	err = m.SetMachineAddresses(controllerConfig,
		network.NewSpaceAddress(fakeIP, network.WithScope(network.ScopeCloudLocal), network.WithCIDR(fmt.Sprintf("%s/32", fakeIP))),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnableHANoErrorVirtualAddressesIpV6 verifies that virtual IPv6 addresses doesn't prevent enabling HA
// (see https://bugs.launchpad.net/juju/+bug/2073986)
func (s *clientSuite) TestEnableHANoErrorVirtualAddressesIpV6(c *tc.C) {
	st := s.ControllerModel(c).State()

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	// Add a virtual address to machine 0
	m, err := st.Machine("0")
	c.Assert(err, tc.ErrorIsNil)
	fakeIP := "fd42:9102:88cb:dce3:216:3eff:fef7:4c4b"
	err = m.SetMachineAddresses(controllerConfig,
		network.NewSpaceAddress(fakeIP, network.WithScope(network.ScopeCloudLocal), network.WithCIDR(fmt.Sprintf("%s/128", fakeIP))),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *clientSuite) TestEnableHAAddMachinesErrorForMultiCloudLocal(c *tc.C) {
	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
	c.Assert(machines[0].Base().String(), tc.Equals, "ubuntu@12.10/stable")

	enableHAResult, err := s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})

	s.setMachineAddresses(c, "1")

	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	m, err := st.Machine("2")
	c.Assert(err, tc.ErrorIsNil)
	err = m.SetMachineAddresses(
		controllerConfig,
		network.NewSpaceAddress("cloud-local2.internal", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("cloud-local22.internal", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.enableHA(c, 5, emptyCons, nil)
	c.Assert(err, tc.ErrorMatches,
		"juju-ha-space is not set and a unique usable address was not found for machines: 2"+
			"\nrun \"juju controller-config juju-ha-space=<name>\" to set a space for Mongo peer communication")
}

func (s *clientSuite) TestEnableHAConstraints(c *tc.C) {
	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	machines, err := s.ControllerModel(c).State().AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 3)
	expectedCons := []constraints.Value{
		controllerCons,
		constraints.MustParse("mem=4G"),
		constraints.MustParse("mem=4G"),
	}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(cons, tc.DeepEquals, expectedCons[i])
	}
}

func (s *clientSuite) TestEnableHAEmptyConstraints(c *tc.C) {
	enableHAResult, err := s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	machines, err := s.ControllerModel(c).State().AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 3)
	for _, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(cons, tc.DeepEquals, controllerCons)
	}
}

func (s *clientSuite) TestEnableHAControllerConfigConstraints(c *tc.C) {
	attrs := controller.Config{controller.JujuHASpace: "ha-space"}
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	err := controllerConfigService.UpdateControllerConfig(c.Context(), attrs, nil)
	c.Assert(err, tc.ErrorIsNil)

	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("spaces=random-space"), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 3)
	expectedCons := []constraints.Value{
		controllerCons,
		constraints.MustParse("spaces=ha-space,random-space"),
		constraints.MustParse("spaces=ha-space,random-space"),
	}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(cons, tc.DeepEquals, expectedCons[i])
	}
}

func (s *clientSuite) TestEnableHAControllerConfigWithFileBackedObjectStore(c *tc.C) {
	attrs := controller.Config{controller.ObjectStoreType: "file"}
	controllerConfigService := s.ControllerDomainServices(c).ControllerConfig()
	err := controllerConfigService.UpdateControllerConfig(c.Context(), attrs, nil)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.enableHANoS3(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorMatches, `cannot enable-ha with filesystem backed object store`)
}

func (s *clientSuite) TestBlockMakeHA(c *tc.C) {
	// Block all changes.
	domainServices := s.ControllerDomainServices(c)
	blockCommandService := domainServices.BlockCommand()
	err := blockCommandService.SwitchBlockOn(c.Context(), blockcommand.ChangeBlock, "TestBlockEnableHA")
	c.Assert(err, tc.ErrorIsNil)

	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G"), nil)
	c.Assert(err, tc.ErrorMatches, "TestBlockEnableHA")

	c.Check(enableHAResult.Maintained, tc.HasLen, 0)
	c.Check(enableHAResult.Added, tc.HasLen, 0)
	c.Check(enableHAResult.Removed, tc.HasLen, 0)
	c.Check(enableHAResult.Converted, tc.HasLen, 0)

	machines, err := s.ControllerModel(c).State().AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)
}

func (s *clientSuite) TestEnableHAPlacement(c *tc.C) {
	placement := []string{"valid"}
	enableHAResult, err := s.enableHA(c, 3, constraints.MustParse("mem=4G tags=foobar"), placement)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	machines, err := s.ControllerModel(c).State().AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 3)
	expectedCons := []constraints.Value{
		controllerCons,
		{},
		constraints.MustParse("mem=4G tags=foobar"),
	}
	expectedPlacement := []string{"", "valid", ""}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(cons, tc.DeepEquals, expectedCons[i])
		c.Check(m.Placement(), tc.Equals, expectedPlacement[i])
	}
}

func (s *clientSuite) TestEnableHAPlacementTo(c *tc.C) {
	st := s.ControllerModel(c).State()
	machine1Cons := constraints.MustParse("mem=8G")
	_, err := st.AddMachines(
		state.MachineTemplate{
			Base:        state.UbuntuBase("12.10"),
			Jobs:        []state.MachineJob{state.JobHostUnits},
			Constraints: machine1Cons,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, tc.ErrorIsNil)

	placement := []string{"1", "2"}
	enableHAResult, err := s.enableHA(c, 3, emptyCons, placement)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.HasLen, 0)
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.DeepEquals, []string{"machine-1", "machine-2"})

	machines, err := st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 3)
	expectedCons := []constraints.Value{
		controllerCons,
		machine1Cons,
		{},
	}
	expectedPlacement := []string{"", "", ""}
	for i, m := range machines {
		cons, err := m.Constraints()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(cons, tc.DeepEquals, expectedCons[i])
		c.Check(m.Placement(), tc.Equals, expectedPlacement[i])
	}
}

func (s *clientSuite) TestEnableHA0Preserves(c *tc.C) {
	// A value of 0 says either "if I'm not HA, make me HA" or "preserve my
	// current HA settings".
	enableHAResult, err := s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 3)

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")

	// Now, we keep agent 1 alive, but not agent 2, calling
	// EnableHA(0) again will cause us to start another machine
	c.Assert(machines[2].Destroy(s.store), tc.ErrorIsNil)
	c.Assert(machines[2].Refresh(), tc.ErrorIsNil)
	node, err := st.ControllerNode(machines[2].Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), tc.ErrorIsNil)
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	c.Assert(st.RemoveControllerReference(node), tc.ErrorIsNil)
	c.Assert(machines[2].EnsureDead(), tc.ErrorIsNil)
	enableHAResult, err = s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0", "machine-1"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-3"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	machines, err = st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 4)
}

func (s *clientSuite) TestEnableHA0Preserves5(c *tc.C) {
	// Start off with 5 servers
	enableHAResult, err := s.enableHA(c, 5, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2", "machine-3", "machine-4"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	st := s.ControllerModel(c).State()
	machines, err := st.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 5)
	nodes, err := st.ControllerNodes()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nodes, tc.HasLen, 5)
	for _, n := range nodes {
		c.Assert(n.SetHasVote(true), tc.ErrorIsNil)
	}

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")
	s.setMachineAddresses(c, "3")
	s.setMachineAddresses(c, "4")
	c.Assert(machines[4].Destroy(s.store), tc.ErrorIsNil)
	c.Assert(machines[4].Refresh(), tc.ErrorIsNil)
	node, err := st.ControllerNode(machines[4].Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(node.SetHasVote(false), tc.ErrorIsNil)
	c.Assert(node.Refresh(), tc.ErrorIsNil)
	c.Assert(st.RemoveControllerReference(node), tc.ErrorIsNil)
	c.Assert(machines[4].EnsureDead(), tc.ErrorIsNil)

	// Keeping all alive but one, will bring up 1 more server to preserve 5
	enableHAResult, err = s.enableHA(c, 0, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0", "machine-1",
		"machine-2", "machine-3"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-5"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	machines, err = st.AllMachines()
	c.Assert(machines, tc.HasLen, 6)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *clientSuite) TestEnableHAErrors(c *tc.C) {
	_, err := s.enableHA(c, -1, emptyCons, nil)
	c.Assert(err, tc.ErrorMatches, "number of controllers must be odd and non-negative")

	enableHAResult, err := s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	s.setMachineAddresses(c, "1")
	s.setMachineAddresses(c, "2")

	_, err = s.enableHA(c, 1, emptyCons, nil)
	c.Assert(err, tc.ErrorMatches, "failed to enable HA with 1 controllers: cannot remove controllers with enable-ha, use remove-machine and chose the controller\\(s\\) to remove")
}

func (s *clientSuite) TestEnableHAHostedModelErrors(c *tc.C) {
	ctx := c.Context()
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	st2 := f.MakeModel(c, &factory.ModelParams{ConfigAttrs: coretesting.Attrs{"controller": false}})
	defer st2.Close()

	haServer, err := highavailability.NewHighAvailabilityAPI(ctx, facadetest.ModelContext{
		State_:          st2,
		Auth_:           s.authorizer,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	enableHAResult, err := enableHA(c, haServer, 3, constraints.MustParse("mem=4G"), nil)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "unsupported with workload models")

	c.Assert(enableHAResult.Maintained, tc.HasLen, 0)
	c.Assert(enableHAResult.Added, tc.HasLen, 0)
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)

	machines, err := st2.AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 0)
}

func (s *clientSuite) TestEnableHAMultipleSpecs(c *tc.C) {
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{
			{NumControllers: 3},
			{NumControllers: 5},
		},
	}
	results, err := s.haServer.EnableHA(c.Context(), arg)
	c.Check(err, tc.ErrorMatches, "only one controller spec is supported")
	c.Check(results.Results, tc.HasLen, 0)
}

func (s *clientSuite) TestEnableHANoSpecs(c *tc.C) {
	arg := params.ControllersSpecs{
		Specs: []params.ControllersSpec{},
	}
	results, err := s.haServer.EnableHA(c.Context(), arg)
	c.Check(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 0)
}

func (s *clientSuite) TestEnableHABootstrap(c *tc.C) {
	// Testing based on lp:1748275 - Juju HA fails due to demotion of Machine 0
	machines, err := s.ControllerModel(c).State().AllMachines()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines, tc.HasLen, 1)

	enableHAResult, err := s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(enableHAResult.Maintained, tc.DeepEquals, []string{"machine-0"})
	c.Assert(enableHAResult.Added, tc.DeepEquals, []string{"machine-1", "machine-2"})
	c.Assert(enableHAResult.Removed, tc.HasLen, 0)
	c.Assert(enableHAResult.Converted, tc.HasLen, 0)
}

func (s *clientSuite) TestHighAvailabilityCAASFails(c *tc.C) {
	c.Skip("TODO - reimplement when facade moved off of mongo")
	ctx := c.Context()
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	st := f.MakeCAASModel(c, nil)
	defer st.Close()

	_, err := highavailability.NewHighAvailabilityAPI(ctx, facadetest.ModelContext{
		State_:          st,
		Auth_:           s.authorizer,
		DomainServices_: s.ControllerDomainServices(c),
	})
	c.Assert(err, tc.ErrorMatches, "high availability on kubernetes controllers not supported")
}

func (s *clientSuite) TestControllerDetails(c *tc.C) {
	cfg, err := s.ControllerDomainServices(c).ControllerConfig().ControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	apiPort := cfg.APIPort()

	_, err = s.enableHA(c, 3, emptyCons, nil)
	c.Assert(err, tc.ErrorIsNil)

	d, err := s.haServer.ControllerDetails(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(d, tc.DeepEquals, params.ControllerDetailsResults{
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
