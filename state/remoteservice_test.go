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
)

type remoteServiceSuite struct {
	ConnSuite
	service *state.RemoteService
}

var _ = gc.Suite(&remoteServiceSuite{})

func (s *remoteServiceSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	eps := []state.Endpoint{
		{
			ServiceName: "mysql",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
		{
			ServiceName: "mysql",
			Relation: charm.Relation{
				Interface: "mysql-root",
				Name:      "db-admin",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	var err error
	s.service, err = s.State.AddRemoteService("mysql", eps)
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

func (s *remoteServiceSuite) TestMysqlEndpoints(c *gc.C) {
	_, err := s.service.Endpoint("foo")
	c.Assert(err, gc.ErrorMatches, `remote service "mysql" has no "foo" relation`)

	serverEP, err := s.service.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ServiceName: "mysql",
		IsRemote:    true,
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	adminEp := state.Endpoint{
		ServiceName: "mysql",
		IsRemote:    true,
		Relation: charm.Relation{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	eps, err := s.service.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{serverEP, adminEp})
}

func (s *remoteServiceSuite) TestServiceRefresh(c *gc.C) {
	s1, err := s.State.RemoteService(s.service.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = s1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteServiceSuite) TestAddRemoteRelationWrongEndpoints(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.RelatedEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteRelation(eps[1], eps[0])
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:db": expecting endpoint "db" to be for a local service`)

	s.AddTestingService(c, "localmysql", s.AddTestingCharm(c, "mysql"))
	localeps, err := s.State.InferEndpoints("wordpress", "localmysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteRelation(eps[0], localeps[1])
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db localmysql:server": expecting endpoint "server" to be for a remote service`)
}

func (s *remoteServiceSuite) TestAddRemoteRelation(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.RelatedEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRemoteRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.String(), gc.Equals, "wordpress:db mysql:db")
	c.Assert(rel.Endpoints(), jc.DeepEquals, []state.Endpoint{{
		ServiceName: "wordpress",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
			Limit:     1,
		},
	}, {
		ServiceName: "mysql",
		IsRemote:    true,
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}})
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
	eps, err := s.State.RelatedEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRemoteRelation(eps[0], eps[1])
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
	eps, err := s.State.RelatedEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel0, err := s.State.AddRemoteRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingService(c, "another", ch)
	eps, err = s.State.RelatedEndpoints("another", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.State.AddRemoteRelation(eps[0], eps[1])
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

	_, err = s.State.AddRemoteService("another", nil)
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

var relatedEndpointsTests = []struct {
	summary string
	inputs  [][]string
	eps     []state.Endpoint
	err     string
}{
	{
		summary: "invalid args",
		inputs: [][]string{
			{"ping:"},
			{":pong"},
			{":"},
		},
		err: `invalid endpoint ".*"`,
	}, {
		summary: "unknown service",
		inputs:  [][]string{{"wooble", "mysql"}},
		err:     `service "wooble" not found`,
	}, {
		summary: "invalid relations",
		inputs: [][]string{
			{"local", "mysql"},
		},
		err: `no relations found`,
	}, {
		summary: "unambiguous provider/requirer relation",
		inputs: [][]string{
			{"wordpress", "mysql"},
			{"wordpress:db", "mysql"},
		},
		eps: []state.Endpoint{{
			ServiceName: "wordpress",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		}, {
			ServiceName: "mysql",
			IsRemote:    true,
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		}},
	},
}

func (s *remoteServiceSuite) TestRelatedEndpoints(c *gc.C) {
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingService(c, "local", s.AddTestingCharm(c, "mysql-alternative"))

	for i, t := range relatedEndpointsTests {
		c.Logf("test %d: %s", i, t.summary)
		for j, input := range t.inputs {
			var local, remote string
			if len(input) > 0 {
				local = input[0]
			}
			if len(input) > 1 {
				remote = input[1]
			}
			c.Logf("  input %d: %v, %v", j, local, remote)
			eps, err := s.State.RelatedEndpoints(local, remote)
			if t.err == "" {
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(eps, gc.DeepEquals, t.eps)
			} else {
				c.Assert(err, gc.ErrorMatches, t.err)
			}
		}
	}
}
