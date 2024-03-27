// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/charm/v13"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type RelationSuite struct {
	ConnSuite
}

var _ = gc.Suite(&RelationSuite{})

func (s *RelationSuite) TestAddRelationErrors(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	riak := s.AddTestingApplication(c, "riak", s.AddTestingCharm(c, "riak"))
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)

	// Check we can't add a relation with application that don't exist.
	yoursqlEP := mysqlEP
	yoursqlEP.ApplicationName = "yoursql"
	_, err = s.State.AddRelation(yoursqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db yoursql:server": application "yoursql" does not exist`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that interfaces have to match.
	msep3 := mysqlEP
	msep3.Interface = "roflcopter"
	_, err = s.State.AddRelation(msep3, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": endpoints do not relate`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check a variety of surprising endpoint combinations.
	_, err = s.State.AddRelation(wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db": relation must have two endpoints`)
	assertNoRelations(c, wordpress)

	_, err = s.State.AddRelation(riakEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db riak:ring": endpoints do not relate`)
	assertOneRelation(c, riak, 0, riakEP)
	assertNoRelations(c, wordpress)

	_, err = s.State.AddRelation(riakEP, riakEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "riak:ring riak:ring": endpoints do not relate`)
	assertOneRelation(c, riak, 0, riakEP)

	_, err = s.State.AddRelation()
	c.Assert(err, gc.ErrorMatches, `cannot add relation "": relation must have two endpoints`)
	_, err = s.State.AddRelation(mysqlEP, wordpressEP, riakEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server riak:ring": relation must have two endpoints`)
	assertOneRelation(c, riak, 0, riakEP)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that a relation can't be added to a Dying application.
	_, err = wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": application "wordpress" is not alive`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)
}

func (s *StateSuite) TestAddRelationWithMaxLimit(c *gc.C) {
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "mariadb", s.AddTestingCharm(c, "mariadb"))
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// First relation should be established without an issue
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Attempting to add a new relation between wordpress and mariadb
	// should fail because wordpress:db specifies a max relation limit of 1
	eps, err = s.State.InferEndpoints("wordpress", "mariadb")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIs, errors.QuotaLimitExceeded, gc.Commentf("expected second AddRelation attempt to fail due to the limit:1 entry in the wordpress charm's metadata.yaml"))
}

func (s *RelationSuite) TestRetrieveSuccess(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlUnit, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	expect, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru, err := expect.Unit(mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	rel, err := s.State.EndpointsRelation(wordpressEP, mysqlEP)
	check := func() {
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(rel.Id(), gc.Equals, expect.Id())
		c.Assert(rel.String(), gc.Equals, expect.String())
		c.Assert(rel.UnitCount(), gc.Equals, 1)
	}
	check()
	rel, err = s.State.EndpointsRelation(mysqlEP, wordpressEP)
	check()
	rel, err = s.State.Relation(expect.Id())
	check()
}

func (s *RelationSuite) TestRetrieveNotFound(c *gc.C) {
	subway := state.Endpoint{
		ApplicationName: "subway",
		Relation: charm.Relation{
			Name:      "db",
			Interface: "mongodb",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		},
	}
	mongo := state.Endpoint{
		ApplicationName: "mongo",
		Relation: charm.Relation{
			Name:      "server",
			Interface: "mongodb",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	_, err := s.State.EndpointsRelation(subway, mongo)
	c.Assert(err, gc.ErrorMatches, `relation "subway:db mongo:server" not found`)
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	_, err = s.State.Relation(999)
	c.Assert(err, gc.ErrorMatches, `relation 999 not found`)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RelationSuite) TestAddRelation(c *gc.C) {
	// Add a relation.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)

	// Check we cannot re-add the same relation, regardless of endpoint ordering.
	_, err = s.State.AddRelation(mysqlEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation wordpress:db mysql:server`)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation wordpress:db mysql:server`)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)
}

func (s *RelationSuite) TestAddRelationSeriesNeedNotMatch(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddSeriesCharm(c, "mysql", "bionic"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	assertOneRelation(c, mysql, 0, mysqlEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, mysqlEP)
}

func (s *RelationSuite) TestAddContainerRelation(c *gc.C) {
	// Add a relation.
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	logging := s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the endpoints both have container scope.
	wordpressEP.Scope = charm.ScopeContainer
	assertOneRelation(c, logging, 0, loggingEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, loggingEP)

	// Check we cannot re-add the same relation, regardless of endpoint ordering.
	_, err = s.State.AddRelation(loggingEP, wordpressEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": relation logging:info wordpress:juju-info`)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": relation logging:info wordpress:juju-info`)
	assertOneRelation(c, logging, 0, loggingEP, wordpressEP)
	assertOneRelation(c, wordpress, 0, wordpressEP, loggingEP)
}

func (s *RelationSuite) TestAddContainerRelationSeriesMustMatch(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	logging := s.AddTestingApplication(c, "logging", s.AddSeriesCharm(c, "logging-raring", "bionic"))
	loggingEP, err := logging.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, loggingEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "logging:info wordpress:juju-info": principal and subordinate applications' bases must match`)
}

func (s *RelationSuite) TestAddContainerRelationMultiSeriesMatch(c *gc.C) {
	principal := s.AddTestingApplication(c, "multi-series", s.AddSeriesCharm(c, "multi-series", "quantal"))
	principalEP, err := principal.Endpoint("multi-directory")
	c.Assert(err, jc.ErrorIsNil)
	subord := s.AddTestingApplication(c, "multi-series-subordinate", s.AddSeriesCharm(c, "multi-series-subordinate", "bionic"))
	subordEP, err := subord.Endpoint("multi-directory")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRelation(principalEP, subordEP)
	principalEP.Scope = charm.ScopeContainer
	c.Assert(err, jc.ErrorIsNil)
	assertOneRelation(c, subord, 0, subordEP, principalEP)
	assertOneRelation(c, principal, 0, principalEP, subordEP)
}

func (s *RelationSuite) TestAddContainerRelationMultiSeriesNoMatch(c *gc.C) {
	principal := s.AddTestingApplication(c, "multi-series", s.AddTestingCharm(c, "multi-series"))
	principalEP, err := principal.Endpoint("multi-directory")
	c.Assert(err, jc.ErrorIsNil)
	meta := `
name: multi-series-subordinate
summary: a test charm
description: a test
subordinate: true
series:
    - xenial
requires:
    multi-directory:
       interface: logging
       scope: container
`[1:]
	subord := s.AddTestingApplication(c, "multi-series-subordinate", s.AddMetaCharm(c, "multi-series-subordinate", meta, 1))
	subordEP, err := subord.Endpoint("multi-directory")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRelation(principalEP, subordEP)
	principalEP.Scope = charm.ScopeContainer
	c.Assert(err, gc.ErrorMatches, `cannot add relation "multi-series-subordinate:multi-directory multi-series:multi-directory": principal and subordinate applications' bases must match`)
}

func (s *RelationSuite) TestAddContainerRelationWithNoSubordinate(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressSubEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	wordpressSubEP.Scope = charm.ScopeContainer
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRelation(mysqlEP, wordpressSubEP)
	c.Assert(err, gc.ErrorMatches,
		`cannot add relation "wordpress:db mysql:server": container scoped relation requires at least one subordinate application`)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)
}

func (s *RelationSuite) TestAddContainerRelationWithTwoSubordinates(c *gc.C) {
	loggingCharm := s.AddTestingCharm(c, "logging")
	logging1 := s.AddTestingApplication(c, "logging1", loggingCharm)
	logging1EP, err := logging1.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	logging2 := s.AddTestingApplication(c, "logging2", loggingCharm)
	logging2EP, err := logging2.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddRelation(logging1EP, logging2EP)
	c.Assert(err, jc.ErrorIsNil)
	// AddRelation changes the scope on the endpoint if relation is container scoped.
	logging1EP.Scope = charm.ScopeContainer
	assertOneRelation(c, logging1, 0, logging1EP, logging2EP)
	assertOneRelation(c, logging2, 0, logging2EP, logging1EP)
}

func (s *RelationSuite) TestDestroyRelation(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// Test that the relation can be destroyed.
	err = rel.Destroy(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	assertNoRelations(c, wordpress)
	assertNoRelations(c, mysql)

	// Check that a second destroy is a no-op.
	err = rel.Destroy(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Create a new relation and check that refreshing the old does not find
	// the new.
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RelationSuite) TestDestroyPeerRelation(c *gc.C) {
	// Check that a peer relation cannot be destroyed directly.
	riakch := s.AddTestingCharm(c, "riak")
	riak := s.AddTestingApplication(c, "riak", riakch)
	riakEP, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rel := assertOneRelation(c, riak, 0, riakEP)
	err = rel.Destroy(nil)
	c.Assert(err, gc.ErrorMatches, `cannot destroy relation "riak:ring": is a peer relation`)
	assertOneRelation(c, riak, 0, riakEP)

	// Check that it is destroyed when the application is destroyed.
	err = riak.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	assertNoRelations(c, riak)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	// Create a new application (and hence a new relation in the background); check
	// that refreshing the old one does not accidentally get the new one.
	newriak := s.AddTestingApplication(c, "riak", riakch)
	assertOneRelation(c, newriak, 1, riakEP)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RelationSuite) TestDestroyRelationIncorrectUnitCount(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	prr.allEnterScope(c)

	rel := prr.rel
	state.RemoveUnitRelations(c, rel)
	err := rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.UnitCount(), gc.Not(gc.Equals), 0)

	_, err = rel.DestroyWithForce(false, dontWait)
	c.Assert(err, gc.ErrorMatches, ".*unit count mismatch on relation wordpress:db mysql:server: expected 4 units in scope but got 0")
}

func (s *RelationSuite) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, gc.Equals, inScope)
}

func (s *RelationSuite) assertDestroyCrossModelRelation(c *gc.C, appStatus *status.Status) {
	rwordpress, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-wordpress",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := rwordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlUnit, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	rel, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru, err := rel.Unit(mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlru, true)

	wpru, err := rel.RemoteUnit("remote-wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = wpru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpru, true)

	if appStatus != nil {
		err = rwordpress.SetStatus(status.StatusInfo{Status: *appStatus}, status.NoopStatusHistoryRecorder)
		c.Assert(err, jc.ErrorIsNil)
	}

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Destroy(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// If the remote app for the relation is terminated, any remote units are
	// forcibly removed from scope, but not local ones.
	s.assertInScope(c, wpru, appStatus == nil || *appStatus != status.Terminated)
	s.assertInScope(c, mysqlru, true)
}

func (s *RelationSuite) TestDestroyCrossModelRelationNoAppStatus(c *gc.C) {
	s.assertDestroyCrossModelRelation(c, nil)
}

func (s *RelationSuite) TestDestroyCrossModelRelationAppNotTerminated(c *gc.C) {
	st := status.Active
	s.assertDestroyCrossModelRelation(c, &st)
}

func (s *RelationSuite) TestDestroyCrossModelRelationAppTerminated(c *gc.C) {
	st := status.Terminated
	s.assertDestroyCrossModelRelation(c, &st)
}

func (s *RelationSuite) TestForceDestroyCrossModelRelationOfferSide(c *gc.C) {
	rwordpress, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:                   "remote-wordpress",
		ExternalControllerUUID: "controller-uuid",
		SourceModel:            names.NewModelTag("source-model"),
		OfferUUID:              "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
		IsConsumerProxy: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := rwordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlUnit, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	rel, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	mysqlru, err := rel.Unit(mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlru, true)

	wpru, err := rel.RemoteUnit("remote-wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = wpru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpru, true)

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	errs, err := rel.DestroyWithForce(true, 0)
	c.Assert(errs, gc.HasLen, 0)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	err = rwordpress.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	s.assertInScope(c, wpru, false)
	s.assertInScope(c, mysqlru, true)
}

func (s *RelationSuite) TestIsCrossModelYup(c *gc.C) {
	rwordpress, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "remote-wordpress",
		SourceModel:     names.NewModelTag("source-model"),
		IsConsumerProxy: true,
		OfferUUID:       "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := rwordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	app, result, err := relation.RemoteApplication()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.IsTrue)
	c.Assert(app.Name(), gc.Equals, "remote-wordpress")
}

func (s *RelationSuite) TestAddCrossModelNotAllowed(c *gc.C) {
	rwordpress, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-wordpress",
		SourceModel: names.NewModelTag("source-model"),
		OfferUUID:   "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := rwordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	err = rwordpress.SetStatus(status.StatusInfo{Status: status.Terminated}, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.ErrorMatches, `cannot add relation "remote-wordpress:db mysql:server": remote offer remote-wordpress is terminated`)

}

func (s *RelationSuite) TestIsCrossModelNope(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	app, result, err := relation.RemoteApplication()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.IsFalse)
	c.Assert(app, gc.IsNil)
}

func assertNoRelations(c *gc.C, app state.ApplicationEntity) {
	rels, err := app.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 0)
}

func assertOneRelation(c *gc.C, srv *state.Application, relId int, endpoints ...state.Endpoint) *state.Relation {
	rels, err := srv.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rels, gc.HasLen, 1)

	rel := rels[0]
	c.Assert(rel.Id(), gc.Equals, relId)

	c.Assert(rel.Endpoints(), jc.SameContents, endpoints)

	name := srv.Name()
	expectEp := endpoints[0]
	ep, err := rel.Endpoint(name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ep, gc.DeepEquals, expectEp)
	if len(endpoints) == 2 {
		expectEp = endpoints[1]
	}
	eps, err := rel.RelatedEndpoints(name)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(eps, gc.DeepEquals, []state.Endpoint{expectEp})
	return rel
}

func (s *RelationSuite) TestRemoveAlsoDeletesNetworks(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	relIngress := state.NewRelationIngressNetworks(s.State)
	_, err = relIngress.Save(relation.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = relIngress.Save(relation.Tag().Id(), true, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)

	relEgress := state.NewRelationEgressNetworks(s.State)
	_, err = relEgress.Save(relation.Tag().Id(), false, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = relEgress.Save(relation.Tag().Id(), true, []string{"1.2.3.4/32", "4.3.2.1/16"})
	c.Assert(err, jc.ErrorIsNil)

	state.RemoveRelation(c, relation, false)
	_, err = relIngress.Networks(relation.Tag().Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	_, err = relEgress.Networks(relation.Tag().Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RelationSuite) TestRemoveAlsoDeletesRemoteTokens(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// Add remote token so we can check it is cleaned up.
	re := s.State.RemoteEntities()
	relToken, err := re.ExportLocalEntity(relation.Tag())
	c.Assert(err, jc.ErrorIsNil)

	state.RemoveRelation(c, relation, false)
	_, err = re.GetToken(relation.Tag())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	_, err = re.GetRemoteEntity(relToken)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RelationSuite) TestRemoveAlsoDeletesRemoteOfferConnections(c *gc.C) {
	rwordpress, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "remote-wordpress",
		SourceModel:     names.NewModelTag("source-model"),
		IsConsumerProxy: true,
		OfferUUID:       "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	wordpressEP, err := rwordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// Add a offer connection record so we can check it is cleaned up.
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: coretesting.ModelTag.Id(),
		RelationId:      relation.Id(),
		RelationKey:     relation.Tag().Id(),
		Username:        "fred",
		OfferUUID:       "offer-uuid",
	})
	c.Assert(err, jc.ErrorIsNil)
	rc, err := s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), gc.Equals, 1)

	state.RemoveRelation(c, relation, false)
	rc, err = s.State.RemoteConnectionStatus("offer-uuid")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rc.TotalConnectionCount(), gc.Equals, 0)
}

func (s *RelationSuite) TestRemoveAlsoDeletesSecretPermissions(c *gc.C) {
	relation := s.Factory.MakeRelation(c, nil)
	app, err := s.State.Application(relation.Endpoints()[0].ApplicationName)
	c.Assert(err, jc.ErrorIsNil)
	store := state.NewSecrets(s.State)
	uri := secrets.NewURI()
	cp := state.CreateSecretParams{
		Version: 1,
		Owner:   app.Tag(),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &fakeToken{},
			Data:        map[string]string{"foo": "bar"},
		},
	}
	_, err = store.CreateSecret(uri, cp)
	c.Assert(err, jc.ErrorIsNil)

	subject := names.NewApplicationTag("wordpress")
	err = s.State.GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: &fakeToken{},
		Scope:       relation.Tag(),
		Subject:     subject,
		Role:        secrets.RoleView,
	})
	c.Assert(err, jc.ErrorIsNil)
	access, err := s.State.SecretAccess(uri, subject)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleView)

	state.RemoveRelation(c, relation, false)
	access, err = s.State.SecretAccess(uri, subject)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleNone)
}

