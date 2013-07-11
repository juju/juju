// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
)

type LifeSuite struct {
	ConnSuite
	charm *state.Charm
	svc   *state.Service
}

func (s *LifeSuite) SetUpTest(c *C) {
	var err error
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	s.svc, err = s.State.AddService("dummysvc", s.charm)
	c.Assert(err, IsNil)
}

var _ = Suite(&LifeSuite{})

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
	setup(s *LifeSuite, c *C) state.Living
	teardown(s *LifeSuite, c *C)
}

type unitLife struct {
	unit *state.Unit
}

func (l *unitLife) id() (coll string, id interface{}) {
	return "units", l.unit.Name()
}

func (l *unitLife) setup(s *LifeSuite, c *C) state.Living {
	unit, err := s.svc.AddUnit()
	c.Assert(err, IsNil)
	preventUnitDestroyRemove(c, unit)
	l.unit = unit
	return l.unit
}

func (l *unitLife) teardown(s *LifeSuite, c *C) {
	err := l.unit.Remove()
	c.Assert(err, IsNil)
}

type machineLife struct {
	machine *state.Machine
}

func (l *machineLife) id() (coll string, id interface{}) {
	return "machines", l.machine.Id()
}

func (l *machineLife) setup(s *LifeSuite, c *C) state.Living {
	var err error
	l.machine, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)
	return l.machine
}

func (l *machineLife) teardown(s *LifeSuite, c *C) {
	err := l.machine.Remove()
	c.Assert(err, IsNil)
}

func (s *LifeSuite) prepareFixture(living state.Living, lfix lifeFixture, cached, dbinitial state.Life, c *C) {
	collName, id := lfix.id()
	coll := s.MgoSuite.Session.DB("juju").C(collName)

	err := coll.UpdateId(id, D{{"$set", D{
		{"life", cached},
	}}})
	c.Assert(err, IsNil)
	err = living.Refresh()
	c.Assert(err, IsNil)

	err = coll.UpdateId(id, D{{"$set", D{
		{"life", dbinitial},
	}}})
	c.Assert(err, IsNil)
}

func (s *LifeSuite) TestLifecycleStateChanges(c *C) {
	for i, lfix := range []lifeFixture{&unitLife{}, &machineLife{}} {
		c.Logf("fixture %d", i)
		for j, v := range stateChanges {
			c.Logf("sequence %d", j)
			living := lfix.setup(s, c)
			s.prepareFixture(living, lfix, v.cached, v.dbinitial, c)
			switch v.desired {
			case state.Dying:
				err := living.Destroy()
				c.Assert(err, IsNil)
			case state.Dead:
				err := living.EnsureDead()
				c.Assert(err, IsNil)
			default:
				panic("desired lifecycle can only be dying or dead")
			}
			err := living.Refresh()
			c.Assert(err, IsNil)
			c.Assert(living.Life(), Equals, v.dbfinal)
			err = living.EnsureDead()
			c.Assert(err, IsNil)
			lfix.teardown(s, c)
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

func runLifeChecks(c *C, obj lifer, expectErr string, checks []func() error) {
	for i, check := range checks {
		c.Logf("check %d when %v", i, obj.Life())
		err := check()
		if expectErr == noErr {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, expectErr)
		}
	}
}

// testWhenDying sets obj to Dying and Dead in turn, and asserts
// that the errors from the given checks match aliveErr, dyingErr and deadErr
// in each respective life state.
func testWhenDying(c *C, obj lifer, dyingErr, deadErr string, checks ...func() error) {
	c.Logf("checking life of %v (%T)", obj, obj)
	err := obj.Destroy()
	c.Assert(err, IsNil)
	runLifeChecks(c, obj, dyingErr, checks)
	err = obj.EnsureDead()
	c.Assert(err, IsNil)
	runLifeChecks(c, obj, deadErr, checks)
}
