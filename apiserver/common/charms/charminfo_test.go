// Copyright 2012-2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"fmt"

	"github.com/juju/charm/v8"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
	jtesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type charmInfoSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite
	api  *charms.CharmInfoAPI
	auth facade.Authorizer
}

var _ = gc.Suite(&charmInfoSuite{})

func (s *charmInfoSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.auth = testing.FakeAuthorizer{
		Tag:        s.AdminUserTag(c),
		Controller: true,
	}

	var err error
	s.api, err = charms.NewCharmInfoAPI(&charms.StateShim{s.State}, s.auth)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmInfoSuite) TestClientCharmInfoCAAS(c *gc.C) {
	var clientCharmInfoTests = []struct {
		about    string
		series   string
		charm    string
		url      string
		expected params.Charm
		err      string
	}{
		{
			about:  "charm info for meta format v2 with containers on a CAAS model",
			series: "focal",
			// Use cockroach for tests so that we can compare Provides and Requires.
			charm: "cockroach",
			url:   "local:focal/cockroachdb-0",
			expected: params.Charm{
				Revision: 0,
				URL:      "local:focal/cockroachdb-0",
				Config:   map[string]params.CharmOption{},
				Manifest: &params.CharmManifest{
					Bases: []params.CharmBase{
						{
							Name:          "ubuntu",
							Channel:       "20.04/stable",
							Architectures: []string{"amd64"},
						},
					},
				},
				Meta: &params.CharmMeta{
					Name:        "cockroachdb",
					Summary:     "cockroachdb",
					Description: "cockroachdb",
					Storage: map[string]params.CharmStorage{
						"database": {
							Name:     "database",
							Type:     "filesystem",
							CountMin: 1,
							CountMax: 1,
						},
					},
					Containers: map[string]params.CharmContainer{
						"cockroachdb": {
							Resource: "cockroachdb-image",
							Mounts: []params.CharmMount{
								{
									Storage:  "database",
									Location: "/cockroach/cockroach-data",
								},
							},
						},
					},
					Provides: map[string]params.CharmRelation{
						"db": {
							Name:      "db",
							Role:      "provider",
							Interface: "roach",
							Scope:     "global",
						},
					},
					Resources: map[string]params.CharmResourceMeta{
						"cockroachdb-image": {
							Name:        "cockroachdb-image",
							Type:        "oci-image",
							Description: "OCI image used for cockroachdb",
						},
					},
					MinJujuVersion: "0.0.0",
				},
				Actions: &params.CharmActions{},
			},
		},
	}

	for i, t := range clientCharmInfoTests {
		c.Logf("test %d. %s", i, t.about)

		otherModelOwner := s.Factory.MakeModelUser(c, nil)
		otherSt := s.Factory.MakeCAASModel(c, &factory.ModelParams{
			Owner: otherModelOwner.UserTag,
			ConfigAttrs: jtesting.Attrs{
				"controller": false,
			},
		})
		defer otherSt.Close()

		otherModel, err := otherSt.Model()
		c.Assert(err, jc.ErrorIsNil)

		repo := testcharms.RepoForSeries(t.series)
		ch := repo.CharmDir(t.charm)
		ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
		curl := charm.MustParseURL(fmt.Sprintf("local:%s/%s", t.series, ident))

		_, err = jujutesting.AddCharm(otherModel.State(), curl, ch, false)

		c.Assert(err, jc.ErrorIsNil)

		s.api, err = charms.NewCharmInfoAPI(&charms.StateShim{otherModel.State()}, s.auth)
		c.Assert(err, jc.ErrorIsNil)

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

func (s *charmInfoSuite) TestClientCharmInfo(c *gc.C) {
	var clientCharmInfoTests = []struct {
		about    string
		charm    string
		series   string
		url      string
		expected params.Charm
		err      string
	}{
		{
			about: "dummy charm which contains an expectedActions spec",
			charm: "dummy",
			url:   "local:quantal/dummy-1",
			expected: params.Charm{
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
			expected: params.Charm{
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
					Description: "lxd profile for testing, will pass validation",
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
							"source": "/dev/loop0",
							"type":   "unix-block",
						},
						"gpu": {
							"type": "gpu",
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
			expected: params.Charm{
				Revision: 3,
				URL:      "local:quantal/wordpress-3",
				Config: map[string]params.CharmOption{
					"blog-title": {
						Type:        "string",
						Description: "A descriptive title used for the blog.",
						Default:     "My Title",
					},
				},
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
			about: "charm info for meta format v2 without containers on a IAAS model",
			// Use cockroach for tests so that we can compare Provides and Requires.
			charm:  "cockroach-container-less",
			series: "focal",
			url:    "local:focal/cockroachdb-0",
			expected: params.Charm{
				Revision: 0,
				URL:      "local:focal/cockroachdb-0",
				Config:   map[string]params.CharmOption{},
				Manifest: &params.CharmManifest{
					Bases: []params.CharmBase{
						{
							Name:          "ubuntu",
							Channel:       "20.04/stable",
							Architectures: []string{"amd64"},
						},
					},
				},
				Meta: &params.CharmMeta{
					Name:        "cockroachdb",
					Summary:     "cockroachdb",
					Description: "cockroachdb",
					Storage: map[string]params.CharmStorage{
						"database": {
							Name:     "database",
							Type:     "filesystem",
							CountMin: 1,
							CountMax: 1,
						},
					},
					Provides: map[string]params.CharmRelation{
						"db": {
							Name:      "db",
							Role:      "provider",
							Interface: "roach",
							Scope:     "global",
						},
					},
					Resources: map[string]params.CharmResourceMeta{
						"cockroachdb-image": {
							Name:        "cockroachdb-image",
							Type:        "oci-image",
							Description: "OCI image used for cockroachdb",
						},
					},
					MinJujuVersion: "0.0.0",
				},
				Actions: &params.CharmActions{},
			},
		},
		{
			about: "invalid URL",
			charm: "wordpress",
			url:   "not-valid!",
			err:   `cannot parse name and/or revision in URL "not-valid!": name "not-valid!" not valid`,
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
		if t.series != "" {
			s.AddTestingCharmForSeries(c, t.charm, t.series)
		} else {
			s.AddTestingCharm(c, t.charm)
		}
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
