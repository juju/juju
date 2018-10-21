// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type StatusSuite struct {
	jujutesting.JujuConnSuite
}

func (s *StatusSuite) setupMultipleRelationsBetweenApplications(c *gc.C) {
	// make an application with 2 endpoints
	application1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	endpoint1, err := application1.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	endpoint2, err := application1.Endpoint("logging-dir")
	c.Assert(err, jc.ErrorIsNil)

	// make another application with 2 endpoints
	application2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "logging",
		}),
	})
	endpoint3, err := application2.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	endpoint4, err := application2.Endpoint("logging-directory")
	c.Assert(err, jc.ErrorIsNil)

	// create relation between a1:e1 and a2:e3
	relation1 := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint1, endpoint3},
	})
	c.Assert(relation1, gc.NotNil)

	// create relation between a1:e2 and a2:e4
	relation2 := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint2, endpoint4},
	})
	c.Assert(relation2, gc.NotNil)
}

func (s *StatusSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := cmdtesting.Context(c)
	command := commands.NewJujuCommand(context)
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	c.Assert(command.Run(context), jc.ErrorIsNil)
	loggo.RemoveWriter("warning")
	return context
}

func (s *StatusSuite) TestMultipleRelationsInYamlFormat(c *gc.C) {
	s.setupMultipleRelationsBetweenApplications(c)
	context := s.run(c, "status", "--format=yaml")
	out := cmdtesting.Stdout(context)

	// expected relations for 'logging'
	c.Assert(out, jc.Contains, `
    relations:
      info:
      - wordpress
      logging-directory:
      - wordpress
    subordinate-to:
    - wordpress
`)
	// expected relations for 'wordpress'
	c.Assert(out, jc.Contains, `
    relations:
      juju-info:
      - logging
      logging-dir:
      - logging
`)
}

func (s *StatusSuite) TestMultipleRelationsInTabularFormat(c *gc.C) {
	s.setupMultipleRelationsBetweenApplications(c)
	context := s.run(c, "status", "--relations")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, `
Relation provider      Requirer                   Interface  Type         Message
wordpress:juju-info    logging:info               juju-info  subordinate  joining  
wordpress:logging-dir  logging:logging-directory  logging    subordinate  joining  
`[1:])
}

func (s *StatusSuite) setupSeveralUnitsOnAMachine(c *gc.C) {
	applicationA := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name:     "mysql",
			Revision: "1",
		}),
	})
	applicationB := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name:     "wordpress",
			Revision: "3",
		}),
	})

	// Put a unit from each, application A and B, on the same machine.
	machine1 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: instance.Id("id0"),
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: applicationA,
		Machine:     machine1,
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: applicationB,
		Machine:     machine1,
	})
}

func (s *StatusSuite) TestStatusWhenFilteringByMachine(c *gc.C) {
	s.setupSeveralUnitsOnAMachine(c)

	// Put a unit from an application on a new machine.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: instance.Id("id1"),
	})
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "another",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name:     "mysql",
			Revision: "5",
		}),
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		Machine:     machine,
	})

	context := s.run(c, "status")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, `
App        Version  Status   Scale  Charm      Store       Rev  OS      Notes
another             waiting    0/1  mysql      jujucharms    5  ubuntu  
mysql               waiting    0/1  mysql      jujucharms    1  ubuntu  
wordpress           waiting    0/1  wordpress  jujucharms    3  ubuntu  

Unit         Workload  Agent       Machine  Public address  Ports  Message
another/0    waiting   allocating  1                               waiting for machine
mysql/0      waiting   allocating  0                               waiting for machine
wordpress/0  waiting   allocating  0                               waiting for machine

Machine  State    DNS  Inst id  Series   AZ  Message
0        pending       id0      quantal      
1        pending       id1      quantal      
`)

	context = s.run(c, "status", "0")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, `
App        Version  Status   Scale  Charm      Store       Rev  OS      Notes
mysql               waiting    0/1  mysql      jujucharms    1  ubuntu  
wordpress           waiting    0/1  wordpress  jujucharms    3  ubuntu  

Unit         Workload  Agent       Machine  Public address  Ports  Message
mysql/0      waiting   allocating  0                               waiting for machine
wordpress/0  waiting   allocating  0                               waiting for machine

Machine  State    DNS  Inst id  Series   AZ  Message
0        pending       id0      quantal      
`)
}

func (s *StatusSuite) TestStatusFilteringByMachineIDMatchesExactly(c *gc.C) {
	s.setupSeveralUnitsOnAMachine(c)

	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "another",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "mysql"}),
	})

	// Put a unit from an application on the 1st machine.
	machine1 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: instance.Id("id1"),
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		Machine:     machine1,
	})

	// Burn machine numbers until we reach 10.

	// Since machine 0 and 1 were created beforehand, we just need to
	// create 7 machines here.
	for i := 0; i < 8; i++ {
		s.Factory.MakeMachine(c, &factory.MachineParams{
			Jobs: []state.MachineJob{state.JobHostUnits},
		})
	}

	// Put a unit from an application on the 10th machine.
	machine10 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: instance.Id("id10"),
	})

	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		Machine:     machine10,
	})

	context := s.run(c, "status", "1")
	// Should not have matched anything from machine 10.
	c.Assert(cmdtesting.Stdout(context), jc.Contains, `
Unit       Workload  Agent       Machine  Public address  Ports  Message
another/0  waiting   allocating  1                               waiting for machine

Machine  State    DNS  Inst id  Series   AZ  Message
1        pending       id1      quantal      

`)

	context = s.run(c, "status", "10")
	// Should not have matched anything from machine 1.
	c.Assert(cmdtesting.Stdout(context), jc.Contains, `
Unit       Workload  Agent       Machine  Public address  Ports  Message
another/1  waiting   allocating  10                              waiting for machine

Machine  State    DNS  Inst id  Series   AZ  Message
10       pending       id10     quantal      

`)
}

// TestStatusMachineFilteringWithUnassignedUnits ensures that machine filtering
// functions even if there are unassigned units. Reproduces scenario
// described in lp#1684718.
func (s *StatusSuite) TestStatusMachineFilteringWithUnassignedUnits(c *gc.C) {
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "another",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "mysql"}),
	})
	u := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
	})
	err := u.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)

	s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs:       []state.MachineJob{state.JobHostUnits},
		InstanceId: instance.Id("id1"),
	})

	context := s.run(c, "status", "1")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, `
Machine  State    DNS  Inst id  Series   AZ  Message
1        pending       id1      quantal      

`)
	c.Assert(cmdtesting.Stderr(context), gc.Equals, ``)
}
