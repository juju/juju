// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	"fmt"

	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	charm "gopkg.in/juju/charm.v2"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type factorySuite struct {
	testing.BaseSuite
	jtesting.MgoSuite
	State   *state.State
	Factory *factory.Factory
}

var _ = gc.Suite(&factorySuite{})

func (s *factorySuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *factorySuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *factorySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	policy := statetesting.MockPolicy{}

	info := &authentication.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{jtesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
	}
	opts := mongo.DialOpts{
		Timeout: testing.LongWait,
	}
	cfg := testing.EnvironConfig(c)
	st, err := state.Initialize(info, cfg, opts, &policy)
	c.Assert(err, gc.IsNil)
	s.State = st
}

func (s *factorySuite) TearDownTest(c *gc.C) {
	if s.State != nil {
		s.State.Close()
	}
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *factorySuite) TestMakeUserNil(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	user := s.Factory.MakeUser()
	c.Assert(user.IsDeactivated(), jc.IsFalse)

	saved, err := s.State.User(user.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Tag(), gc.Equals, user.Tag())
	c.Assert(saved.Name(), gc.Equals, user.Name())
	c.Assert(saved.DisplayName(), gc.Equals, user.DisplayName())
	c.Assert(saved.CreatedBy(), gc.Equals, user.CreatedBy())
	c.Assert(saved.DateCreated(), gc.Equals, user.DateCreated())
	c.Assert(saved.LastLogin(), gc.Equals, user.LastLogin())
	c.Assert(saved.IsDeactivated(), gc.Equals, user.IsDeactivated())
}

func (s *factorySuite) TestMakeUserParams(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	username := "bob"
	displayName := "Bob the Builder"
	creator := "eric"
	password := "sekrit"
	user := s.Factory.MakeUser(factory.UserParams{
		Name:        username,
		DisplayName: displayName,
		Creator:     creator,
		Password:    password,
	})
	c.Assert(user.IsDeactivated(), jc.IsFalse)
	c.Assert(user.Name(), gc.Equals, username)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.CreatedBy(), gc.Equals, creator)
	c.Assert(user.PasswordValid(password), jc.IsTrue)

	saved, err := s.State.User(user.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(saved.Tag(), gc.Equals, user.Tag())
	c.Assert(saved.Name(), gc.Equals, user.Name())
	c.Assert(saved.DisplayName(), gc.Equals, user.DisplayName())
	c.Assert(saved.CreatedBy(), gc.Equals, user.CreatedBy())
	c.Assert(saved.DateCreated(), gc.Equals, user.DateCreated())
	c.Assert(saved.LastLogin(), gc.Equals, user.LastLogin())
	c.Assert(saved.IsDeactivated(), gc.Equals, user.IsDeactivated())
}

func (s *factorySuite) TestMakeMachineNil(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	machine := s.Factory.MakeMachine()
	c.Assert(machine, gc.NotNil)

	saved, err := s.State.Machine(machine.Id())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.Series(), gc.Equals, machine.Series())
	c.Assert(saved.Id(), gc.Equals, machine.Id())
	c.Assert(saved.Series(), gc.Equals, machine.Series())
	c.Assert(saved.Tag(), gc.Equals, machine.Tag())
	c.Assert(saved.Life(), gc.Equals, machine.Life())
	c.Assert(saved.Jobs(), gc.DeepEquals, machine.Jobs())
	savedInstanceId, err := saved.InstanceId()
	c.Assert(err, gc.IsNil)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(savedInstanceId, gc.Equals, machineInstanceId)
	c.Assert(saved.Clean(), gc.Equals, machine.Clean())
}

