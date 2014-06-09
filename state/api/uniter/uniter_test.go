// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/uniter"
	coretesting "github.com/juju/juju/testing"
)

// NOTE: This suite is intended for embedding into other suites,
// so common code can be reused. Do not add test cases to it,
// otherwise they'll be run by each other suite that embeds it.
type uniterSuite struct {
	testing.JujuConnSuite

	st                 *api.State
	stateServerMachine *state.Machine
	wordpressMachine   *state.Machine
	wordpressService   *state.Service
	wordpressCharm     *state.Charm
	wordpressUnit      *state.Unit

	uniter *uniter.State
}

var _ = gc.Suite(&uniterSuite{})

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

func (s *uniterSuite) SetUpTest(c *gc.C) {
	s.setUpTest(c, true)
}

func (s *uniterSuite) setUpTest(c *gc.C, addStateServer bool) {
	s.JujuConnSuite.SetUpTest(c)

	if addStateServer {
		s.stateServerMachine = testing.AddStateServerMachine(c, s.State)
	}

	// Create a machine, a service and add a unit so we can log in as
	// its agent.
	s.wordpressMachine, s.wordpressService, s.wordpressCharm, s.wordpressUnit = s.addMachineServiceCharmAndUnit(c, "wordpress")
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = s.wordpressUnit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.st = s.OpenAPIAs(c, s.wordpressUnit.Tag(), password)

	// Create the uniter API facade.
	s.uniter = s.st.Uniter()
	c.Assert(s.uniter, gc.NotNil)
}

func (s *uniterSuite) addMachineServiceCharmAndUnit(c *gc.C, serviceName string) (*state.Machine, *state.Service, *state.Charm, *state.Unit) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	charm := s.AddTestingCharm(c, serviceName)
	service := s.AddTestingService(c, serviceName, charm)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	return machine, service, charm, unit
}

func (s *uniterSuite) addRelation(c *gc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints([]string{first, second})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)
	return rel
}

func (s *uniterSuite) addRelatedService(c *gc.C, firstSvc, relatedSvc string, unit *state.Unit) (*state.Relation, *state.Service, *state.Unit) {
	relatedService := s.AddTestingService(c, relatedSvc, s.AddTestingCharm(c, relatedSvc))
	rel := s.addRelation(c, firstSvc, relatedSvc)
	relUnit, err := rel.Unit(unit)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	relatedUnit, err := relatedService.Unit(relatedSvc + "/0")
	c.Assert(err, gc.IsNil)
	return rel, relatedService, relatedUnit
}

func (s *uniterSuite) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, gc.Equals, inScope)
}
