// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type remoteApplicationSuite struct {
	ConnSuite
	application            *state.RemoteApplication
	externalControllerUUID string
}

var _ = gc.Suite(&remoteApplicationSuite{})

func (s *remoteApplicationSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.externalControllerUUID = utils.MustNewUUID().String()
	s.makeRemoteApplication(c, "mysql", "me/model.mysql")
	rc, err := state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc, gc.Equals, 1)
}

func (s *remoteApplicationSuite) makeRemoteApplication(c *gc.C, name, url string) {
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
	mac, err := newMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	s.application, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:                   name,
		URL:                    url,
		ExternalControllerUUID: s.externalControllerUUID,
		SourceModel:            s.Model.ModelTag(),
		Token:                  "app-token",
		Endpoints:              eps,
		Spaces:                 spaces,
		Bindings:               bindings,
		Macaroon:               mac,
	})
	c.Assert(err, jc.ErrorIsNil)
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

func (s *remoteApplicationSuite) TestUseSuppliedVersionForConsumerProxy(c *gc.C) {
	application, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  666,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(application.ConsumeVersion(), gc.Equals, 666)
}

func (s *remoteApplicationSuite) TestStateApplicationRemoveExisting(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  666,
	})
	c.Assert(err, jc.ErrorIsNil)

	application, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  668,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(application.ConsumeVersion(), gc.Equals, 668)
}

func (s *remoteApplicationSuite) TestStateApplicationRemoveExistingWithRelations(c *gc.C) {
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  666,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Relate the remote app to a local application and put both a local and a
	// remote unit in scope.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressUnit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	proxyEP, err := app.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(proxyEP, wordpressEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru, err := rel.Unit(wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysqlru.EnterScope(nil), jc.ErrorIsNil)
	rru, err := rel.RemoteUnit("hosted-mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rru.EnterScope(nil), jc.ErrorIsNil)

	// Re-consume with a newer version: the old remote app and its relations are
	// torn down synchronously before the new remote app exists.
	app, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  668,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.ConsumeVersion(), gc.Equals, 668)

	// The old relation is gone; its units are out of scope.
	err = rel.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	inScope, err := mysqlru.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inScope, jc.IsFalse)
	inScope, err = rru.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inScope, jc.IsFalse)

	// The queued scopes/settings cleanups remove all leftover docs.
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)
	for _, collName := range []string{"relationscopes", "settings"} {
		coll := s.Session.DB("juju").C(collName)
		n, err := coll.Find(bson.M{"_id": bson.M{"$regex": "^" + state.DocID(s.State, "r#")}}).Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 0,
			gc.Commentf("leftover %s docs for removed relation", collName))
	}
}