func (s *factorySuite) TestMakeMachine(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	series := "quantal"
	jobs := []state.MachineJob{state.JobManageEnviron}
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	nonce := "some-nonce"
	id := instance.Id("some-id")

	machine := s.Factory.MakeMachine(factory.MachineParams{
		Series:     series,
		Jobs:       jobs,
		Password:   password,
		Nonce:      nonce,
		InstanceId: id,
	})
	c.Assert(machine, gc.NotNil)

	c.Assert(machine.Series(), gc.Equals, series)
	c.Assert(machine.Jobs(), gc.DeepEquals, jobs)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(machineInstanceId, gc.Equals, id)
	c.Assert(machine.CheckProvisioned(nonce), gc.Equals, true)
	c.Assert(machine.PasswordValid(password), gc.Equals, true)

	saved, err := s.State.Machine(machine.Id())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.Id(), gc.Equals, machine.Id())
	c.Assert(saved.Series(), gc.Equals, machine.Series())
	c.Assert(saved.Tag(), gc.Equals, machine.Tag())
	c.Assert(saved.Life(), gc.Equals, machine.Life())
	c.Assert(saved.Jobs(), gc.DeepEquals, machine.Jobs())
	savedInstanceId, err := saved.InstanceId()
	c.Assert(err, gc.IsNil)
	c.Assert(savedInstanceId, gc.Equals, machineInstanceId)
	c.Assert(saved.Clean(), gc.Equals, machine.Clean())
}

func (s *factorySuite) TestMakeCharmNil(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	charm := s.Factory.MakeCharm()
	c.Assert(charm, gc.NotNil)

	saved, err := s.State.Charm(charm.URL())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.URL(), gc.DeepEquals, charm.URL())
	c.Assert(saved.Meta(), gc.DeepEquals, charm.Meta())
	c.Assert(saved.BundleURL(), gc.DeepEquals, charm.BundleURL())
	c.Assert(saved.BundleSha256(), gc.Equals, charm.BundleSha256())
}

func (s *factorySuite) TestMakeCharm(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	series := "quantal"
	name := "wordpress"
	revision := 13
	url := fmt.Sprintf("cs:%s/%s-%d", series, name, revision)
	ch := s.Factory.MakeCharm(factory.CharmParams{
		Name: name,
		URL:  url,
	})
	c.Assert(ch, gc.NotNil)

	c.Assert(ch.URL(), gc.DeepEquals, charm.MustParseURL(url))

	saved, err := s.State.Charm(ch.URL())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.URL(), gc.DeepEquals, ch.URL())
	c.Assert(saved.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(saved.Meta().Name, gc.Equals, name)
	c.Assert(saved.BundleURL(), gc.DeepEquals, ch.BundleURL())
	c.Assert(saved.BundleSha256(), gc.Equals, ch.BundleSha256())
}

func (s *factorySuite) TestMakeServiceNil(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	service := s.Factory.MakeService()
	c.Assert(service, gc.NotNil)

	saved, err := s.State.Service(service.Name())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.Name(), gc.Equals, service.Name())
	c.Assert(saved.Tag(), gc.Equals, service.Tag())
	c.Assert(saved.Life(), gc.Equals, service.Life())
}

func (s *factorySuite) TestMakeService(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	charm := s.Factory.MakeCharm(factory.CharmParams{Name: "wordpress"})
	creator := s.Factory.MakeUser(factory.UserParams{Name: "bill"}).Tag().String()
	service := s.Factory.MakeService(factory.ServiceParams{
		Charm:   charm,
		Creator: creator,
	})
	c.Assert(service, gc.NotNil)

	c.Assert(service.Name(), gc.Equals, "wordpress")
	c.Assert(service.GetOwnerTag(), gc.Equals, creator)
	curl, _ := service.CharmURL()
	c.Assert(curl, gc.DeepEquals, charm.URL())

	saved, err := s.State.Service(service.Name())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.Name(), gc.Equals, service.Name())
	c.Assert(saved.Tag(), gc.Equals, service.Tag())
	c.Assert(saved.Life(), gc.Equals, service.Life())
}

