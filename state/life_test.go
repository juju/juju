// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
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
	st   *state.State
}

func (l *unitLife) id() (coll string, id interface{}) {
	return "units", state.DocID(l.st, l.unit.Name())
}

func (l *unitLife) setup(s *LifeSuite, c *gc.C) state.AgentLiving {
	unit, err := s.svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)
	l.unit = unit
	return l.unit
}

type machineLife struct {
	machine *state.Machine
	st      *state.State
}

func (l *machineLife) id() (coll string, id interface{}) {
	return "machines", state.DocID(l.st, l.machine.Id())
}

func (l *machineLife) setup(s *LifeSuite, c *gc.C) state.AgentLiving {
	var err error
	l.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return l.machine
}

func (s *LifeSuite) prepareFixture(living state.Living, lfix lifeFixture, cached, dbinitial state.Life, c *gc.C) {
	collName, id := lfix.id()
	coll := s.MgoSuite.Session.DB("juju").C(collName)

	err := coll.UpdateId(id, bson.D{{"$set", bson.D{
		{"life", cached},
	}}})
	c.Assert(err, jc.ErrorIsNil)
	err = living.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	err = coll.UpdateId(id, bson.D{{"$set", bson.D{
		{"life", dbinitial},
	}}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LifeSuite) TestLifecycleStateChanges(c *gc.C) {
	for i, lfix := range []lifeFixture{&unitLife{st: s.State}, &machineLife{st: s.State}} {
		c.Logf("fixture %d", i)
		for j, v := range stateChanges {
			c.Logf("sequence %d", j)
			living := lfix.setup(s, c)
			s.prepareFixture(living, lfix, v.cached, v.dbinitial, c)
			switch v.desired {
			case state.Dying:
				err := living.Destroy()
				c.Assert(err, jc.ErrorIsNil)
			case state.Dead:
				err := living.EnsureDead()
				c.Assert(err, jc.ErrorIsNil)
			default:
				panic("desired lifecycle can only be dying or dead")
			}
			err := living.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(living.Life(), gc.Equals, v.dbfinal)
			err = living.EnsureDead()
			c.Assert(err, jc.ErrorIsNil)
			err = living.Remove()
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}

func (s *LifeSuite) TestLifeString(c *gc.C) {
	var tests = []struct {
		life state.Life
		want string
	}{
		{state.Alive, "alive"},
		{state.Dying, "dying"},
		{state.Dead, "dead"},
		{42, "unknown"},
	}
	for _, test := range tests {
		got := test.life.String()
		c.Assert(got, gc.Equals, test.want)
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
			c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	runLifeChecks(c, obj, dyingErr, checks)
	err = obj.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	runLifeChecks(c, obj, deadErr, checks)
}
