// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
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

	spaces := []*environs.ProviderSpaceInfo{{
		CloudType: "ec2",
		ProviderAttributes: map[string]interface{}{
			"thing1":  23,
			"thing2":  "halberd",
			"network": "network-1",
		},
		SpaceInfo: network.SpaceInfo{
			Name:       "public",
			ProviderId: "juju-space-public",
			Subnets: []network.SubnetInfo{{
				ProviderId:        "juju-subnet-12",
				CIDR:              "1.2.3.0/24",
				AvailabilityZones: []string{"az1", "az2"},
				ProviderSpaceId:   "juju-space-public",
				ProviderNetworkId: "network-1",
			}},
		},
	}, {
		CloudType: "ec2",
		ProviderAttributes: map[string]interface{}{
			"thing1":  24,
			"thing2":  "bardiche",
			"network": "network-1",
		},
		SpaceInfo: network.SpaceInfo{
			Name:       "private",
			ProviderId: "juju-space-private",
			Subnets: []network.SubnetInfo{{
				ProviderId:        "juju-subnet-24",
				CIDR:              "1.2.4.0/24",
				AvailabilityZones: []string{"az1", "az2"},
				ProviderSpaceId:   "juju-space-private",
				ProviderNetworkId: "network-1",
			}},
		},
	}}
	bindings := map[string]string{
		"db":       "private",
		"db-admin": "private",
		"logging":  "public",
	}
	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	s.application, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		URL:         "me/model.mysql",
		SourceModel: s.Model.ModelTag(),
		Token:       "app-token",
		Endpoints:   eps,
		Spaces:      spaces,
		Bindings:    bindings,
		Macaroon:    mac,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *remoteApplicationSuite) assertApplicationRelations(c *gc.C, app *state.Application, expectedKeys ...string) []*state.Relation {
	rels, err := app.Relations()
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

func (s *remoteApplicationSuite) TestNoStatusForConsumerProxy(c *gc.C) {
	application, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = application.Status()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestInitialStatus(c *gc.C) {
	appStatus, err := s.application.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Since, gc.NotNil)
	appStatus.Since = nil
	c.Assert(appStatus, gc.DeepEquals, status.StatusInfo{
		Status: status.Unknown,
		Data:   map[string]interface{}{},
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
	c.Assert(url, gc.Equals, "me/model.mysql")

	// Add another remote application without a URL.
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql1",
		SourceModel: s.Model.ModelTag(),
		Token:       "app-token",
	})
	c.Assert(err, jc.ErrorIsNil)
	url, ok = app.URL()
	c.Assert(ok, jc.IsFalse)
	c.Assert(url, gc.Equals, "")
}

func (s *remoteApplicationSuite) TestSpaces(c *gc.C) {
	spaces := s.application.Spaces()
	c.Assert(spaces, gc.DeepEquals, []state.RemoteSpace{{
		CloudType:  "ec2",
		Name:       "public",
		ProviderId: "juju-space-public",
		ProviderAttributes: map[string]interface{}{
			"thing1":  23,
			"thing2":  "halberd",
			"network": "network-1",
		},
		Subnets: []state.RemoteSubnet{{
			ProviderId:        "juju-subnet-12",
			CIDR:              "1.2.3.0/24",
			AvailabilityZones: []string{"az1", "az2"},
			ProviderSpaceId:   "juju-space-public",
			ProviderNetworkId: "network-1",
		}},
	}, {
		CloudType:  "ec2",
		Name:       "private",
		ProviderId: "juju-space-private",
		ProviderAttributes: map[string]interface{}{
			"thing1":  24,
			"thing2":  "bardiche",
			"network": "network-1",
		},
		Subnets: []state.RemoteSubnet{{
			ProviderId:        "juju-subnet-24",
			CIDR:              "1.2.4.0/24",
			AvailabilityZones: []string{"az1", "az2"},
			ProviderSpaceId:   "juju-space-private",
			ProviderNetworkId: "network-1",
		}},
	}})
}

func (s *remoteApplicationSuite) TestSpaceForEndpoint(c *gc.C) {
	space, ok := s.application.SpaceForEndpoint("db")
	c.Assert(ok, jc.IsTrue)
	c.Assert(space.Name, gc.Equals, "private")
	space, ok = s.application.SpaceForEndpoint("logging")
	c.Assert(ok, jc.IsTrue)
	c.Assert(space.Name, gc.Equals, "public")
	space, ok = s.application.SpaceForEndpoint("something else")
	c.Assert(ok, jc.IsFalse)
}

func (s *remoteApplicationSuite) TestBindings(c *gc.C) {
	c.Assert(s.application.Bindings(), gc.DeepEquals, map[string]string{
		"db":       "private",
		"db-admin": "private",
		"logging":  "public",
	})
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

func (s *remoteApplicationSuite) TestMacaroon(c *gc.C) {
	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	appMac, err := s.application.Macaroon()
	c.Assert(err, jc.ErrorIsNil)
	apitesting.MacaroonEquals(c, appMac, mac)
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
		Name: "wordpress", Endpoints: wpep, SourceModel: s.Model.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:db": cannot add relation between remote applications "wordpress" and "mysql"`)
}

func (s *remoteApplicationSuite) TestInferEndpointsWrongScope(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingApplication(c, "logging", subCharm)
	_, err := s.State.InferEndpoints("logging", "mysql")
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *remoteApplicationSuite) TestAddRemoteApplicationErrors(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "haha/borken", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "haha/borken": name "haha/borken" not valid`)
	_, err = s.State.RemoteApplication("haha/borken")
	c.Assert(err, gc.ErrorMatches, `remote application name "haha/borken" not valid`)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "borken", URL: "haha/borken", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches,
		`cannot add remote application "borken": validating offer URL: `+
			`application offer URL is missing application`,
	)
	_, err = s.State.RemoteApplication("borken")
	c.Assert(err, gc.ErrorMatches, `remote application "borken" not found`)
}

func (s *remoteApplicationSuite) TestParamsValidateChecksBindings(c *gc.C) {
	eps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}

	spaces := []*environs.ProviderSpaceInfo{{
		SpaceInfo: network.SpaceInfo{
			Name: "public",
		},
	}}
	bindings := map[string]string{
		"db": "private",
	}
	args := state.AddRemoteApplicationParams{
		Name:        "mysql",
		URL:         "me/model.mysql",
		SourceModel: s.Model.ModelTag(),
		Token:       "app-token",
		Endpoints:   eps,
		Spaces:      spaces,
		Bindings:    bindings,
	}
	err := args.Validate()
	c.Assert(err, gc.ErrorMatches, `endpoint "db" bound to missing space "private" not valid`)
	bindings["db"] = "public"
	// Tolerates bindings for non-existent endpoints.
	bindings["gidget"] = "public"
	err = args.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *remoteApplicationSuite) TestAddRemoteApplication(c *gc.C) {
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", URL: "me/model.foo", SourceModel: s.Model.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Name(), gc.Equals, "foo")
	c.Assert(foo.IsConsumerProxy(), jc.IsFalse)
	foo, err = s.State.RemoteApplication("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Name(), gc.Equals, "foo")
	c.Assert(foo.OfferUUID(), gc.Equals, "offer-uuid")
	url, ok := foo.URL()
	c.Assert(ok, jc.IsTrue)
	c.Assert(url, gc.Equals, "me/model.foo")
	c.Assert(foo.IsConsumerProxy(), jc.IsFalse)
	c.Assert(foo.SourceModel().Id(), gc.Equals, s.Model.ModelTag().Id())
}

func (s *remoteApplicationSuite) TestAddRemoteApplicationFromConsumer(c *gc.C) {
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", SourceModel: s.Model.ModelTag(), IsConsumerProxy: true})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.IsConsumerProxy(), jc.IsTrue)
	foo, err = s.State.RemoteApplication("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(foo.Name(), gc.Equals, "foo")
	c.Assert(foo.IsConsumerProxy(), jc.IsTrue)
}

func (s *remoteApplicationSuite) TestAddEndpoints(c *gc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, jc.ErrorIsNil)

	newEps := []charm.Relation{
		{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}

	err = foo.AddEndpoints(newEps)
	c.Assert(err, jc.ErrorIsNil)

	var expected []state.Endpoint
	for _, r := range origEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}
	for _, r := range newEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}

	// Test results without and then with refresh.
	for i := 0; i < 2; i++ {
		eps, err := foo.Endpoints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(eps, jc.SameContents, expected)

		err = foo.Refresh()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *remoteApplicationSuite) TestAddEndpointsConflicting(c *gc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, jc.ErrorIsNil)

	newEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	err = foo.AddEndpoints(newEps)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, gc.ErrorMatches, "endpoint ep1 already exists")
}

func (s *remoteApplicationSuite) TestAddEndpointsConcurrentOneDeleted(c *gc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, jc.ErrorIsNil)

	reducedEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
	}
	defer state.SetBeforeHooks(c, s.State, func() {
		// Destroy foo and recreate with fewer endpoints to simulate
		// endpoint removal.
		err := foo.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
			Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
			Endpoints: reducedEps,
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	newEps := []charm.Relation{
		{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	err = foo.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = foo.AddEndpoints(newEps)
	c.Assert(err, jc.ErrorIsNil)

	var expected []state.Endpoint
	for _, r := range reducedEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}
	for _, r := range newEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}

	// Test results without and then with refresh.
	for i := 0; i < 2; i++ {
		eps, err := foo.Endpoints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(eps, jc.SameContents, expected)

		err = foo.Refresh()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *remoteApplicationSuite) TestAddEndpointsConcurrentConflictingOneAdded(c *gc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, jc.ErrorIsNil)

	defer state.SetBeforeHooks(c, s.State, func() {
		newEps := []charm.Relation{
			{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		}
		app, err := s.State.RemoteApplication("foo")
		c.Assert(err, jc.ErrorIsNil)
		err = app.AddEndpoints(newEps)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	newEps := []charm.Relation{
		{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	err = foo.AddEndpoints(newEps)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Assert(err, gc.ErrorMatches, "endpoint ep3 already exists")
}

func (s *remoteApplicationSuite) TestAddEndpointsConcurrentDifferentOneAdded(c *gc.C) {
	origEps := []charm.Relation{
		{Name: "ep1", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep2", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints: origEps,
	})
	c.Assert(err, jc.ErrorIsNil)

	concurrrentEps := []charm.Relation{
		{Name: "ep5", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
	}
	defer state.SetBeforeHooks(c, s.State, func() {
		app, err := s.State.RemoteApplication("foo")
		c.Assert(err, jc.ErrorIsNil)
		err = app.AddEndpoints(concurrrentEps)
		c.Assert(err, jc.ErrorIsNil)
	}).Check()

	newEps := []charm.Relation{
		{Name: "ep3", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal, Limit: 1},
		{Name: "ep4", Role: charm.RoleProvider, Scope: charm.ScopeGlobal, Limit: 1},
	}
	err = foo.AddEndpoints(newEps)
	c.Assert(err, jc.ErrorIsNil)

	var expected []state.Endpoint
	for _, r := range origEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}
	for _, r := range newEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}
	for _, r := range concurrrentEps {
		expected = append(expected, state.Endpoint{ApplicationName: "foo", Relation: r})
	}

	// Test results without and then with refresh.
	for i := 0; i < 2; i++ {
		eps, err := foo.Endpoints()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(eps, jc.SameContents, expected)

		err = foo.Refresh()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *remoteApplicationSuite) TestAddRemoteRelationWrongScope(c *gc.C) {
	subCharm := s.AddTestingCharm(c, "logging")
	s.AddTestingApplication(c, "logging", subCharm)
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
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:logging-client mysql:logging": local endpoint must be globally scoped for remote relations`)
}

func (s *remoteApplicationSuite) TestAddRemoteRelationLocalFirst(c *gc.C) {
	s.assertAddRemoteRelation(c, "wordpress", "mysql")
}

func (s *remoteApplicationSuite) TestAddRemoteRelationRemoteFirst(c *gc.C) {
	s.assertAddRemoteRelation(c, "mysql", "wordpress")
}

func (s *remoteApplicationSuite) assertAddRemoteRelation(c *gc.C, application1, application2 string) {
	endpoints := map[string]state.Endpoint{
		"wordpress": {
			ApplicationName: "wordpress",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleRequirer,
				Scope:     charm.ScopeGlobal,
				Limit:     1,
			},
		},
		"mysql": {
			ApplicationName: "mysql",
			Relation: charm.Relation{
				Interface: "mysql",
				Name:      "db",
				Role:      charm.RoleProvider,
				Scope:     charm.ScopeGlobal,
			},
		},
	}
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints(application1, application2)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.String(), gc.Equals, "wordpress:db mysql:db")
	c.Assert(rel.Endpoints(), jc.DeepEquals, []state.Endpoint{endpoints[application1], endpoints[application2]})
	remoteapp, err := s.State.RemoteApplication("mysql")
	c.Assert(err, jc.ErrorIsNil)
	relations, err := remoteapp.Relations()
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
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.application.Refresh(), jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)

	// Destroy the remote application with no units in relation scope; check application and
	// unit removed.
	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyWithRemoteTokens(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	// Add remote token so we can check it is cleaned up.
	re := s.State.RemoteEntities()
	relToken, err := re.ExportLocalEntity(rel.Tag())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.application.Refresh(), jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	_, err = re.GetToken(s.application.Tag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = re.GetToken(rel.Tag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = re.GetRemoteEntity("app-token")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = re.GetRemoteEntity(relToken)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyWithOfferConnections(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.application.Refresh(), jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)

	// Add a offer connection record so we can check it is cleaned up.
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: coretesting.ModelTag.Id(),
		RelationId:      rel.Id(),
		RelationKey:     rel.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	rc, err := s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), gc.Equals, 1)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	rc, err = s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), gc.Equals, 0)
}

