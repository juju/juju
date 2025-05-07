// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type containerProvisionerSuite struct {
	provisionerSuite
}

var _ = tc.Suite(&containerProvisionerSuite{})

func (s *containerProvisionerSuite) SetUpTest(c *tc.C) {
	// We have a Controller machine, and 5 other machines to provision in
	s.setUpTest(c, true)
}

func addContainerToMachine(
	c *tc.C,
	st *state.State,
	machine *state.Machine,
) *state.Machine {
	// Add a container machine with machine as its host.
	containerTemplate := state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	}
	container, err := st.AddMachineInsideMachine(containerTemplate, machine.Id(), instance.LXD)
	c.Assert(err, tc.ErrorIsNil)
	return container
}

func (s *containerProvisionerSuite) TestPrepareContainerInterfaceInfoPermission(c *tc.C) {
	c.Skip("dummy provider needs networking https://pad.lv/1651974")

	// Login as a machine agent for machine 1, which has a container put on it
	st := s.ControllerModel(c).State()
	addContainerToMachine(c, st, s.machines[1])
	addContainerToMachine(c, st, s.machines[1])
	addContainerToMachine(c, st, s.machines[2])

	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[1].Tag()
	aProvisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{{
			Tag: "machine-1/lxd/0", // valid
		}, {
			Tag: "machine-1/lxd/1", // valid
		}, {
			Tag: "machine-2/lxd/0", // wrong host machine
		}, {
			Tag: "machine-2", // host machine
		}, {
			Tag: "unit-mysql-0", // not a valid machine tag
		}}}
	// Only machine 0 can have its containers updated.
	results, err := aProvisioner.PrepareContainerInterfaceInfo(context.Background(), args)
	c.Assert(err, tc.ErrorMatches, "dummy provider network config not supported")
	// Overall request is ok
	c.Assert(err, tc.ErrorIsNil)

	errors := make([]*params.Error, 0)
	c.Check(results.Results, tc.HasLen, 4)
	for _, configResult := range results.Results {
		errors = append(errors, configResult.Error)
	}
	c.Check(errors, tc.DeepEquals, []*params.Error{
		nil,                              // can touch 1/lxd/0
		nil,                              // can touch 1/lxd/1
		apiservertesting.ErrUnauthorized, // not 2/lxd/0
		apiservertesting.ErrUnauthorized, // nor 2
	})
}

// TODO(jam): Add a test for requesting PrepareContainerInterfaceInfo with a
// machine that is not yet provisioned.

func (s *containerProvisionerSuite) TestHostChangesForContainersPermission(c *tc.C) {
	c.Skip("dummy provider needs networking https://pad.lv/1651974")

	// Login as a machine agent for machine 1, which has a container put on it
	st := s.ControllerModel(c).State()
	addContainerToMachine(c, st, s.machines[1])
	addContainerToMachine(c, st, s.machines[1])
	addContainerToMachine(c, st, s.machines[2])

	anAuthorizer := s.authorizer
	anAuthorizer.Controller = false
	anAuthorizer.Tag = s.machines[1].Tag()
	aProvisioner, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           anAuthorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(aProvisioner, tc.NotNil)

	args := params.Entities{
		Entities: []params.Entity{{
			Tag: "machine-1/lxd/0", // valid
		}, {
			Tag: "machine-1/lxd/1", // valid
		}, {
			Tag: "machine-2/lxd/0", // wrong host machine
		}, {
			Tag: "machine-2", // host machine
		}, {
			Tag: "unit-mysql-0", // not a valid machine tag
		}}}
	// Only machine 0 can have it's containers updated.
	results, err := aProvisioner.HostChangesForContainers(context.Background(), args)
	c.Assert(err, tc.ErrorMatches, "dummy provider network config not supported")

	// Overall request is ok
	c.Assert(err, tc.ErrorIsNil)

	errors := make([]*params.Error, 0)
	c.Check(results.Results, tc.HasLen, 4)
	for _, configResult := range results.Results {
		errors = append(errors, configResult.Error)
	}
	c.Check(errors, tc.DeepEquals, []*params.Error{
		nil,                              // can touch 1/lxd/0
		nil,                              // can touch 1/lxd/1
		apiservertesting.ErrUnauthorized, // not 2/lxd/0
		apiservertesting.ErrUnauthorized, // nor 2
	})
}

// TODO(jam): Add a test for requesting HostChangesForContainers with a
// machine that is not yet provisioned.
