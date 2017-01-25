// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
)

type remoteApplicationSuite struct {
	ConnSuite
	application *state.RemoteApplication
}

var _ = gc.Suite(&remoteApplicationSuite{})

func (s *remoteApplicationSuite) SetUpTest(c *gc.C) {
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
	s.application, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		URL:         "local:/u/me/mysql",
		SourceModel: s.State.ModelTag(),
		Token:       "t0",
		Endpoints:   eps,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *remoteApplicationSuite) assertApplicationRelations(c *gc.C, svc *state.Application, expectedKeys ...string) []*state.Relation {
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

func (s *remoteApplicationSuite) TestInitialStatus(c *gc.C) {
	appStatus, err := s.application.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Since, gc.NotNil)
	appStatus.Since = nil
	c.Assert(appStatus, gc.DeepEquals, status.StatusInfo{
		Status:  status.Unknown,
		Message: "waiting for remote connection",
		Data:    map[string]interface{}{},
	})
}

func (s *remoteApplicationSuite) TestStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "busy",
		Data:    map[string]interface{}{"foo": "bar"},
		Since:   &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.RemoteApplication("mysql")
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := app.Status()
	c.Assert(appStatus.Since, gc.NotNil)
	appStatus.Since = nil
	c.Assert(appStatus, gc.DeepEquals, status.StatusInfo{
		Status:  status.Maintenance,
		Message: "busy",
		Data:    map[string]interface{}{"foo": "bar"},
	})
}

func (s *remoteApplicationSuite) TestSetStatusSince(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := s.application.Status()
	c.Assert(err, jc.ErrorIsNil)
	firstTime := appStatus.Since
	c.Assert(firstTime, gc.NotNil)
	c.Assert(timeBeforeOrEqual(now, *firstTime), jc.IsTrue)

	// Setting the same status a second time also updates the timestamp.
	err = s.application.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err = s.application.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *appStatus.Since), jc.IsTrue)
}

func (s *remoteApplicationSuite) TestGetSetStatusNotFound(c *gc.C) {
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "not really",
		Since:   &now,
	}
	err = s.application.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set status: remote application not found`)

	statusInfo, err := s.application.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: remote application not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *remoteApplicationSuite) TestTag(c *gc.C) {
	c.Assert(s.application.Tag().String(), gc.Equals, "application-mysql")
}

func (s *remoteApplicationSuite) TestURL(c *gc.C) {
	url, ok := s.application.URL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(url, gc.Equals, "local:/u/me/mysql")

	// Add another remote application without a URL.
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql1",
		SourceModel: s.State.ModelTag(),
		Token:       "t0",
	})
	c.Assert(err, jc.ErrorIsNil)
	url, ok = app.URL()
	c.Assert(ok, jc.IsFalse)
	c.Assert(url, gc.Equals, "")
}

func (s *remoteApplicationSuite) TestMysqlEndpoints(c *gc.C) {
	_, err := s.application.Endpoint("foo")
	c.Assert(err, gc.ErrorMatches, `remote application "mysql" has no "foo" relation`)

	serverEP, err := s.application.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(serverEP, gc.DeepEquals, state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	})

	adminEp := state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "mysql-root",
			Name:      "db-admin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	loggingEp := state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	eps, err := s.application.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{serverEP, adminEp, loggingEp})
}

func (s *remoteApplicationSuite) TestApplicationRefresh(c *gc.C) {
	s1, err := s.State.RemoteApplication(s.application.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = s1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestAddRelationBothRemote(c *gc.C) {
	wpep := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	}
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "wordpress", URL: "local:/u/me/wordpress", Endpoints: wpep, SourceModel: s.State.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:db": cannot add relation between remote applications "wordpress" and "mysql"`)
}