func (s *remoteApplicationSuite) TestDestroyWithReferencedRelation(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, true)
}

func (s *remoteApplicationSuite) TestDestroyWithReferencedRelationStaleCount(c *gc.C) {
	s.assertDestroyWithReferencedRelation(c, false)
}

func (s *remoteApplicationSuite) assertDestroyWithReferencedRelation(c *gc.C, refresh bool) {
	ch := s.AddTestingCharm(c, "wordpress")
	wordpress := s.AddTestingApplication(c, "wordpress", ch)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel0, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)

	another := s.AddTestingApplication(c, "another", ch)
	eps, err = s.State.InferEndpoints("another", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(another.Refresh(), jc.ErrorIsNil)
	c.Assert(s.application.Refresh(), jc.ErrorIsNil)

	// Add a separate reference to the first relation.
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
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

func (s *remoteApplicationSuite) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, gc.Equals, inScope)
}

func (s *remoteApplicationSuite) assertDestroyAppWithStatus(c *gc.C, appStatus *status.Status) {
	mysqlEP, err := s.application.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wpUnit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	wpEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	rel, err := s.State.AddRelation(wpEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	wpru, err := rel.Unit(wpUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = wpru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpru, true)

	mysqlru, err := rel.RemoteUnit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlru, true)

	c.Assert(s.application.Refresh(), jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)

	if appStatus != nil {
		err = s.application.SetStatus(status.StatusInfo{Status: *appStatus})
		c.Assert(err, jc.ErrorIsNil)
	}

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.application.Life(), gc.Equals, state.Dying)

	// If the remote app is terminated, any remote units are
	// forcibly removed from scope, but not local ones.
	s.assertInScope(c, mysqlru, appStatus == nil || *appStatus != status.Terminated)
	s.assertInScope(c, wpru, true)
}