func (s *remoteApplicationSuite) TestReplaceStaleProxyQueuesConvergenceCleanups(c *gc.C) {
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  666,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressUnit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	proxyEP, err := app.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(proxyEP, wordpressEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru, err := rel.Unit(wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mysqlru.EnterScope(nil), jc.ErrorIsNil)
	rru, err := rel.RemoteUnit("hosted-mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rru.EnterScope(nil), jc.ErrorIsNil)
	oldRelID := rel.Id()

	// Re-consume with a newer version.
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  668,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	coll := s.Session.DB("juju").C("cleanups")
	n, err := coll.Find(bson.M{
		"model-uuid": s.State.ModelUUID(),
		"kind":       "forceDestroyRelation",
		"prefix":     fmt.Sprintf("%d", oldRelID),
	}).Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(n, gc.Equals, 1,
		gc.Commentf("fencing the old relation must queue a force destroy cleanup"))

	// The queued cleanup is harmless once the replacement has completed:
	// it finds the relation gone and only removes leftover docs.
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)
}

func (s *remoteApplicationSuite) TestReplaceStaleProxyDoesNotResurrectDying(c *gc.C) {
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  666,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Relate the proxy to a local application, with no units in scope.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	proxyEP, err := app.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(proxyEP, wordpressEP)
	c.Assert(err, jc.ErrorIsNil)

	// Mark the proxy Dying directly, as a concurrent non-force
	// remove-saas would while the relation teardown is deferred to
	// queued cleanups.
	coll := s.Session.DB("juju").C("remoteApplications")
	err = coll.UpdateId(state.DocID(s.State, "hosted-mysql"),
		bson.M{"$set": bson.M{"life": 1}})
	c.Assert(err, jc.ErrorIsNil)

	// Re-consuming a newer version must not resurrect the Dying proxy:
	// the replacement fails fast with a legible error instead of
	// asserting the replaced app is Alive on every retry.
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  668,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
	c.Assert(err, gc.ErrorMatches, `(?s)cannot add saas application "hosted-mysql": saas application "hosted-mysql" is being removed; retry the consume once the removal completes`)

	// The proxy is still Dying with the old consume version.
	got, err := s.State.RemoteApplication("hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.Life(), gc.Equals, state.Dying)
	c.Assert(got.ConsumeVersion(), gc.Equals, 666)
}

func (s *remoteApplicationSuite) TestReplaceStaleProxyDoesNotFenceNewerRelation(c *gc.C) {
	args := state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "token-v1",
		IsConsumerProxy: true,
		ConsumeVersion:  1,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	}
	app, err := s.State.AddRemoteApplication(args)
	c.Assert(err, jc.ErrorIsNil)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	localEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	remoteEP, err := app.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	oldRelation, err := s.State.AddRelation(localEP, remoteEP)
	c.Assert(err, jc.ErrorIsNil)

	var newRelation *state.Relation
	var relationUnit *state.RelationUnit
	defer state.SetBeforeHooks(c, s.State, func() {
		// A newer consume wins before the older request fences its
		// relations. Its new relation reuses the old relation's key.
		newer := args
		newer.ConsumeVersion = 3
		newer.Token = "token-v3"
		replacement, err := s.State.AddRemoteApplication(newer)
		c.Assert(err, jc.ErrorIsNil)
		ep, err := replacement.Endpoint("db")
		c.Assert(err, jc.ErrorIsNil)
		newRelation, err = s.State.AddRelation(localEP, ep)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(newRelation.String(), gc.Equals, oldRelation.String())
		c.Check(newRelation.Id(), gc.Not(gc.Equals), oldRelation.Id())
		relationUnit, err = newRelation.Unit(unit)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(relationUnit.EnterScope(nil), jc.ErrorIsNil)
	}).Check()
	args.ConsumeVersion = 2
	args.Token = "token-v2"
	_, err = s.State.AddRemoteApplication(args)
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)

	// The losing request must not damage the winning proxy or relation,
	// including when queued cleanup work runs afterwards.
	c.Assert(s.State.Cleanup(fakeSecretDeleter), jc.ErrorIsNil)
	current, err := s.State.RemoteApplication(args.Name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(current.ConsumeVersion(), gc.Equals, 3)
	c.Check(current.RelationCount(), gc.Equals, 1)
	token, err := s.State.RemoteEntities().GetToken(current.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(token, gc.Equals, "token-v3")
	c.Assert(newRelation.Refresh(), jc.ErrorIsNil)
	c.Check(newRelation.Life(), gc.Equals, state.Alive)
	inScope, err := relationUnit.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(inScope, jc.IsTrue)
}

func (s *remoteApplicationSuite) TestReplaceStaleProxyRoundTripsFields(c *gc.C) {
	mkSpace := func(name string) *environs.ProviderSpaceInfo {
		return &environs.ProviderSpaceInfo{
			SpaceInfo: network.SpaceInfo{
				Name: network.SpaceName(name),
			},
		}
	}
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "old-token",
		IsConsumerProxy: true,
		ConsumeVersion:  666,
		Spaces:          []*environs.ProviderSpaceInfo{mkSpace("space-a")},
		Bindings:        map[string]string{"db": "space-a"},
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Seed a macaroon on the old document directly; the new version
	// does not carry one and the replacement must clear it.
	coll := s.Session.DB("juju").C("remoteApplications")
	err = coll.UpdateId(state.DocID(s.State, "hosted-mysql"),
		bson.M{"$set": bson.M{"macaroon": "old-mac"}})
	c.Assert(err, jc.ErrorIsNil)

	// Replace with a newer version that omits the URL and macaroon
	// and changes endpoints and bindings.
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "new-token",
		IsConsumerProxy: true,
		ConsumeVersion:  668,
		Spaces:          []*environs.ProviderSpaceInfo{mkSpace("space-a"), mkSpace("space-b")},
		Bindings:        map[string]string{"dbadmin": "space-b"},
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     2,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}, {
			Interface: "mysql-root",
			Limit:     1,
			Name:      "dbadmin",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Every field of the new version must round trip. Consumer proxies
	// never record an offer uuid on the document (the offer details live
	// in the offer connection), so it stays empty across the swap.
	got, err := s.State.RemoteApplication("hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	url, ok := got.URL()
	c.Assert(ok, jc.IsFalse)
	c.Assert(url, gc.Equals, "")
	c.Assert(got.OfferUUID(), gc.Equals, "")
	c.Assert(got.ConsumeVersion(), gc.Equals, 668)
	c.Assert(got.IsConsumerProxy(), jc.IsTrue)
	c.Assert(got.Life(), gc.Equals, state.Alive)
	c.Assert(got.RelationCount(), gc.Equals, 0)
	c.Assert(got.Bindings(), jc.DeepEquals, map[string]string{"dbadmin": "space-b"})
	_, err = got.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	_, err = got.Endpoint("dbadmin")
	c.Assert(err, jc.ErrorIsNil)

	// The omitted fields are omitted from the stored document.
	var raw bson.M
	err = coll.FindId(state.DocID(s.State, "hosted-mysql")).One(&raw)
	c.Assert(err, jc.ErrorIsNil)
	_, ok = raw["url"]
	c.Assert(ok, jc.IsFalse)
	_, ok = raw["macaroon"]
	c.Assert(ok, jc.IsFalse)
}

func (s *remoteApplicationSuite) TestStateApplicationRemoveExistingCorruptCountAborts(c *gc.C) {
	app, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  666,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	proxyEP, err := app.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(proxyEP, wordpressEP)
	c.Assert(err, jc.ErrorIsNil)

	// Load a stale handle, then corrupt the recorded count in state.
	stale, err := s.State.RemoteApplication("hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	coll := s.Session.DB("juju").C("remoteApplications")
	err = coll.UpdateId(state.DocID(s.State, "hosted-mysql"),
		bson.M{"$set": bson.M{"relationcount": 2}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stale.ConsumeVersion(), gc.Equals, 666)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "hosted-mysql",
		URL:             "me/model.mysql",
		SourceModel:     s.Model.ModelTag(),
		Token:           "app-token",
		IsConsumerProxy: true,
		ConsumeVersion:  668,
	})
	c.Assert(err, jc.ErrorIs, stateerrors.RelationCountCorruptError)
	c.Assert(err, gc.ErrorMatches,
		`(?s)cannot add saas application "hosted-mysql": cannot replace saas application "hosted-mysql": `+
			`relation count for remote application "hosted-mysql" is corrupt: recorded 2, actual relations 1; `+
			`force remove the saas application with 'juju remove-saas hosted-mysql --force' to clean up its relations`)

	// Nothing changed: the old remote app and relation are intact.
	app2, err := s.State.RemoteApplication("hosted-mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app2.ConsumeVersion(), gc.Equals, 666)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Alive)
}

func (s *remoteApplicationSuite) TestConsumeVersion(c *gc.C) {
	c.Assert(s.application.ConsumeVersion(), gc.Equals, 1)
	application, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "hosted-mysql",
		URL:         "me/model.mysql",
		SourceModel: s.Model.ModelTag(),
		Token:       "app-token",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(application.ConsumeVersion(), gc.Equals, 2)
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Check(err, jc.ErrorIsNil)

	statusInfo, err := s.application.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: saas application "mysql" not found`)
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
	c.Assert(err, gc.ErrorMatches, `saas application "mysql" has no "foo" relation`)

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
	mac, err := newMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	appMac, err := s.application.Macaroon()
	c.Assert(err, jc.ErrorIsNil)
	assertMacaroonEquals(c, appMac, mac)
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
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:db": cannot add relation between saas applications "wordpress" and "mysql"`)
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
	c.Assert(err, gc.ErrorMatches, `cannot add saas application "haha/borken": name "haha/borken" not valid`)
	_, err = s.State.RemoteApplication("haha/borken")
	c.Assert(err, gc.ErrorMatches, `saas application name "haha/borken" not valid`)

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "borken", URL: "haha/borken", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches,
		`cannot add saas application "borken": validating offer URL: `+
			`application offer URL is missing application`,
	)
	_, err = s.State.RemoteApplication("borken")
	c.Assert(err, gc.ErrorMatches, `saas application "borken" not found`)
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

func (s *remoteApplicationSuite) TestSetSourceController(c *gc.C) {
	foo, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "foo", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = foo.SetSourceController("source-controller-uuid")
	c.Assert(err, jc.ErrorIsNil)

	// Test results without and then with refresh.
	for i := 0; i < 2; i++ {
		sourceCtrl := foo.SourceController()
		c.Assert(sourceCtrl, gc.Equals, "source-controller-uuid")

		err = foo.Refresh()
		c.Assert(err, jc.ErrorIsNil)
	}
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
	_, err = state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

}

func (s *remoteApplicationSuite) TestDestroyRemovesExternalController(c *gc.C) {
	ec := state.NewExternalControllers(s.State)
	err := ec.Save(crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(s.externalControllerUUID),
		Addrs:         []string{"10.0.0.1:17070"},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.application.Life(), gc.Equals, state.Dying)
	err = s.application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = ec.Controller(s.externalControllerUUID)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyDoesNotRemoveExternalController(c *gc.C) {
	s.makeRemoteApplication(c, "mariadb", "user/model.mariadb")
	rc, err := state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc, gc.Equals, 2)

	ec := state.NewExternalControllers(s.State)
	err = ec.Save(crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(s.externalControllerUUID),
		Addrs:         []string{"10.0.0.1:17070"},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.application.Life(), gc.Equals, state.Dying)
	err = s.application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = ec.Controller(s.externalControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	rc, err = state.ControllerRefCount(s.State, s.externalControllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc, gc.Equals, 1)
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
	// the application are both removed.
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	err = rel0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyAlsoDeletesSecretConsumerInfo(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "another", ch)
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   app.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.SaveSecretRemoteConsumer(uri, s.application.Tag(), &secrets.SecretConsumerMetadata{CurrentRevision: 666})
	c.Assert(err, jc.ErrorIsNil)

	unit := names.NewUnitTag(s.application.Name() + "/666")
	err = s.State.SaveSecretRemoteConsumer(uri, unit, &secrets.SecretConsumerMetadata{CurrentRevision: 667})
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.GetSecretRemoteConsumer(uri, s.application.Tag())
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.GetSecretRemoteConsumer(uri, unit)
	c.Assert(err, jc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.GetSecretRemoteConsumer(uri, s.application.Tag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.GetSecretRemoteConsumer(uri, unit)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *remoteApplicationSuite) TestDestroyDoesNotDeletePrefixNamedConsumerInfo(c *gc.C) {
	ch := s.AddTestingCharm(c, "wordpress")
	app := s.AddTestingApplication(c, "another", ch)
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   app.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err := store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	// Remote consumer records for the application being destroyed ("mysql").
	err = s.State.SaveSecretRemoteConsumer(uri, s.application.Tag(), &secrets.SecretConsumerMetadata{CurrentRevision: 1})
	c.Assert(err, jc.ErrorIsNil)
	destroyedUnit := names.NewUnitTag(s.application.Name() + "/0")
	err = s.State.SaveSecretRemoteConsumer(uri, destroyedUnit, &secrets.SecretConsumerMetadata{CurrentRevision: 1})
	c.Assert(err, jc.ErrorIsNil)

	// Remote consumer records for a different application whose name has
	// "mysql" as a prefix.
	siblingApp := names.NewApplicationTag("mysql-root")
	siblingUnit := names.NewUnitTag("mysql-root/0")
	err = s.State.SaveSecretRemoteConsumer(uri, siblingApp, &secrets.SecretConsumerMetadata{CurrentRevision: 1})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SaveSecretRemoteConsumer(uri, siblingUnit, &secrets.SecretConsumerMetadata{CurrentRevision: 1})
	c.Assert(err, jc.ErrorIsNil)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	// The destroyed application's records are gone.
	_, err = s.State.GetSecretRemoteConsumer(uri, s.application.Tag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.GetSecretRemoteConsumer(uri, destroyedUnit)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// The prefix-named application's records survive.
	_, err = s.State.GetSecretRemoteConsumer(uri, siblingApp)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.GetSecretRemoteConsumer(uri, siblingUnit)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *remoteApplicationSuite) TestDestroyAlsoDeletesSecretPermissions(c *gc.C) {
	wpEP := []charm.Relation{
		{Name: "db", Interface: "mysql", Role: charm.RoleRequirer, Scope: charm.ScopeGlobal},
	}

	wp, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "remote-wordpress", OfferUUID: "offer-uuid", SourceModel: s.Model.ModelTag(),
		Endpoints:       wpEP,
		IsConsumerProxy: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "mysqldb"})

	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   mysql.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Label:       ptr("label"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(state.Endpoint{
		ApplicationName: "remote-wordpress",
		Relation:        wpEP[0],
	}, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       rel.Tag(),
		Subject:     wp.Tag(),
		Role:        secrets.RoleView,
	})
	c.Assert(err, jc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, wp.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleView)

	_, err = wp.DestroyWithForce(true, time.Duration(0))
	c.Assert(err, jc.ErrorIsNil)
	access, err = s.State.SecretAccess(uri, wp.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleNone)
}

func (s *remoteApplicationSuite) TestDestroyRemovesStatusHistory(c *gc.C) {
	err := s.application.SetStatus(status.StatusInfo{
		Status: status.Active,
	})
	c.Assert(err, jc.ErrorIsNil)
	filter := status.StatusHistoryFilter{Size: 100}
	agentInfo, err := s.application.StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(agentInfo), gc.Equals, 1)

	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	agentInfo, err = s.application.StatusHistory(filter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agentInfo, gc.HasLen, 0)
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
	if appStatus == nil || *appStatus != status.Terminated {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(s.application.Life(), gc.Equals, state.Dying)
	} else {
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}

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

func (s *remoteApplicationSuite) TestDestroyTerminatedDead(c *gc.C) {
	err := s.application.SetStatus(status.StatusInfo{Status: status.Terminated})
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.SetDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Refresh()
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
	c.Assert(err, gc.ErrorMatches, `cannot add saas application "s1": model is no longer alive`)
}

func (s *remoteApplicationSuite) TestAddApplicationSameLocalExists(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	_, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "s1", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
			OS:      "ubuntu",
			Channel: "20.04/stable",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add saas application "s1": local application with same name already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationLocalAddedAfterInitial(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	// Check that a application with a name conflict cannot be added if
	// there is no conflict initially but a local application is added
	// before the transaction is run.
	defer state.SetBeforeHooks(c, s.State, func() {
		_, err := s.State.AddApplication(state.AddApplicationArgs{
			Name: "s1", Charm: charm,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{
				OS:      "ubuntu",
				Channel: "20.04/stable",
			}},
		})
		c.Assert(err, jc.ErrorIsNil)
	}).Check()
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add saas application "s1": local application with same name already exists`)
}

func (s *remoteApplicationSuite) TestAddApplicationSameRemoteExists(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "s1", SourceModel: s.Model.ModelTag()})
	c.Assert(err, gc.ErrorMatches, `cannot add saas application "s1": saas application already exists`)
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
	c.Assert(err, gc.ErrorMatches, `cannot add saas application "s1": saas application already exists`)
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
	c.Assert(err, gc.ErrorMatches, `cannot add saas application "s1": model "testmodel" is dying`)
}

func (s *remoteApplicationSuite) TestWatchRemoteApplications(c *gc.C) {
	w := s.State.WatchRemoteApplications()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("mysql") // initial
	wc.AssertNoChange()

	db2, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name: "db2", SourceModel: s.Model.ModelTag()})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange("db2")
	wc.AssertNoChange()

	err = db2.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = db2.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	wc.AssertChange("db2")
	wc.AssertNoChange()
}

func (s *remoteApplicationSuite) TestWatchRemoteApplicationsDying(c *gc.C) {
	w := s.State.WatchRemoteApplications()
	defer testing.AssertStop(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange("mysql") // initial
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

	wc.AssertChange("mysql")
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
	c.Assert(err, jc.ErrorIsNil)

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

	err = s.application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := s.application.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appStatus.Status, gc.Equals, status.Terminated)
	c.Assert(appStatus.Message, gc.Equals, "do-do-do do-do-do do-do")
	c.Assert(s.application.Life(), gc.Equals, state.Dead)

	remoteRelUnits1, err := rel1.AllRemoteUnits("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteRelUnits1, gc.HasLen, 0)

	remoteRelUnits2, err := rel2.AllRemoteUnits("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remoteRelUnits2, gc.HasLen, 0)
}