func (s *factorySuite) TestMakeUnitNil(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	unit := s.Factory.MakeUnit()
	c.Assert(unit, gc.NotNil)

	saved, err := s.State.Unit(unit.Name())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.Name(), gc.Equals, unit.Name())
	c.Assert(saved.ServiceName(), gc.Equals, unit.ServiceName())
	c.Assert(saved.Series(), gc.Equals, unit.Series())
	c.Assert(saved.Life(), gc.Equals, unit.Life())
}

func (s *factorySuite) TestMakeUnit(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	service := s.Factory.MakeService()
	unit := s.Factory.MakeUnit(factory.UnitParams{
		Service: service,
	})
	c.Assert(unit, gc.NotNil)

	c.Assert(unit.ServiceName(), gc.Equals, service.Name())

	saved, err := s.State.Unit(unit.Name())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.Name(), gc.Equals, unit.Name())
	c.Assert(saved.ServiceName(), gc.Equals, unit.ServiceName())
	c.Assert(saved.Series(), gc.Equals, unit.Series())
	c.Assert(saved.Life(), gc.Equals, unit.Life())
}

func (s *factorySuite) TestMakeRelationNil(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	relation := s.Factory.MakeRelation()
	c.Assert(relation, gc.NotNil)

	saved, err := s.State.Relation(relation.Id())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.Id(), gc.Equals, relation.Id())
	c.Assert(saved.Tag(), gc.Equals, relation.Tag())
	c.Assert(saved.Life(), gc.Equals, relation.Life())
	c.Assert(saved.Endpoints(), gc.DeepEquals, relation.Endpoints())
}

func (s *factorySuite) TestMakeRelation(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	s1 := s.Factory.MakeService(factory.ServiceParams{
		Name: "service1",
		Charm: s.Factory.MakeCharm(factory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := s1.Endpoint("db")
	c.Assert(err, gc.IsNil)

	s2 := s.Factory.MakeService(factory.ServiceParams{
		Name: "service2",
		Charm: s.Factory.MakeCharm(factory.CharmParams{
			Name: "mysql",
		}),
	})
	e2, err := s2.Endpoint("server")
	c.Assert(err, gc.IsNil)

	relation := s.Factory.MakeRelation(factory.RelationParams{
		Endpoints: []state.Endpoint{e1, e2},
	})
	c.Assert(relation, gc.NotNil)

	saved, err := s.State.Relation(relation.Id())
	c.Assert(err, gc.IsNil)

	c.Assert(saved.Id(), gc.Equals, relation.Id())
	c.Assert(saved.Tag(), gc.Equals, relation.Tag())
	c.Assert(saved.Life(), gc.Equals, relation.Life())
	c.Assert(saved.Endpoints(), gc.DeepEquals, relation.Endpoints())
}

func (s *factorySuite) TestMultileParamPanics(c *gc.C) {
	s.Factory = factory.NewFactory(s.State, c)

	c.Assert(func() { s.Factory.MakeUser(factory.UserParams{}, factory.UserParams{}) },
		gc.PanicMatches, "expecting 1 parameter or none")
	c.Assert(func() { s.Factory.MakeMachine(factory.MachineParams{}, factory.MachineParams{}) },
		gc.PanicMatches, "expecting 1 parameter or none")
	c.Assert(func() { s.Factory.MakeService(factory.ServiceParams{}, factory.ServiceParams{}) },
		gc.PanicMatches, "expecting 1 parameter or none")
	c.Assert(func() { s.Factory.MakeCharm(factory.CharmParams{}, factory.CharmParams{}) },
		gc.PanicMatches, "expecting 1 parameter or none")
	c.Assert(func() { s.Factory.MakeUnit(factory.UnitParams{}, factory.UnitParams{}) },
		gc.PanicMatches, "expecting 1 parameter or none")
	c.Assert(func() { s.Factory.MakeRelation(factory.RelationParams{}, factory.RelationParams{}) },
		gc.PanicMatches, "expecting 1 parameter or none")
}