func (s *remoteApplicationSuite) TestDestroyNoStatus(c *gc.C) {
	s.assertDestroyAppWithStatus(c, nil)
}

func (s *remoteApplicationSuite) TestDestroyNotTerminated(c *gc.C) {
	appStatus := status.Active
	s.assertDestroyAppWithStatus(c, &appStatus)
}

func (s *remoteApplicationSuite) TestDestroyTerminated(c *gc.C) {
	appStatus := status.Terminated
	s.assertDestroyAppWithStatus(c, &appStatus)
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
		Name: "another", SourceModel: s.Model.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	applications, err = s.State.AllRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(applications, gc.HasLen, 2)

	// Check the returned application, order is defined by sorted keys.
	names := make([]string, len(applications))
	for i, app := range applications {
		names[i] = app.Name()
	}
	sort.Strings(names)
	c.Assert(names[0], gc.Equals, "another")
	c.Assert(names[1], gc.Equals, "mysql")
}

func (s *remoteApplicationSuite) TestAddApplicationModelDying(c *gc.C) {
	// Check that applications cannot be added if the model is initially Dying.
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": model is no longer alive`)
}

func (s *remoteApplicationSuite) TestAddApplicationSameLocalExists(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddApplication(state.AddApplicationArgs{Name: "s1", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
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
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": local application with same name already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationSameRemoteExists(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": remote application already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationRemoteAddedAfterInitial(c *gc.C) {
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a remote application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
			Name: "s1", SourceModel: s.Model.ModelTag()})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": remote application already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationModelDiesAfterInitial(c *gc.C) {
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a remote application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		model, err := s.State.Model()
		c.Assert(err, jc.ErrorIsNil)
		err = model.Destroy(state.DestroyModelParams{})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add remote application "s1": model "testmodel" is no longer alive`)
}

func (s *remoteApplicationSuite) TestWatchRemoteApplications(c *gc.C) {
	w := s.State.WatchRemoteApplications()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, s.State, w)
	wc.AssertChangeInSingleEvent("mysql") // initial
	wc.AssertNoChange()

	db2, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "db2", SourceModel: s.Model.ModelTag()})
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
	wordpress := s.AddTestingApplication(c, "wordpress", ch)
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.application.Refresh(), jc.ErrorIsNil)
	c.Assert(wordpress.Refresh(), jc.ErrorIsNil)

	// Add a unit to the relation so the remote application is not
	// short-circuit removed.
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
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

func (s *remoteApplicationSuite) TestTerminateOperationLeavesScopes(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")

	_ = s.AddTestingApplication(c, "wp1", ch)
	eps1, err := s.State.InferEndpoints("wp1", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel1, err := s.State.AddRelation(eps1...)
	c.Assert(err, jc.ErrorIsNil)

	_ = s.AddTestingApplication(c, "wp2", ch)
	eps2, err := s.State.InferEndpoints("wp2", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel2, err := s.State.AddRelation(eps2...)

	ru1, err := rel1.RemoteUnit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	err = ru1.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	ru2, err := rel2.RemoteUnit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	err = ru2.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	op := s.application.TerminateOperation("do-do-do do-do-do do-do")
	err = s.State.ApplyOperation(op)
	c.Assert(err, jc.ErrorIsNil)

	appStatus, err := s.application.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Status, gc.Equals, status.Terminated)
	c.Assert(appStatus.Message, gc.Equals, "do-do-do do-do-do do-do")

	remoteRelUnits1, err := rel1.AllRemoteUnits("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteRelUnits1, gc.HasLen, 0)

	remoteRelUnits2, err := rel2.AllRemoteUnits("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteRelUnits2, gc.HasLen, 0)
}
