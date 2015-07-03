// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/charms"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type baseCharmsSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite
}

type charmsSuite struct {
	baseCharmsSuite
	api *charms.API
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) SetUpTest(c *gc.C) {
	s.baseCharmsSuite.SetUpTest(c)

	var err error
	auth := testing.FakeAuthorizer{
		Tag:            s.AdminUserTag(c),
		EnvironManager: true,
	}
	s.api, err = charms.NewAPI(s.State, common.NewResources(), auth)
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&baseCharmsSuite{})

func (s *baseCharmsSuite) TestClientCharmInfo(c *gc.C) {
	var clientCharmInfoTests = []struct {
		about           string
		charm           string
		url             string
		expectedActions *charm.Actions
		err             string
	}{
		{
			about: "dummy charm which contains an expectedActions spec",
			charm: "dummy",
			url:   "local:quantal/dummy-1",
			expectedActions: &charm.Actions{
				ActionSpecs: map[string]charm.ActionSpec{
					"snapshot": {
						Description: "Take a snapshot of the database.",
						Params: map[string]interface{}{
							"type":        "object",
							"title":       "snapshot",
							"description": "Take a snapshot of the database.",
							"properties": map[string]interface{}{
								"outfile": map[string]interface{}{
									"default":     "foo.bz2",
									"description": "The file to write out to.",
									"type":        "string",
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
			expectedActions: &charm.Actions{ActionSpecs: map[string]charm.ActionSpec{
				"fakeaction": {
					Description: "No description",
					Params: map[string]interface{}{
						"type":        "object",
						"title":       "fakeaction",
						"description": "No description",
						"properties":  map[string]interface{}{},
					},
				},
			}},
			url: "local:quantal/wordpress-3",
		},
		{
			about:           "invalid URL",
			charm:           "wordpress",
			expectedActions: &charm.Actions{ActionSpecs: nil},
			url:             "not-valid",
			err:             "charm url series is not resolved",
		},
		{
			about:           "invalid schema",
			charm:           "wordpress",
			expectedActions: &charm.Actions{ActionSpecs: nil},
			url:             "not-valid:your-arguments",
			err:             `charm URL has invalid schema: "not-valid:your-arguments"`,
		},
		{
			about:           "unknown charm",
			charm:           "wordpress",
			expectedActions: &charm.Actions{ActionSpecs: nil},
			url:             "cs:missing/one-1",
			err:             `charm "cs:missing/one-1" not found`,
		},
	}

	for i, t := range clientCharmInfoTests {
		c.Logf("test %d. %s", i, t.about)
		aCharm := s.AddTestingCharm(c, t.charm)
		info, err := s.APIState.Client().CharmInfo(t.url)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		expected := &api.CharmInfo{
			Revision: aCharm.Revision(),
			URL:      aCharm.URL().String(),
			Config:   aCharm.Config(),
			Meta:     aCharm.Meta(),
			Actions:  aCharm.Actions(),
		}
		c.Check(info, jc.DeepEquals, expected)
		c.Check(info.Actions, jc.DeepEquals, t.expectedActions)
	}
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
	metered, err := s.api.IsMetered(params.CharmInfo{
		CharmURL: charm.URL().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metered.Metered, jc.IsFalse)
}

func (s *charmsSuite) TestIsMeteredTrue(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	metered, err := s.api.IsMetered(params.CharmInfo{
		CharmURL: meteredCharm.URL().String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(metered.Metered, jc.IsTrue)
}
