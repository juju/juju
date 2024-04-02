// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type LifeSuite struct {
	ConnSuite
	charm *state.Charm
	app   *state.Application
}

func (s *LifeSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	s.app = s.AddTestingApplication(c, "dummyapp", s.charm)
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
	isDying(s *LifeSuite, c *gc.C) bool
}

type unitLife struct {
	unit *state.Unit
	st   *state.State
}

func (l *unitLife) id() (coll string, id interface{}) {
	return state.UnitsC, state.DocID(l.st, l.unit.Name())
}

func (l *unitLife) setup(s *LifeSuite, c *gc.C) state.AgentLiving {
	unit, err := s.app.AddUnit(state.AddUnitParams{}, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	preventUnitDestroyRemove(c, unit)
	l.unit = unit
	return l.unit
}

func (l *unitLife) isDying(s *LifeSuite, c *gc.C) bool {
	col, id := l.id()
	dying, err := state.IsDying(l.st, col, id)
	c.Assert(err, jc.ErrorIsNil)
	return dying
}

type machineLife struct {
	machine *state.Machine
	st      *state.State
}

func (l *machineLife) id() (coll string, id interface{}) {
	return state.MachinesC, state.DocID(l.st, l.machine.Id())
}

func (l *machineLife) setup(s *LifeSuite, c *gc.C) state.AgentLiving {
	var err error
	l.machine, err = s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), status.NoopStatusHistoryRecorder, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return l.machine
}

func (l *machineLife) isDying(s *LifeSuite, c *gc.C) bool {
	col, id := l.id()
	dying, err := state.IsDying(l.st, col, id)
	c.Assert(err, jc.ErrorIsNil)
	return dying
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
	store := state.NewObjectStore(c, s.State.ModelUUID())
	for i, lfix := range []lifeFixture{&unitLife{st: s.State}, &machineLife{st: s.State}} {
		c.Logf("fixture %d", i)
		for j, v := range stateChanges {
			c.Logf("sequence %d", j)
			living := lfix.setup(s, c)
			s.prepareFixture(living, lfix, v.cached, v.dbinitial, c)
			switch v.desired {
			case state.Dying:
				err := living.Destroy(store, status.NoopStatusHistoryRecorder)
				c.Assert(err, jc.ErrorIsNil)

				// If we're already in the dead state, we can't transition, so
				// don't test that permutation.
				if v.dbinitial != state.Dead {
					ok := lfix.isDying(s, c)
					c.Assert(ok, jc.IsTrue)
				}
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
			err = living.Remove(store)
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
	notAliveErr = ".*: .* is not found or not alive"
	deadErr     = ".*: not found or dead"
	noErr       = ""
)

type lifer interface {
	EnsureDead() error
	Destroy(objectstore.ObjectStore, status.StatusHistoryRecorder) error
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
func testWhenDying(c *gc.C, store objectstore.ObjectStore, obj lifer, dyingErr, deadErr string, checks ...func() error) {
	c.Logf("checking life of %v (%T)", obj, obj)
	err := obj.Destroy(store, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	runLifeChecks(c, obj, dyingErr, checks)
	err = obj.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	runLifeChecks(c, obj, deadErr, checks)
}
