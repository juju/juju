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

type relationLife struct {
	rel *state.Relation
}

func (l *relationLife) id() (coll string, id interface{}) {
	return "relations", l.rel.String()
}

func (l *relationLife) setup(s *LifeSuite, c *C) state.Living {
	var err error
	ep := state.RelationEndpoint{s.svc.Name(), "ifce", "baz", state.RolePeer, charm.ScopeGlobal}
	l.rel, err = s.State.AddRelation(ep)
	c.Assert(err, IsNil)
	return l.rel
}

func (l *relationLife) teardown(s *LifeSuite, c *C) {
	err := s.State.RemoveRelation(l.rel)
	c.Assert(err, IsNil)
}

type unitLife struct {
	unit *state.Unit
}

func (l *unitLife) id() (coll string, id interface{}) {
	return "units", l.unit.Name()
}

func (l *unitLife) setup(s *LifeSuite, c *C) state.Living {
	var err error
	l.unit, err = s.svc.AddUnit()
	c.Assert(err, IsNil)
	return l.unit
}

func (l *unitLife) teardown(s *LifeSuite, c *C) {
	err := s.svc.RemoveUnit(l.unit)
	c.Assert(err, IsNil)
}

type serviceLife struct {
	service *state.Service
}

func (l *serviceLife) id() (coll string, id interface{}) {
	return "services", l.service.Name()
}

func (l *serviceLife) setup(s *LifeSuite, c *C) state.Living {
	l.service = s.svc
	return l.service
}

func (l *serviceLife) teardown(s *LifeSuite, c *C) {
}

type machineLife struct {
	machine *state.Machine
}

func (l *machineLife) id() (coll string, id interface{}) {
	return "machines", l.machine.Id()
}

func (l *machineLife) setup(s *LifeSuite, c *C) state.Living {
	var err error
	l.machine, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	return l.machine
}

func (l *machineLife) teardown(s *LifeSuite, c *C) {
	err := s.State.RemoveMachine(l.machine.Id())
	c.Assert(err, IsNil)
}

func (s *LifeSuite) prepareFixture(living state.Living, lfix lifeFixture, cached, dbinitial state.Life, c *C) {
	collName, id := lfix.id()
	coll := s.MgoSuite.Session.DB("juju").C(collName)

	err := coll.UpdateId(id, bson.D{{"$set", bson.D{
		{"life", cached},
	}}})
	c.Assert(err, IsNil)
	err = living.Refresh()
	c.Assert(err, IsNil)

	err = coll.UpdateId(id, bson.D{{"$set", bson.D{
		{"life", dbinitial},
	}}})
	c.Assert(err, IsNil)
}

func (s *LifeSuite) TestLifecycleStateChanges(c *C) {
	for _, lfix := range []lifeFixture{&relationLife{}, &unitLife{}, &serviceLife{}, &machineLife{}} {
		for _, v := range stateChanges {
			living := lfix.setup(s, c)
			s.prepareFixture(living, lfix, v.cached, v.dbinitial, c)
			switch v.desired {
			case state.Dying:
				err := living.Kill()
				c.Assert(err, IsNil)
			case state.Dead:
				err := living.Die()
				c.Assert(err, IsNil)
			default:
				panic("desired lifecycle can only be dying or dead")
			}
			err := living.Refresh()
			c.Assert(err, IsNil)
			c.Assert(living.Life(), Equals, v.dbfinal)
			err = living.Die()
			c.Assert(err, IsNil)
			lfix.teardown(s, c)
		}
	}
}