func (s *RelationSuite) TestRemoveNoFeatureFlag(c *gc.C) {
	s.SetFeatureFlags( /*none*/ )
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	state.RemoveRelation(c, relation, false)
	_, err = s.State.KeyRelation(relation.Tag().Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RelationSuite) TestWatchLifeSuspendedStatus(c *gc.C) {
	rel := s.setupRelationStatus(c)
	mysql, err := s.State.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	u, err := mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	m := s.Factory.MakeMachine(c, &factory.MachineParams{})
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	relUnit, err := rel.Unit(u)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	w := rel.WatchLifeSuspendedStatus()
	defer workertest.CleanKill(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	// Initial event.
	wc.AssertChange(rel.Tag().Id())
	wc.AssertNoChange()

	err = rel.SetSuspended(true, "reason")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(rel.Tag().Id())
	wc.AssertNoChange()

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = rel.Destroy(nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(rel.Tag().Id())
	wc.AssertNoChange()
}

func (s *RelationSuite) TestWatchLifeSuspendedStatusDead(c *gc.C) {
	// Create a pair of application and a relation between them.
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	w := rel.WatchLifeSuspendedStatus()
	defer workertest.CleanKill(c, w)
	wc := testing.NewStringsWatcherC(c, w)
	wc.AssertChange(rel.Tag().Id())

	err = rel.Destroy(nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(rel.Tag().Id())
	wc.AssertNoChange()
}

func (s *RelationSuite) setupRelationStatus(c *gc.C) *state.Relation {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
	relStatus, err := rel.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relStatus.Status, gc.Equals, status.Joining)
	ao := state.NewApplicationOffers(s.State)
	offer, err := ao.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       "hosted-mysql",
		ApplicationName: "mysql",
		Owner:           s.Owner.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "fred", Access: permission.WriteAccess})
	_, err = s.State.AddOfferConnection(state.AddOfferConnectionParams{
		SourceModelUUID: uuid.MustNewUUID().String(),
		OfferUUID:       offer.OfferUUID,
		RelationKey:     rel.Tag().Id(),
		RelationId:      rel.Id(),
		Username:        user.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return rel
}

func (s *RelationSuite) TestStatus(c *gc.C) {
	rel := s.setupRelationStatus(c)
	err := rel.SetStatus(status.StatusInfo{
		Status:  status.Suspended,
		Message: "for a while",
	}, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	relStatus, err := rel.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relStatus.Since, gc.NotNil)
	relStatus.Since = nil
	c.Assert(relStatus, jc.DeepEquals, status.StatusInfo{
		Status:  status.Suspended,
		Message: "for a while",
		Data:    map[string]interface{}{},
	})
}

func (s *RelationSuite) TestInvalidStatus(c *gc.C) {
	rel := s.setupRelationStatus(c)

	err := rel.SetStatus(status.StatusInfo{
		Status: status.Status("invalid"),
	}, status.NoopStatusHistoryRecorder)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "invalid"`)
}

func (s *RelationSuite) TestSetSuspend(c *gc.C) {
	rel := s.setupRelationStatus(c)
	// Suspend doesn't need an offer connection to be there.
	state.RemoveOfferConnectionsForRelation(c, rel)
	c.Assert(rel.Suspended(), jc.IsFalse)
	err := rel.SetSuspended(true, "reason")
	c.Assert(err, jc.ErrorIsNil)
	rel, err = s.State.Relation(rel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Suspended(), jc.IsTrue)
	c.Assert(rel.SuspendedReason(), gc.Equals, "reason")
}

func (s *RelationSuite) TestSetSuspendFalse(c *gc.C) {
	rel := s.setupRelationStatus(c)
	// Suspend doesn't need an offer connection to be there.
	state.RemoveOfferConnectionsForRelation(c, rel)
	c.Assert(rel.Suspended(), jc.IsFalse)
	err := rel.SetSuspended(true, "reason")
	c.Assert(err, jc.ErrorIsNil)
	err = rel.SetSuspended(false, "reason")
	c.Assert(err, gc.ErrorMatches, "cannot set suspended reason if not suspended")
	err = rel.SetSuspended(false, "")
	c.Assert(err, jc.ErrorIsNil)
	rel, err = s.State.Relation(rel.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Suspended(), jc.IsFalse)
}

func (s *RelationSuite) TestApplicationSettings(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	settingsMap, err := relation.ApplicationSettings("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settingsMap, gc.HasLen, 0)

	settings := state.NewStateSettings(s.State)
	key := fmt.Sprintf("r#%d#mysql", relation.Id())
	err = settings.ReplaceSettings(key, map[string]interface{}{
		"bailterspace": "blammo",
	})
	c.Assert(err, jc.ErrorIsNil)

	settingsMap, err = relation.ApplicationSettings("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settingsMap, gc.DeepEquals, map[string]interface{}{
		"bailterspace": "blammo",
	})
}

func (s *RelationSuite) TestApplicationSettingsPeer(c *gc.C) {
	app := state.AddTestingApplication(c, s.State, s.objectStore, "riak", state.AddTestingCharm(c, s.State, "riak"))
	ep, err := app.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)

	settings := state.NewStateSettings(s.State)
	key := fmt.Sprintf("r#%d#riak", rel.Id())
	err = settings.ReplaceSettings(key, map[string]interface{}{
		"mermaidens": "disappear",
	})
	c.Assert(err, jc.ErrorIsNil)

	settingsMap, err := rel.ApplicationSettings("riak")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settingsMap, gc.DeepEquals, map[string]interface{}{
		"mermaidens": "disappear",
	})
}

func (s *RelationSuite) TestApplicationSettingsErrors(c *gc.C) {
	app := state.AddTestingApplication(c, s.State, s.objectStore, "riak", state.AddTestingCharm(c, s.State, "riak"))
	ep, err := app.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)

	state.AddTestingApplication(c, s.State, s.objectStore, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))

	settings, err := rel.ApplicationSettings("wordpress")
	c.Assert(err, gc.ErrorMatches, `application "wordpress" is not a member of "riak:ring"`)
	c.Assert(settings, gc.HasLen, 0)
}

func (s *RelationSuite) TestUpdateApplicationSettingsSuccess(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// fakeToken always succeeds.
	err = relation.UpdateApplicationSettings(
		"mysql", &fakeToken{}, map[string]interface{}{
			"rendezvouse": "rendezvous",
			"olden":       "yolk",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	settingsMap, err := relation.ApplicationSettings("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settingsMap, gc.DeepEquals, map[string]interface{}{
		"rendezvouse": "rendezvous",
		"olden":       "yolk",
	})

	// Check that updates only overwrite existing keys.
	err = relation.UpdateApplicationSettings(
		"mysql", &fakeToken{}, map[string]interface{}{
			"olden": "times",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	settingsMap, err = relation.ApplicationSettings("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settingsMap, gc.DeepEquals, map[string]interface{}{
		"rendezvouse": "rendezvous",
		"olden":       "times",
	})
}

func (s *RelationSuite) TestUpdateApplicationSettingsNotLeader(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	err = relation.UpdateApplicationSettings(
		"mysql",
		&fakeToken{errors.New("not the leader")},
		map[string]interface{}{
			"rendezvouse": "rendezvous",
		},
	)
	c.Assert(err, gc.ErrorMatches,
		`relation "wordpress:db mysql:server" application "mysql": checking leadership continuity: not the leader`)

	settingsMap, err := relation.ApplicationSettings("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settingsMap, gc.HasLen, 0)
}

func (s *RelationSuite) TestWatchApplicationSettings(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w, err := relation.WatchApplicationSettings(mysql)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	err = relation.UpdateApplicationSettings(
		"mysql", &fakeToken{}, map[string]interface{}{
			"castor": "pollux",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// No notify for a null change.
	err = relation.UpdateApplicationSettings(
		"mysql", &fakeToken{}, map[string]interface{}{
			"castor": "pollux",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *RelationSuite) TestWatchApplicationSettingsOtherEnd(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.Model.UUID())

	w, err := relation.WatchApplicationSettings(mysql)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := testing.NewNotifyWatcherC(c, w)
	wc.AssertOneChange()

	// No notify if the other application's settings are changed.
	err = relation.UpdateApplicationSettings(
		"wordpress", &fakeToken{}, map[string]interface{}{
			"grand": "palais",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *RelationSuite) TestDestroyForceSchedulesCleanupForStuckUnits(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	// If a unit agent is gone or down for some reason, a unit might
	// not leave the relation scope when the relation goes to
	// dying. If the destroy is forced, we shouldn't wait indefinitely
	// for that unit to leave scope.
	addRelationUnit := func(c *gc.C, app *state.Application) *state.RelationUnit {
		unit, err := app.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		machine := s.Factory.MakeMachine(c, &factory.MachineParams{})
		err = unit.AssignToMachine(machine)
		c.Assert(err, jc.ErrorIsNil)
		relUnit, err := rel.Unit(unit)
		c.Assert(err, jc.ErrorIsNil)
		err = relUnit.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)
		return relUnit
	}

	relUnits := []*state.RelationUnit{
		addRelationUnit(c, wordpress),
		addRelationUnit(c, wordpress),
		addRelationUnit(c, mysql),
	}
	// Destroy one of the units to be sure the cleanup isn't
	// retrieving it.
	unit, err := s.State.Unit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = unit.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	s.assertRelationCleanedUp(c, rel, relUnits)
}

func (s *RelationSuite) TestDestroyForceStuckSubordinateUnits(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeContainer)
	prr.allEnterScope(c)

	rel := prr.rel
	relUnits := []*state.RelationUnit{
		prr.pru0, prr.pru1, prr.rru0, prr.rru1,
	}
	s.assertRelationCleanedUp(c, rel, relUnits)
}

func (s *RelationSuite) TestDestroyForceStuckRemoteUnits(c *gc.C) {
	mysqlEps := []charm.Relation{
		{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		},
	}
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: s.Model.ModelTag(),
		Token:       "t0",
		Endpoints:   mysqlEps,
	})
	c.Assert(err, jc.ErrorIsNil)

	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{})
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
	localRelUnit, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = localRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	remoteRelUnit, err := rel.RemoteUnit("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	err = remoteRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	opErrs, err := rel.DestroyWithForce(true, time.Minute)
	c.Assert(opErrs, gc.HasLen, 0)
	c.Assert(err, jc.ErrorIsNil)

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// Schedules a cleanup to remove the unit scope if needed.
	s.assertNeedsCleanup(c)

	// But running cleanup immediately doesn't do it all.
	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)

	// Remote units forced out of scope.
	assertInScope(c, localRelUnit)
	assertNotInScope(c, remoteRelUnit)

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	s.Clock.Advance(time.Minute)

	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)

	assertNotInScope(c, localRelUnit)

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RelationSuite) TestDestroyForceIsFineIfUnitsAlreadyLeft(c *gc.C) {
	prr := newProReqRelation(c, &s.ConnSuite, charm.ScopeGlobal)
	prr.allEnterScope(c)
	rel := prr.rel
	relUnits := []*state.RelationUnit{
		prr.pru0, prr.pru1, prr.rru0, prr.rru1,
	}
	opErrs, err := rel.DestroyWithForce(true, time.Minute)
	c.Assert(opErrs, gc.HasLen, 0)
	c.Assert(err, jc.ErrorIsNil)

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// Schedules a cleanup to remove the unit scope if needed.
	s.assertNeedsCleanup(c)

	// But running cleanup immediately doesn't do it.
	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)
	for i, ru := range relUnits {
		c.Logf("%d", i)
		assertJoined(c, ru)
	}
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// In the meantime the units correctly leave scope.
	s.Clock.Advance(30 * time.Second)

	for i, ru := range relUnits {
		c.Logf("%d", i)
		err := ru.LeaveScope()
		c.Assert(err, jc.ErrorIsNil)
	}
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	s.Clock.Advance(30 * time.Second)

	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)

	// If the cleanup had failed because the relation had gone, it
	// would be left in the collection.
	s.assertNoCleanups(c)
}

func (s *RelationSuite) assertRelationCleanedUp(c *gc.C, rel *state.Relation, relUnits []*state.RelationUnit) {
	opErrs, err := rel.DestroyWithForce(true, time.Minute)
	c.Assert(opErrs, gc.HasLen, 0)
	c.Assert(err, jc.ErrorIsNil)

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	// Schedules a cleanup to remove the unit scope if needed.
	s.assertNeedsCleanup(c)

	// But running cleanup immediately doesn't do it.
	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)
	s.assertNeedsCleanup(c)
	for i, ru := range relUnits {
		c.Logf("%d", i)
		assertJoined(c, ru)
	}
	err = rel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Dying)

	s.Clock.Advance(time.Minute)

	err = s.State.Cleanup(context.Background(), state.NewObjectStore(c, s.State.ModelUUID()), fakeMachineRemover{}, fakeAppRemover{}, fakeUnitRemover{})
	c.Assert(err, jc.ErrorIsNil)

	for i, ru := range relUnits {
		c.Logf("%d", i)
		assertNotInScope(c, ru)
	}

	err = rel.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *RelationSuite) assertNeedsCleanup(c *gc.C) {
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)
}

func (s *RelationSuite) assertNoCleanups(c *gc.C) {
	dirty, err := s.State.NeedsCleanup()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)
}
