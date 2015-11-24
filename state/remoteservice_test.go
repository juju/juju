// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
)

type remoteServiceSuite struct {
	ConnSuite
	service *state.RemoteService
}

var _ = gc.Suite(&remoteServiceSuite{})

func (s *remoteServiceSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	eps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
		{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
		{
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	var err error
	s.service, err = s.State.AddRemoteService("mysql", "local:/u/me/mysql", eps)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *remoteServiceSuite) assertServiceRelations(c *gc.C, svc *state.Service, expectedKeys ...string) []*state.Relation {
	rels, err := svc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	if len(rels) == 0 {
		return nil
	}
	relKeys := make([]string, len(expectedKeys))
	for i, rel := range rels {
		relKeys[i] = rel.String()
	}
	sort.Strings(relKeys)
	c.Assert(relKeys, gc.DeepEquals, expectedKeys)
	return rels
}

func (s *remoteServiceSuite) TestTag(c *gc.C) {
	c.Assert(s.service.Tag().String(), gc.Equals, "service-mysql")
}

func (s *remoteServiceSuite) TestURL(c *gc.C) {
	c.Assert(s.service.URL(), gc.Equals, "local:/u/me/mysql")
}

func (s *remoteServiceSuite) TestMysqlEndpoints(c *gc.C) {
	_, err := s.service.Endpoint("foo")
	c.Assert(err, gc.ErrorMatches, `remote service "mysql" has no "foo" relation`)

	serverEP, err := s.service.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	adminEp := state.Endpoint{
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	loggingEp := state.Endpoint{
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	eps, err := s.service.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{serverEP, adminEp, loggingEp})
}

func (s *remoteServiceSuite) TestServiceRefresh(c *gc.C) {
	s1, err := s.State.RemoteService(s.service.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = s1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteServiceSuite) TestAddRelationBothRemote(c *gc.C) {
	wpep := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	}
	_, err := s.State.AddRemoteService("wordpress", "local:/u/me/wordpress", wpep)
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:db": cannot add relation between remote services "wordpress" and "mysql"`)
}

func (s *remoteServiceSuite) TestInferEndpointsWrongScope(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingService(c, "logging", subCharm)
	_, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *remoteServiceSuite) TestAddRemoteServiceErrors(c *gc.C) {
	_, err := s.State.AddRemoteService("haha/borken", "local:/u/me/mysql", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add remote service "haha/borken": invalid name`)
	_, err = s.State.RemoteService("haha/borken")
	c.Assert(err, gc.ErrorMatches, `remote service name "haha/borken" not valid`)

	_, err = s.State.AddRemoteService("borken", "haha/borken", nil)
	c.Assert(err, gc.ErrorMatches,
		`cannot add remote service "borken": validating service URL: `+
			`service URL has invalid form, missing "/u/<user>": "haha/borken"`,
	)
	_, err = s.State.RemoteService("borken")
	c.Assert(err, gc.ErrorMatches, `remote service "borken" not found`)
}

func (s *remoteServiceSuite) TestAddRemoteService(c *gc.C) {
	foo, err := s.State.AddRemoteService("foo", "local:/u/me/foo", nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Name(), gc.Equals, "foo")
	foo, err = s.State.RemoteService("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Name(), gc.Equals, "foo")
}

func (s *remoteServiceSuite) TestAddRemoteRelationWrongScope(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingService(c, "logging", subCharm)
	ep1 := state.Endpoint{
		ServiceName: "mysql",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	ep2 := state.Endpoint{
		ServiceName: "logging",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging-client",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeContainer,
		},
	}
	_, err := s.State.AddRelation(ep1, ep2)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:logging-client mysql:logging": both endpoints must be globally scoped for remote relations`)
}

func (s *remoteServiceSuite) TestAddRemoteRelationLocalFirst(c *gc.C) {
	s.assertAddRemoteRelation(c, "wordpress", "mysql")
}

func (s *remoteServiceSuite) TestAddRemoteRelationRemoteFirst(c *gc.C) {
	s.assertAddRemoteRelation(c, "mysql", "wordpress")
}

func (s *remoteServiceSuite) assertAddRemoteRelation(c *gc.C, service1, service2 string) {
	endpoints := map[string]state.Endpoint{
		"wordpress": state.Endpoint{
			ServiceName: "wordpress",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		},
		"mysql": state.Endpoint{
			ServiceName: "mysql",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints(service1, service2)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.String(), gc.Equals, "wordpress:db mysql:db")
	c.Assert(rel.Endpoints(), jc.DeepEquals, []state.Endpoint{endpoints[service1], endpoints[service2]})
	remoteSvc, err := s.State.RemoteService("mysql")
	c.Assert(err, jc.ErrorIsNil)
	relations, err := remoteSvc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relations, gc.HasLen, 1)
	c.Assert(relations[0], jc.DeepEquals, rel)
}

func (s *remoteServiceSuite) TestDestroySimple(c *gc.C) {
	err := s.service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.service.Life(), gc.Equals, state.Dying)
	err = s.service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteServiceSuite) TestDestroyWithRemovableRelation(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	// Destroy a local service with no units in relation scope; check service and
	// unit removed.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteServiceSuite) TestDestroyWithReferencedRelation(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *remoteServiceSuite) TestDestroyWithReferencedRelationStaleCount(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *remoteServiceSuite) assertDestroyWithReferencedRelation(c *gc.C, refresh bool) {
	ch := s.AddTestingCharm(c, "wordpress")
	wordpress := s.AddTestingService(c, "wordpress", ch)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel0, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingService(c, "another", ch)
	eps, err = s.State.InferEndpoints("another", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel0.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Optionally update the service document to get correct relation counts.
	if refresh {
		err = s.service.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	err = s.service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel0.Life(), gc.Equals, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Drop the last reference to the first relation; check the relation and
	// the service are are both removed.
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteServiceSuite) TestAllRemoteServicesNone(c *gc.C) {
	err := s.service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	services, err := s.State.AllRemoteServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 0)
}

func (s *remoteServiceSuite) TestAllRemoteServices(c *gc.C) {
	// There's initially the service created in test setup.
	services, err := s.State.AllRemoteServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(services), gc.Equals, 1)

	_, err = s.State.AddRemoteService("another", "local:/u/me/another", nil)
	c.Assert(err, jc.ErrorIsNil)
	services, err = s.State.AllRemoteServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(services, gc.HasLen, 2)

	// Check the returned service, order is defined by sorted keys.
	names := make([]string, len(services))
	for i, svc := range services {
		names[i] = svc.Name()
	}
	sort.Strings(names)
	c.Assert(names[0], gc.Equals, "another")
	c.Assert(names[1], gc.Equals, "mysql")
}

func (s *remoteServiceSuite) TestAddServiceEnvironmentDying(c *gc.C) {
	// Check that services cannot be added if the environment is initially Dying.
	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	err = env.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteService("s1", "local:/u/me/s1", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add remote service "s1": environment is no longer alive`)
}

func (s *remoteServiceSuite) TestAddServiceSameLocalExists(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddService(state.AddServiceArgs{Name: "s1", Owner: s.Owner.String(), Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteService("s1", "local:/u/me/s1", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add remote service "s1": local service with same name already exists`)
}

func (s *remoteServiceSuite) TestAddServiceLocalAddedAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that a service with a name conflict cannot be added if
	// there is no conflict initially but a local service is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddService(state.AddServiceArgs{Name: "s1", Owner: s.Owner.String(), Charm: charm})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteService("s1", "local:/u/me/s1", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add remote service "s1": local service with same name already exists`)
}

func (s *remoteServiceSuite) TestAddServiceSameRemoteExists(c *gc.C) {
	_, err := s.State.AddRemoteService("s1", "local:/u/me/s1", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteService("s1", "local:/u/me/s1", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add remote service "s1": remote service already exists`)
}

func (s *remoteServiceSuite) TestAddServiceRemoteAddedAfterInitial(c *gc.C) {
	// Check that a service with a name conflict cannot be added if
	// there is no conflict initially but a remote service is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddRemoteService("s1", "local:/u/me/s1", nil)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteService("s1", "local:/u/me/s1", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add remote service "s1": remote service already exists`)
}

func (s *remoteServiceSuite) TestAddServiceEnvironDiesAfterInitial(c *gc.C) {
	// Check that a service with a name conflict cannot be added if
	// there is no conflict initially but a remote service is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		env, err := s.State.Environment()
		c.Assert(err, jc.ErrorIsNil)
		err = env.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteService("s1", "local:/u/me/s1", nil)
	c.Assert(err, gc.ErrorMatches, `cannot add remote service "s1": environment is no longer alive`)
}

func (s *remoteServiceSuite) TestWatchRemoteServices(c *gc.C) {
	w := s.State.WatchRemoteServices()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("mysql") // initial
	wc.AssertNoChange()

	db2, err := s.State.AddRemoteService("db2", "local:/u/ibm/db2", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("db2")
	wc.AssertNoChange()

	err = db2.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = db2.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	wc.AssertChangeInSingleEvent("db2")
	wc.AssertNoChange()
}

func (s *remoteServiceSuite) TestWatchRemoteServicesDying(c *gc.C) {
	w := s.State.WatchRemoteServices()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("mysql") // initial
	wc.AssertNoChange()

	ch := s.AddTestingCharm(c, "wordpress")
	wordpress := s.AddTestingService(c, "wordpress", ch)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	// Add a unit to the relation so the remote service is not
	// short-circuit removed.
	unit, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.service.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChangeInSingleEvent("mysql")
	wc.AssertNoChange()
}
