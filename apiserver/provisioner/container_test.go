// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	jc "github.com/juju/testing/checkers"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/provisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type containerProvisionerSuite struct {
	provisionerSuite
}

var _ = gc.Suite(&containerProvisionerSuite{})

func (s *containerProvisionerSuite) SetUpTest(c *gc.C) {
	// We have a Controller machine, and 5 other machines to provision in
	s.setUpTest(c, true)
}

func addContainerToMachine(c *gc.C, st *state.State, machine *state.Machine) *state.Machine {
	// Add a container machine with machine as its host.
	containerTemplate := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	container, err := st.AddMachineInsideMachine(containerTemplate, machine.Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	return container
}

func (s *containerProvisionerSuite) TestPrepareContainerInterfaceInfoPermission(c *gc.C) {
	// Login as a machine agent for machine 1, which has a container put on it
	addContainerToMachine(c, s.State, s.machines[1])
	addContainerToMachine(c, s.State, s.machines[1])
	addContainerToMachine(c, s.State, s.machines[2])

	anAuthorizer := s.authorizer
	anAuthorizer.EnvironManager = false
	anAuthorizer.Tag = s.machines[1].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

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
	results, err := aProvisioner.PrepareContainerInterfaceInfo(args)
	c.Assert(err, gc.ErrorMatches, "dummy provider network config not supported")
	c.Skip("dummy provider needs networking interface")
	// Overall request is ok
	c.Assert(err, jc.ErrorIsNil)

	errors := make([]*params.Error, 0)
	c.Check(results.Results, gc.HasLen, 4)
	for _, configResult := range results.Results {
		errors = append(errors, configResult.Error)
	}
	c.Check(errors, gc.DeepEquals, []*params.Error{
		nil, // can touch 1/lxd/0
		nil, // can touch 1/lxd/1
		apiservertesting.ErrUnauthorized, // not 2/lxd/0
		apiservertesting.ErrUnauthorized, // nor 2
	})
}

// TODO(jam): Add a test for requesting PrepareContainerInterfaceInfo with a
// machine that is not yet provisioned.

func (s *containerProvisionerSuite) TestHostChangesForContainersPermission(c *gc.C) {
	// Login as a machine agent for machine 1, which has a container put on it
	addContainerToMachine(c, s.State, s.machines[1])
	addContainerToMachine(c, s.State, s.machines[1])
	addContainerToMachine(c, s.State, s.machines[2])

	anAuthorizer := s.authorizer
	anAuthorizer.EnvironManager = false
	anAuthorizer.Tag = s.machines[1].Tag()
	aProvisioner, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(aProvisioner, gc.NotNil)

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
	results, err := aProvisioner.HostChangesForContainers(args)
	c.Assert(err, gc.ErrorMatches, "dummy provider network config not supported")
	c.Skip("dummy provider needs networking interface")
	// Overall request is ok
	c.Assert(err, jc.ErrorIsNil)

	errors := make([]*params.Error, 0)
	c.Check(results.Results, gc.HasLen, 4)
	for _, configResult := range results.Results {
		errors = append(errors, configResult.Error)
	}
	c.Check(errors, gc.DeepEquals, []*params.Error{
		nil, // can touch 1/lxd/0
		nil, // can touch 1/lxd/1
		apiservertesting.ErrUnauthorized, // not 2/lxd/0
		apiservertesting.ErrUnauthorized, // nor 2
	})
}

// TODO(jam): Add a test for requesting HostChangesForContainers with a
// machine that is not yet provisioned.