func (s *remoteApplicationSuite) TestInferEndpointsWrongScope(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingService(c, "logging", subCharm)
	_, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *remoteApplicationSuite) TestAddRemoteApplicationErrors(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "haha/borken", URL: "local:/u/me/mysql", SourceModel: s.State.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "haha/borken": name "haha/borken" not valid`)
	_, err = s.State.RemoteApplication("haha/borken")
	c.Assert(err, gc.ErrorMatches, `remote application name "haha/borken" not valid`)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "borken", URL: "haha/borken", SourceModel: s.State.ModelTag()})
	c.Assert(err, gc.ErrorMatches,
		`cannot add remote application "borken": validating application URL: `+
			`application URL has invalid form, missing "/u/<user>": "haha/borken"`,
	)
	_, err = s.State.RemoteApplication("borken")
	c.Assert(err, gc.ErrorMatches, `remote application "borken" not found`)
}

func (s *remoteApplicationSuite) TestAddRemoteApplication(c *gc.C) {
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferName: "bar", URL: "local:/u/me/foo", SourceModel: s.State.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Name(), gc.Equals, "foo")
	c.Assert(foo.Registered(), jc.IsFalse)
	foo, err = s.State.RemoteApplication("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Name(), gc.Equals, "foo")
	c.Assert(foo.OfferName(), gc.Equals, "bar")
	c.Assert(foo.Registered(), jc.IsFalse)
	c.Assert(foo.SourceModel().Id(), gc.Equals, s.State.ModelTag().Id())
}

func (s *remoteApplicationSuite) TestAddRemoteApplicationRegistered(c *gc.C) {
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", URL: "local:/u/me/foo", SourceModel: s.State.ModelTag(), Registered: true})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Registered(), jc.IsTrue)
	foo, err = s.State.RemoteApplication("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Name(), gc.Equals, "foo")
	c.Assert(foo.Registered(), jc.IsTrue)
}

func (s *remoteApplicationSuite) TestAddRemoteRelationWrongScope(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingService(c, "logging", subCharm)
	ep1 := state.Endpoint{
		ApplicationName: "mysql",
		Relation: charm.Relation{
			Interface: "logging",
			Name:      "logging",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	ep2 := state.Endpoint{
		ApplicationName: "logging",
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

func (s *remoteApplicationSuite) TestAddRemoteRelationLocalFirst(c *gc.C) {
	s.assertAddRemoteRelation(c, "wordpress", "mysql")
}

func (s *remoteApplicationSuite) TestAddRemoteRelationRemoteFirst(c *gc.C) {
	s.assertAddRemoteRelation(c, "mysql", "wordpress")
}

func (s *remoteApplicationSuite) assertAddRemoteRelation(c *gc.C, application1, application2 string) {
	endpoints := map[string]state.Endpoint{
		"wordpress": state.Endpoint{
			ApplicationName: "wordpress",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		},
		"mysql": state.Endpoint{
			ApplicationName: "mysql",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints(application1, application2)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.String(), gc.Equals, "wordpress:db mysql:db")
	c.Assert(rel.Endpoints(), jc.DeepEquals, []state.Endpoint{endpoints[application1], endpoints[application2]})
	remoteSvc, err := s.State.RemoteApplication("mysql")
	c.Assert(err, jc.ErrorIsNil)
	relations, err := remoteSvc.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relations, gc.HasLen, 1)
	c.Assert(relations[0], jc.DeepEquals, rel)
}

func (s *remoteApplicationSuite) TestDestroySimple(c *gc.C) {
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.application.Life(), gc.Equals, state.Dying)
	err = s.application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyWithRemovableRelation(c *gc.C) {
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	// Destroy a local application with no units in relation scope; check application and
	// unit removed.
	err = wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyWithReferencedRelation(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *remoteApplicationSuite) TestDestroyWithReferencedRelationStaleCount(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *remoteApplicationSuite) assertDestroyWithReferencedRelation(c *gc.C, refresh bool) {
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

	// Optionally update the application document to get correct relation counts.
	if refresh {
		err = s.application.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}

	// Destroy, and check that the first relation becomes Dying...
	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = rel0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel0.Life(), gc.Equals, state.Dying)

	// ...while the second is removed directly.
	err = rel1.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Drop the last reference to the first relation; check the relation and
	// the application are are both removed.
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestAllRemoteApplicationsNone(c *gc.C) {
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	applications, err := s.State.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(applications), gc.Equals, 0)
}

func (s *remoteApplicationSuite) TestAllRemoteApplications(c *gc.C) {
	// There's initially the application created in test setup.
	applications, err := s.State.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(applications), gc.Equals, 1)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "another", URL: "local:/u/me/another", SourceModel: s.State.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	applications, err = s.State.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(applications, gc.HasLen, 2)

	// Check the returned application, order is defined by sorted keys.
	names := make([]string, len(applications))
	for i, svc := range applications {
		names[i] = svc.Name()
	}
	sort.Strings(names)
	c.Assert(names[0], gc.Equals, "another")
	c.Assert(names[1], gc.Equals, "mysql")
}

func (s *remoteApplicationSuite) TestAddApplicationEnvironmentDying(c *gc.C) {
	// Check that applications cannot be added if the environment is initially Dying.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", URL: "local:/u/me/s1", SourceModel: s.State.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": model is no longer alive`)
}

func (s *remoteApplicationSuite) TestAddApplicationSameLocalExists(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", URL: "local:/u/me/s1", SourceModel: s.State.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": local application with same name already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationLocalAddedAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a local application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", URL: "local:/u/me/s1", SourceModel: s.State.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": local application with same name already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationSameRemoteExists(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", URL: "local:/u/me/s1", SourceModel: s.State.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", URL: "local:/u/me/s1", SourceModel: s.State.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": remote application already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationRemoteAddedAfterInitial(c *gc.C) {
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a remote application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
			Name: "s1", URL: "local:/u/me/s1", SourceModel: s.State.ModelTag()})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", URL: "local:/u/me/s1", SourceModel: s.State.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": remote application already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationEnvironDiesAfterInitial(c *gc.C) {
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a remote application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		model, err := s.State.Model()
		c.Assert(err, jc.ErrorIsNil)
		err = model.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", URL: "local:/u/me/s1", SourceModel: s.State.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": model "testenv" is no longer alive`)
}

func (s *remoteApplicationSuite) TestWatchRemoteApplications(c *gc.C) {
	w := s.State.WatchRemoteApplications()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("mysql") // initial
	wc.AssertNoChange()

	db2, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "db2", URL: "local:/u/ibm/db2", SourceModel: s.State.ModelTag()})
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

func (s *remoteApplicationSuite) TestWatchRemoteApplicationsDying(c *gc.C) {
	w := s.State.WatchRemoteApplications()
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

	// Add a unit to the relation so the remote application is not
	// short-circuit removed.
	unit, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChangeInSingleEvent("mysql")
	wc.AssertNoChange()
}
