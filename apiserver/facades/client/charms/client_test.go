// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"os"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
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

func (ctx *charmsSuiteContext) Abort() <-chan struct{}      { return nil }
func (ctx *charmsSuiteContext) Auth() facade.Authorizer     { return ctx.cs.auth }
func (ctx *charmsSuiteContext) Dispose()                    {}
func (ctx *charmsSuiteContext) Resources() facade.Resources { return common.NewResources() }
func (ctx *charmsSuiteContext) State() *state.State         { return ctx.cs.State }
func (ctx *charmsSuiteContext) StatePool() *state.StatePool { return nil }
func (ctx *charmsSuiteContext) ID() string                  { return "" }
func (ctx *charmsSuiteContext) Presence() facade.Presence   { return nil }
func (ctx *charmsSuiteContext) Hub() facade.Hub             { return nil }

func (ctx *charmsSuiteContext) LeadershipClaimer(string) (leadership.Claimer, error) { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipChecker() (leadership.Checker, error)       { return nil, nil }
func (ctx *charmsSuiteContext) LeadershipPinner(string) (leadership.Pinner, error)   { return nil, nil }
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

func (s *charmsSuite) TestClientCharmInfo(c *gc.C) {
	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.LXDProfile)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

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
					"skill-level": {
						Type:        "int",
						Description: "A number indicating skill."},
					"title": {
						Type:        "string",
						Description: "A descriptive title used for the application.",
						Default:     "My Title"},
					"outlook": {
						Type:        "string",
						Description: "No default outlook."},
					"username": {
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
						"snapshot": {
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
			about: "dummy charm which contains lxd profile spec",
			charm: "lxd-profile",
			url:   "local:quantal/lxd-profile-0",
			expected: params.CharmInfo{
				Revision: 0,
				URL:      "local:quantal/lxd-profile-0",
				Config:   map[string]params.CharmOption{},
				Meta: &params.CharmMeta{
					Name:           "lxd-profile",
					Summary:        "start a juju machine with a lxd profile",
					Description:    "Run an Ubuntu system, with the given lxd-profile\n",
					Subordinate:    false,
					MinJujuVersion: "0.0.0",
					Provides: map[string]params.CharmRelation{
						"ubuntu": {
							Name:      "ubuntu",
							Interface: "ubuntu",
							Role:      "provider",
							Scope:     "global",
						},
					},
					ExtraBindings: map[string]string{
						"another": "another",
					},
					Tags: []string{
						"misc",
						"application_development",
					},
					Series: []string{
						"bionic",
						"xenial",
						"quantal",
					},
				},
				Actions: &params.CharmActions{},
				LXDProfile: &params.CharmLXDProfile{
					Description: "lxd profile for testing, black list items grouped commented out",
					Config: map[string]string{
						"security.nesting":       "true",
						"security.privileged":    "true",
						"linux.kernel_modules":   "openvswitch,nbd,ip_tables,ip6_tables",
						"environment.http_proxy": "",
					},
					Devices: map[string]map[string]string{
						"tun": {
							"path": "/dev/net/tun",
							"type": "unix-char",
						},
						"sony": {
							"type":      "usb",
							"vendorid":  "0fce",
							"productid": "51da",
						},
						"bdisk": {
							"type":   "unix-block",
							"source": "/dev/loop0",
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
					"blog-title": {Type: "string", Description: "A descriptive title used for the blog.", Default: "My Title"}},
				Meta: &params.CharmMeta{
					Name:        "wordpress",
					Summary:     "Blog engine",
					Description: "A pretty popular blog engine",
					Subordinate: false,
					Provides: map[string]params.CharmRelation{
						"logging-dir": {
							Name:      "logging-dir",
							Role:      "provider",
							Interface: "logging",
							Scope:     "container",
						},
						"monitoring-port": {
							Name:      "monitoring-port",
							Role:      "provider",
							Interface: "monitoring",
							Scope:     "container",
						},
						"url": {
							Name:      "url",
							Role:      "provider",
							Interface: "http",
							Scope:     "global",
						},
					},
					Requires: map[string]params.CharmRelation{
						"cache": {
							Name:      "cache",
							Role:      "requirer",
							Interface: "varnish",
							Optional:  true,
							Limit:     2,
							Scope:     "global",
						},
						"db": {
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
						"fakeaction": {
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
			err:   `cannot parse URL "not-valid!": name "not-valid!" not valid`,
		},
		{
			about: "invalid schema",
			charm: "wordpress",
			url:   "not-valid:your-arguments",
			err:   `cannot parse URL "not-valid:your-arguments": schema "not-valid" not valid`,
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
		info, err := s.api.CharmInfo(params.CharmURL{URL: t.url})
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		}
		if c.Check(err, jc.ErrorIsNil) == false {
			continue
		}
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
