// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/charms"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type charmsSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite
	api *charms.API
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	auth := testing.FakeAuthorizer{
		Tag:            s.AdminUserTag(c),
		EnvironManager: true,
	}
	s.api, err = charms.NewAPI(s.State, common.NewResources(), auth)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmsSuite) TestClientCharmInfo(c *gc.C) {
	var clientCharmInfoTests = []struct {
		about    string
		charm    string
		url      string
		expected params.CharmInfo
		err      string
	}{
		{
			about: "dummy charm which contains an expectedActions spec",
			charm: "dummy",
			url:   "local:quantal/dummy-1",
			expected: params.CharmInfo{
				Revision: 1,
				URL:      "local:quantal/dummy-1",
				Config: map[string]params.CharmOption{
					"skill-level": params.CharmOption{
						Type:        "int",
						Description: "A number indicating skill."},
					"title": params.CharmOption{
						Type:        "string",
						Description: "A descriptive title used for the application.",
						Default:     "My Title"},
					"outlook": params.CharmOption{
						Type:        "string",
						Description: "No default outlook."},
					"username": params.CharmOption{
						Type:        "string",
						Description: "The name of the initial account (given admin permissions).",
						Default:     "admin001"},
				},
				Meta: &params.CharmMeta{
					Name:           "dummy",
					Summary:        "That's a dummy charm.",
					Description:    "This is a longer description which\npotentially contains multiple lines.\n",
					Subordinate:    false,
					MinJujuVersion: "0.0.0",
				},
				Actions: &params.CharmActions{
					ActionSpecs: map[string]params.CharmActionSpec{
						"snapshot": params.CharmActionSpec{
							Description: "Take a snapshot of the database.",
							Params: map[string]interface{}{
								"title":       "snapshot",
								"description": "Take a snapshot of the database.",
								"type":        "object",
								"properties": map[string]interface{}{
									"outfile": map[string]interface{}{
										"type":        "string",
										"description": "The file to write out to.",
										"default":     "foo.bz2",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			about: "retrieves charm info",
			// Use wordpress for tests so that we can compare Provides and Requires.
			charm: "wordpress",
			url:   "local:quantal/wordpress-3",
			expected: params.CharmInfo{
				Revision: 3,
				URL:      "local:quantal/wordpress-3",
				Config: map[string]params.CharmOption{
					"blog-title": params.CharmOption{Type: "string", Description: "A descriptive title used for the blog.", Default: "My Title"}},
				Meta: &params.CharmMeta{
					Name:        "wordpress",
					Summary:     "Blog engine",
					Description: "A pretty popular blog engine",
					Subordinate: false,
					Provides: map[string]params.CharmRelation{
						"logging-dir": params.CharmRelation{
							Name:      "logging-dir",
							Role:      "provider",
							Interface: "logging",
							Scope:     "container",
						},
						"monitoring-port": params.CharmRelation{
							Name:      "monitoring-port",
							Role:      "provider",
							Interface: "monitoring",
							Scope:     "container",
						},
						"url": params.CharmRelation{
							Name:      "url",
							Role:      "provider",
							Interface: "http",
							Scope:     "global",
						},
					},
					Requires: map[string]params.CharmRelation{
						"cache": params.CharmRelation{
							Name:      "cache",
							Role:      "requirer",
							Interface: "varnish",
							Optional:  true,
							Limit:     2,
							Scope:     "global",
						},
						"db": params.CharmRelation{
							Name:      "db",
							Role:      "requirer",
							Interface: "mysql",
							Limit:     1,
							Scope:     "global",
						},
					},
					ExtraBindings: map[string]string{
						"admin-api": "admin-api",
						"foo-bar":   "foo-bar",
						"db-client": "db-client",
					},
					MinJujuVersion: "0.0.0",
				},
				Actions: &params.CharmActions{
					ActionSpecs: map[string]params.CharmActionSpec{
						"fakeaction": params.CharmActionSpec{
							Description: "No description",
							Params: map[string]interface{}{
								"properties":  map[string]interface{}{},
								"description": "No description",
								"type":        "object",
								"title":       "fakeaction"},
						},
					},
				},
			},
		},
		{
			about: "invalid URL",
			charm: "wordpress",
			url:   "not-valid!",
			err:   `URL has invalid charm or bundle name: "not-valid!"`,
		},
		{
			about: "invalid schema",
			charm: "wordpress",
			url:   "not-valid:your-arguments",
			err:   `charm or bundle URL has invalid schema: "not-valid:your-arguments"`,
		},
		{
			about: "unknown charm",
			charm: "wordpress",
			url:   "cs:missing/one-1",
			err:   `charm "cs:missing/one-1" not found`,
		},
	}

	for i, t := range clientCharmInfoTests {
		c.Logf("test %d. %s", i, t.about)
		s.AddTestingCharm(c, t.charm)
		info, err := s.api.CharmInfo(params.CharmURL{t.url})
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Check(info, jc.DeepEquals, t.expected)
	}
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
			"pings": params.CharmMetric{
				Type:        "gauge",
				Description: "Description of the metric."},
			"juju-units": params.CharmMetric{
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
