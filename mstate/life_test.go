package mstate_test

import (
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	state "launchpad.net/juju-core/mstate"
)

type LifeSuite struct {
	ConnSuite
	charm *state.Charm
}

func (s *LifeSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
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

func (s *RelationSuite) createRelationWithLife(svc *state.Service, cached, dbinitial state.Life, c *C) *state.Relation {
	peerep := state.RelationEndpoint{svc.Name(), "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
	rel, err := s.State.AddRelation(peerep)
	c.Assert(err, IsNil)

	err = s.relations.UpdateId(rel.Id(), bson.D{{"$set", bson.D{
		{"life", cached},
	}}})
	c.Assert(err, IsNil)
	err = rel.Refresh()
	c.Assert(err, IsNil)

	err = s.relations.UpdateId(rel.Id(), bson.D{{"$set", bson.D{
		{"life", dbinitial},
	}}})
	c.Assert(err, IsNil)

	return rel
}

func (s *RelationSuite) createUnitWithLife(svc *state.Service, cached, dbinitial state.Life, c *C) *state.Unit {
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)

	err = s.units.UpdateId(unit.Name(), bson.D{{"$set", bson.D{
		{"life", cached},
	}}})
	c.Assert(err, IsNil)
	err = unit.Refresh()
	c.Assert(err, IsNil)

	err = s.units.UpdateId(unit.Name(), bson.D{{"$set", bson.D{
		{"life", dbinitial},
	}}})
	c.Assert(err, IsNil)

	return unit
}

type livingEnv struct {
	setup    func(cached, dbinitial state.Life, c *C) state.Living
	teardown func(l state.Living, c *C)
}

func (s *RelationSuite) testLifecycleStateChangesForLiving(env *livingEnv, c *C) {
	for _, v := range stateChanges {
		l := env.setup(v.cached, v.dbinitial, c)
		switch v.desired {
		case state.Dying:
			err := l.Kill()
			c.Assert(err, IsNil)
		case state.Dead:
			err := l.Die()
			c.Assert(err, IsNil)
		default:
			panic("desired lifecycle can only be dying or dead")
		}
		err := l.Refresh()
		c.Assert(err, IsNil)
		c.Assert(l.Life(), Equals, v.dbfinal)

		env.teardown(l, c)
	}
}

func (s *RelationSuite) TestLifecycleStateChanges(c *C) {
	peer, err := s.State.AddService("peer", s.charm)
	c.Assert(err, IsNil)

	envs := []*livingEnv{
		{
			// Relation.
			setup: func(cached, dbinitial state.Life, c *C) state.Living {
				return s.createRelationWithLife(peer, cached, dbinitial, c)
			},
			teardown: func(l state.Living, c *C) {
				r, ok := l.(*state.Relation)
				if !ok {
					c.Errorf("unexpected living type")
				}
				err := r.Die()
				c.Assert(err, IsNil)
				err = s.State.RemoveRelation(r)
				c.Assert(err, IsNil)

			},
		},
		{
			// Unit.
			setup: func(cached, dbinitial state.Life, c *C) state.Living {
				return s.createUnitWithLife(peer, cached, dbinitial, c)
			},
			teardown: func(l state.Living, c *C) {
				u, ok := l.(*state.Unit)
				if !ok {
					c.Errorf("unexpected living type")
				}
				err := u.Die()
				c.Assert(err, IsNil)
				err = peer.RemoveUnit(u)
				c.Assert(err, IsNil)

			},
		},
	}

	for _, env := range envs {
		s.testLifecycleStateChangesForLiving(env, c)
	}
}
