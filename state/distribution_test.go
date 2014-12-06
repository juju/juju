// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
)

type InstanceDistributorSuite struct {
	ConnSuite
	distributor mockInstanceDistributor
	wordpress   *state.Service
	machines    []*state.Machine
}

var _ = gc.Suite(&InstanceDistributorSuite{})

type mockInstanceDistributor struct {
	candidates        []instance.Id
	distributionGroup []instance.Id
	result            []instance.Id
	err               error
}

func (p *mockInstanceDistributor) DistributeInstances(candidates, distributionGroup []instance.Id) ([]instance.Id, error) {
	p.candidates = candidates
	p.distributionGroup = distributionGroup
	result := p.result
	if result == nil {
		result = candidates
	}
	return result, p.err
}

func (s *InstanceDistributorSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.distributor = mockInstanceDistributor{}
	s.policy.GetInstanceDistributor = func(*config.Config) (state.InstanceDistributor, error) {
		return &s.distributor, nil
	}
	s.wordpress = s.AddTestingServiceWithNetworks(
		c,
		"wordpress",
		s.AddTestingCharm(c, "wordpress"),
		[]string{"net1", "net2"},
	)
	s.wordpress.SetConstraints(constraints.MustParse("networks=net3,^net4,^net5"))
	s.machines = make([]*state.Machine, 3)
	for i := range s.machines {
		var err error
		s.machines[i], err = s.State.AddOneMachine(state.MachineTemplate{
			Series: "quantal",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		})
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *InstanceDistributorSuite) setupScenario(c *gc.C) {
	// Assign a unit so we have a non-empty distribution group, and
	// provision all instances so we have candidates.
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machines[0])
	c.Assert(err, jc.ErrorIsNil)
	for i, m := range s.machines {
		instId := instance.Id(fmt.Sprintf("i-blah-%d", i))
		err = m.SetProvisioned(instId, "fake-nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *InstanceDistributorSuite) TestDistributeInstances(c *gc.C) {
	s.setupScenario(c)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.distributor.candidates, jc.SameContents, []instance.Id{"i-blah-1", "i-blah-2"})
	c.Assert(s.distributor.distributionGroup, jc.SameContents, []instance.Id{"i-blah-0"})
	s.distributor.result = []instance.Id{}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesInvalidInstances(c *gc.C) {
	s.setupScenario(c)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	s.distributor.result = []instance.Id{"notthere"}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/1" to clean machine: invalid instance returned: notthere`)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesNoEmptyMachines(c *gc.C) {
	for i := range s.machines {
		// Assign a unit so we have a non-empty distribution group.
		unit, err := s.wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		m, err := unit.AssignToCleanMachine()
		c.Assert(err, jc.ErrorIsNil)
		instId := instance.Id(fmt.Sprintf("i-blah-%d", i))
		err = m.SetProvisioned(instId, "fake-nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
	}

	// InstanceDistributor is not called if there are no empty instances.
	s.distributor.err = fmt.Errorf("no assignment for you")
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesErrors(c *gc.C) {
	s.setupScenario(c)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that assignment fails when DistributeInstances returns an error.
	s.distributor.err = fmt.Errorf("no assignment for you")
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, ".*no assignment for you")
	_, err = unit.AssignToCleanEmptyMachine()
	c.Assert(err, gc.ErrorMatches, ".*no assignment for you")
	// If the policy's InstanceDistributor method fails, that will be returned first.
	s.policy.GetInstanceDistributor = func(*config.Config) (state.InstanceDistributor, error) {
		return nil, fmt.Errorf("incapable of InstanceDistributor")
	}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, ".*incapable of InstanceDistributor")
}

func (s *InstanceDistributorSuite) TestDistributeInstancesEmptyDistributionGroup(c *gc.C) {
	s.distributor.err = fmt.Errorf("no assignment for you")

	// InstanceDistributor is not called if the distribution group is empty.
	unit0, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit0.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)

	// Distribution group is still empty, because the machine assigned to has
	// not been provisioned.
	unit1, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit1.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestInstanceDistributorUnimplemented(c *gc.C) {
	s.setupScenario(c)
	var distributorErr error
	s.policy.GetInstanceDistributor = func(*config.Config) (state.InstanceDistributor, error) {
		return nil, distributorErr
	}
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/1" to clean machine: policy returned nil instance distributor without an error`)
	distributorErr = errors.NotImplementedf("InstanceDistributor")
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesNoPolicy(c *gc.C) {
	s.policy.GetInstanceDistributor = func(*config.Config) (state.InstanceDistributor, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}
	state.SetPolicy(s.State, nil)
	unit, err := s.wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
}
