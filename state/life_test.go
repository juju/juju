// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

type LifeSuite struct {
	ConnSuite
	charm *state.Charm
	svc   *state.Service
}

func (s *LifeSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	s.svc = s.AddTestingService(c, "dummysvc", s.charm)
}

var _ = gc.Suite(&LifeSuite{})

var stateChanges = []struct {
	cached, desired    state.Life
	dbinitial, dbfinal state.Life
}{
	{
		state.Alive, state.Dying,
		state.Alive, state.Dying,
	},
	{
		state.Alive, state.Dying,
		state.Dying, state.Dying,
	},
	{
		state.Alive, state.Dying,
		state.Dead, state.Dead,
	},
	{
		state.Alive, state.Dead,
		state.Alive, state.Dead,
	},
	{
		state.Alive, state.Dead,
		state.Dying, state.Dead,
	},
	{
		state.Alive, state.Dead,
		state.Dead, state.Dead,
	},
	{
		state.Dying, state.Dying,
		state.Dying, state.Dying,
	},
	{
		state.Dying, state.Dying,
		state.Dead, state.Dead,
	},
	{
		state.Dying, state.Dead,
		state.Dying, state.Dead,
	},
	{
		state.Dying, state.Dead,
		state.Dead, state.Dead,
	},
	{
		state.Dead, state.Dying,
		state.Dead, state.Dead,
	},
	{
		state.Dead, state.Dead,
		state.Dead, state.Dead,
	},
}

type lifeFixture interface {
	id() (coll string, id interface{})
	setup(s *LifeSuite, c *gc.C) state.AgentLiving
}

type unitLife struct {
	unit *state.Unit
}

func (l *unitLife) id() (coll string, id interface{}) {
	return "units", l.unit.Name()
}

func (l *unitLife) setup(s *LifeSuite, c *gc.C) state.AgentLiving {
	unit, err := s.svc.AddUnit()
	c.Assert(err, gc.IsNil)
	preventUnitDestroyRemove(c, unit)
	l.unit = unit
	return l.unit
}

type machineLife struct {
	machine *state.Machine
}

func (l *machineLife) id() (coll string, id interface{}) {
	return "machines", l.machine.Id()
}

func (l *machineLife) setup(s *LifeSuite, c *gc.C) state.AgentLiving {
	var err error
	l.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	return l.machine
}

func (s *LifeSuite) prepareFixture(living state.Living, lfix lifeFixture, cached, dbinitial state.Life, c *gc.C) {
	collName, id := lfix.id()
	coll := s.MgoSuite.Session.DB("juju").C(collName)

	err := coll.UpdateId(id, D{{"$set", D{
		{"life", cached},
	}}})
	c.Assert(err, gc.IsNil)
	err = living.Refresh()
	c.Assert(err, gc.IsNil)

	err = coll.UpdateId(id, D{{"$set", D{
		{"life", dbinitial},
	}}})
	c.Assert(err, gc.IsNil)
}

func (s *LifeSuite) TestLifecycleStateChanges(c *gc.C) {
	for i, lfix := range []lifeFixture{&unitLife{}, &machineLife{}} {
		c.Logf("fixture %d", i)
		for j, v := range stateChanges {
			c.Logf("sequence %d", j)
			living := lfix.setup(s, c)
			s.prepareFixture(living, lfix, v.cached, v.dbinitial, c)
			switch v.desired {
			case state.Dying:
				err := living.Destroy()
				c.Assert(err, gc.IsNil)
			case state.Dead:
				err := living.EnsureDead()
				c.Assert(err, gc.IsNil)
			default:
				panic("desired lifecycle can only be dying or dead")
			}
			err := living.Refresh()
			c.Assert(err, gc.IsNil)
			c.Assert(living.Life(), gc.Equals, v.dbfinal)
			err = living.EnsureDead()
			c.Assert(err, gc.IsNil)
			err = living.Remove()
			c.Assert(err, gc.IsNil)
		}
	}
}

const (
	notAliveErr = ".*: not found or not alive"
	deadErr     = ".*: not found or dead"
	noErr       = ""
)

type lifer interface {
	EnsureDead() error
	Destroy() error
	Life() state.Life
}

func runLifeChecks(c *gc.C, obj lifer, expectErr string, checks []func() error) {
	for i, check := range checks {
		c.Logf("check %d when %v", i, obj.Life())
		err := check()
		if expectErr == noErr {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, expectErr)
		}
	}
}

// testWhenDying sets obj to Dying and Dead in turn, and asserts
// that the errors from the given checks match aliveErr, dyingErr and deadErr
// in each respective life state.
func testWhenDying(c *gc.C, obj lifer, dyingErr, deadErr string, checks ...func() error) {
	c.Logf("checking life of %v (%T)", obj, obj)
	err := obj.Destroy()
	c.Assert(err, gc.IsNil)
	runLifeChecks(c, obj, dyingErr, checks)
	err = obj.EnsureDead()
	c.Assert(err, gc.IsNil)
	runLifeChecks(c, obj, deadErr, checks)
}
