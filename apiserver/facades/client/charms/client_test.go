// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type charmsSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite
	api  *charms.API
	auth facade.Authorizer
}

var _ = gc.Suite(&charmsSuite{})

// charmsSuiteContext implements the facade.Context interface.
type charmsSuiteContext struct{ cs *charmsSuite }

func (ctx *charmsSuiteContext) Abort() <-chan struct{}                        { return nil }
func (ctx *charmsSuiteContext) Auth() facade.Authorizer                       { return ctx.cs.auth }
func (ctx *charmsSuiteContext) Cancel() <-chan struct{}                       { return nil }
func (ctx *charmsSuiteContext) Dispose()                                      {}
func (ctx *charmsSuiteContext) Resources() facade.Resources                   { return common.NewResources() }
func (ctx *charmsSuiteContext) State() *state.State                           { return ctx.cs.State }
func (ctx *charmsSuiteContext) StatePool() *state.StatePool                   { return nil }
func (ctx *charmsSuiteContext) ID() string                                    { return "" }
func (ctx *charmsSuiteContext) Presence() facade.Presence                     { return nil }
func (ctx *charmsSuiteContext) Hub() facade.Hub                               { return nil }
func (ctx *charmsSuiteContext) Controller() *cache.Controller                 { return nil }
func (ctx *charmsSuiteContext) CachedModel(uuid string) (*cache.Model, error) { return nil, nil }
func (ctx *charmsSuiteContext) MultiwatcherFactory() multiwatcher.Factory     { return nil }

func (ctx *charmsSuiteContext) LeadershipClaimer(string) (leadership.Claimer, error) { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipRevoker(string) (leadership.Revoker, error) { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipChecker() (leadership.Checker, error)       { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipPinner(string) (leadership.Pinner, error)   { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipReader(string) (leadership.Reader, error)   { return nil, nil }
func (ctx *charmsSuiteContext) SingularClaimer() (lease.Claimer, error)              { return nil, nil }

func (s *charmsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.auth = testing.FakeAuthorizer{
		Tag:        s.AdminUserTag(c),
		Controller: true,
	}

	var err error
	s.api, err = charms.NewFacade(&charmsSuiteContext{cs: s})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmsSuite) TestMeteredCharmInfo(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(
		c, &factory.CharmParams{Name: "metered", URL: "cs:xenial/metered"})
	info, err := s.api.CharmInfo(params.CharmURL{
		URL: meteredCharm.URL().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := &params.CharmMetrics{
		Plan: params.CharmPlan{
			Required: true,
		},
		Metrics: map[string]params.CharmMetric{
			"pings": {
				Type:        "gauge",
				Description: "Description of the metric."},
			"pongs": {
				Type:        "gauge",
				Description: "Description of the metric."},
			"juju-units": {
				Type:        "",
				Description: ""}}}
	c.Assert(info.Metrics, jc.DeepEquals, expected)
}

func (s *charmsSuite) TestListCharmsNoFilter(c *gc.C) {
	s.assertListCharms(c, []string{"dummy"}, []string{}, []string{"local:quantal/dummy-1"})
}

func (s *charmsSuite) TestListCharmsWithFilterMatchingNone(c *gc.C) {
	s.assertListCharms(c, []string{"dummy"}, []string{"notdummy"}, []string{})
}

func (s *charmsSuite) TestListCharmsFilteredOnly(c *gc.C) {
	s.assertListCharms(c, []string{"dummy", "wordpress"}, []string{"dummy"}, []string{"local:quantal/dummy-1"})
}

func (s *charmsSuite) assertListCharms(c *gc.C, someCharms, args, expected []string) {
	for _, aCharm := range someCharms {
		s.AddTestingCharm(c, aCharm)
	}
	found, err := s.api.List(params.CharmsList{Names: args})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.CharmURLs, gc.HasLen, len(expected))
	c.Check(found.CharmURLs, jc.DeepEquals, expected)
}

func (s *charmsSuite) TestIsMeteredFalse(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	metered, err := s.api.IsMetered(params.CharmURL{
		URL: charm.URL().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metered.Metered, jc.IsFalse)
}

func (s *charmsSuite) TestIsMeteredTrue(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	metered, err := s.api.IsMetered(params.CharmURL{
		URL: meteredCharm.URL().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metered.Metered, jc.IsTrue)
}
