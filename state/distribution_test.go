// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/state"
)

type InstanceDistributorSuite struct {
	ConnSuite
	distributor mockInstanceDistributor
	wordpress   *state.Application
	machines    []*state.Machine
	hwChar      *instance.HardwareCharacteristics
}

var _ = gc.Suite(&InstanceDistributorSuite{})

type mockInstanceDistributor struct {
	candidates        []instance.Id
	distributionGroup []instance.Id
	result            []instance.Id
	err               error
}

func (p *mockInstanceDistributor) DistributeInstances(
	ctx envcontext.ProviderCallContext, candidates, distributionGroup []instance.Id, limitZones []string,
) ([]instance.Id, error) {
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
	s.policy.GetInstanceDistributor = func() (envcontext.Distributor, error) {
		return &s.distributor, nil
	}

	a := arch.DefaultArchitecture
	s.hwChar = &instance.HardwareCharacteristics{
		Arch: &a,
	}

	s.wordpress = s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	s.machines = make([]*state.Machine, 3)
	for i := range s.machines {
		m, err := s.State.AddOneMachine(state.MachineTemplate{
			Base:        state.UbuntuBase("12.10"),
			Jobs:        []state.MachineJob{state.JobHostUnits},
			Constraints: constraints.MustParse("arch=amd64"),
		})
		c.Assert(err, jc.ErrorIsNil)

		hwChar := *s.hwChar
		if i <= 1 {
			az := "az1"
			hwChar.AvailabilityZone = &az
		}

		instId := instance.Id(fmt.Sprintf("i-blah-%d", i))
		err = m.SetProvisioned(instId, "", "fake-nonce", &hwChar)
		c.Assert(err, jc.ErrorIsNil)

		s.machines[i] = m
	}
}

func (s *InstanceDistributorSuite) setupScenario(c *gc.C) {
	// Assign a unit so we have a non-empty distribution group, and
	// provision all instances so we have candidates.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machines[0])
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstances(c *gc.C) {
	s.setupScenario(c)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
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
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.distributor.result = []instance.Id{"notthere"}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/1" to clean machine: invalid instance returned: notthere`)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesNoEmptyMachines(c *gc.C) {
	for range s.machines {
		// Assign a unit so we have a non-empty distribution group.
		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		_, err = unit.AssignToCleanMachine()
		c.Assert(err, jc.ErrorIsNil)
	}

	// InstanceDistributor is not called if there are no empty instances.
	s.distributor.err = fmt.Errorf("no assignment for you")
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, eligibleMachinesInUse)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesErrors(c *gc.C) {
	s.setupScenario(c)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that assignment fails when DistributeInstances returns an error.
	s.distributor.err = fmt.Errorf("no assignment for you")
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, ".*no assignment for you")
	_, err = unit.AssignToCleanEmptyMachine()
	c.Assert(err, gc.ErrorMatches, ".*no assignment for you")
	// If the policy's InstanceDistributor method fails, that will be returned first.
	s.policy.GetInstanceDistributor = func() (envcontext.Distributor, error) {
		return nil, fmt.Errorf("incapable of InstanceDistributor")
	}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, ".*incapable of InstanceDistributor")
}

func (s *InstanceDistributorSuite) TestDistributeInstancesDistributionGroup(c *gc.C) {
	unit0, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit0.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)

	// Distribution group is not empty, because the machine assigned.
	unit1, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit1.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesEmptyDistributionGroup(c *gc.C) {
	s.distributor.err = fmt.Errorf("no assignment for you")

	// InstanceDistributor is not called if the distribution group is empty.
	unit0, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit0.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesEmptyDistributionGroupAfterAssignWithNonProvision(c *gc.C) {
	s.distributor.err = fmt.Errorf("no assignment for you")

	// InstanceDistributor is not called if the distribution group is empty.
	m, err := s.State.AddOneMachine(state.MachineTemplate{
		Base:                    state.UbuntuBase("12.10"),
		Jobs:                    []state.MachineJob{state.JobHostUnits},
		Constraints:             constraints.MustParse("arch=amd64"),
		HardwareCharacteristics: *s.hwChar,
	})
	c.Assert(err, jc.ErrorIsNil)

	unit0, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit0.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)

	// Distribution group is still empty, because the machine assigned to has
	// not been provisioned.
	unit1, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit1.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestInstanceDistributorUnimplemented(c *gc.C) {
	s.setupScenario(c)

	var distributorErr error
	s.policy.GetInstanceDistributor = func() (envcontext.Distributor, error) {
		return nil, distributorErr
	}
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, gc.ErrorMatches, `cannot assign unit "wordpress/1" to clean machine: policy returned nil instance distributor without an error`)

	distributorErr = errors.NotImplementedf("InstanceDistributor")
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesNoPolicy(c *gc.C) {
	s.policy.GetInstanceDistributor = func() (envcontext.Distributor, error) {
		c.Errorf("should not have been invoked")
		return nil, nil
	}
	state.SetPolicy(s.State, nil)
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InstanceDistributorSuite) TestDistributeInstancesWithZoneConstraints(c *gc.C) {
	err := s.wordpress.SetConstraints(constraints.MustParse("zones=az1"))
	c.Assert(err, jc.ErrorIsNil)

	// Initial unit, assigned to machine 0, to get a distribution group.
	unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.machines[0])
	c.Assert(err, jc.ErrorIsNil)

	unit, err = s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Only machine 1 is empty, and in the desired AZ.
	s.distributor.result = []instance.Id{"i-blah-1"}
	_, err = unit.AssignToCleanMachine()
	c.Assert(err, jc.ErrorIsNil)

	// Machine 2 filtered by zone constraint.
	c.Check(s.distributor.candidates, jc.SameContents, []instance.Id{"i-blah-1"})
	c.Check(s.distributor.distributionGroup, jc.SameContents, []instance.Id{"i-blah-0"})
}

type ApplicationMachinesSuite struct {
	ConnSuite
	wordpress *state.Application
	mysql     *state.Application
	machines  []*state.Machine
}

var _ = gc.Suite(&ApplicationMachinesSuite{})

func (s *ApplicationMachinesSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.wordpress = s.AddTestingApplication(
		c,
		"wordpress",
		s.AddTestingCharm(c, "wordpress"),
	)
	s.mysql = s.AddTestingApplication(
		c,
		"mysql",
		s.AddTestingCharm(c, "mysql"),
	)

	s.machines = make([]*state.Machine, 5)
	for i := range s.machines {
		var err error
		s.machines[i], err = s.State.AddOneMachine(state.MachineTemplate{
			Base: state.UbuntuBase("12.10"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	for _, i := range []int{0, 1, 4} {
		unit, err := s.wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Assert(err, jc.ErrorIsNil)
	}
	for _, i := range []int{2, 3} {
		unit, err := s.mysql.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = unit.AssignToMachine(s.machines[i])
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *ApplicationMachinesSuite) TestApplicationMachines(c *gc.C) {
	machines, err := state.ApplicationMachines(s.State, "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.DeepEquals, []string{"2", "3"})

	machines, err = state.ApplicationMachines(s.State, "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.DeepEquals, []string{"0", "1", "4"})

	machines, err = state.ApplicationMachines(s.State, "fred")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(machines), gc.Equals, 0)
}
