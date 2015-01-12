// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type CharmSuite struct {
	ConnSuite
	curl *charm.URL
}

var _ = gc.Suite(&CharmSuite{})

func (s *CharmSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	added := s.AddTestingCharm(c, "dummy")
	s.curl = added.URL()
}

func (s *CharmSuite) TestCharm(c *gc.C) {
	dummy, err := s.State.Charm(s.curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummy.URL().String(), gc.Equals, s.curl.String())
	c.Assert(dummy.Revision(), gc.Equals, 1)
	c.Assert(dummy.StoragePath(), gc.Equals, "dummy-path")
	c.Assert(dummy.BundleSha256(), gc.Equals, "quantal-dummy-1-sha256")
	c.Assert(dummy.IsUploaded(), jc.IsTrue)
	meta := dummy.Meta()
	c.Assert(meta.Name, gc.Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], gc.Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
	actions := dummy.Actions()
	c.Assert(actions, gc.NotNil)
	c.Assert(actions.ActionSpecs, gc.Not(gc.HasLen), 0)
	c.Assert(actions.ActionSpecs["snapshot"], gc.NotNil)
	c.Assert(actions.ActionSpecs["snapshot"].Params, gc.Not(gc.HasLen), 0)
	c.Assert(actions.ActionSpecs["snapshot"], gc.DeepEquals,
		charm.ActionSpec{
			Description: "Take a snapshot of the database.",
			Params: map[string]interface{}{
				"type":        "object",
				"title":       "snapshot",
				"description": "Take a snapshot of the database.",
				"properties": map[string]interface{}{
					"outfile": map[string]interface{}{
						"description": "The file to write out to.",
						"type":        "string",
						"default":     "foo.bz2",
					},
				},
			},
		})
}

func (s *CharmSuite) TestCharmNotFound(c *gc.C) {
	curl := charm.MustParseURL("local:anotherseries/dummy-1")
	_, err := s.State.Charm(curl)
	c.Assert(err, gc.ErrorMatches, `charm "local:anotherseries/dummy-1" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

type CharmTestHelperSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CharmTestHelperSuite{})

func assertCustomCharm(c *gc.C, ch *state.Charm, series string, meta *charm.Meta, config *charm.Config, metrics *charm.Metrics, revision int) {
	// Check Charm interface method results.
	c.Assert(ch.Meta(), gc.DeepEquals, meta)
	c.Assert(ch.Config(), gc.DeepEquals, config)
	c.Assert(ch.Metrics(), gc.DeepEquals, metrics)
	c.Assert(ch.Revision(), gc.DeepEquals, revision)

	// Test URL matches charm and expected series.
	url := ch.URL()
	c.Assert(url.Series, gc.Equals, series)
	c.Assert(url.Revision, gc.Equals, ch.Revision())

	// Ignore the StoragePath and BundleSHA256 methods, they're irrelevant.
}

func assertStandardCharm(c *gc.C, ch *state.Charm, series string) {
	chd := testcharms.Repo.CharmDir(ch.Meta().Name)
	assertCustomCharm(c, ch, series, chd.Meta(), chd.Config(), chd.Metrics(), chd.Revision())
}

func forEachStandardCharm(c *gc.C, f func(name string)) {
	for _, name := range []string{
		"logging", "mysql", "riak", "wordpress",
	} {
		c.Logf("checking %s", name)
		f(name)
	}
}

func (s *CharmTestHelperSuite) TestSimple(c *gc.C) {
	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		meta := chd.Meta()
		config := chd.Config()
		metrics := chd.Metrics()
		revision := chd.Revision()

		ch := s.AddTestingCharm(c, name)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, revision)

		ch = s.AddSeriesCharm(c, name, "anotherseries")
		assertCustomCharm(c, ch, "anotherseries", meta, config, metrics, revision)
	})
}

var configYaml = `
options:
  working:
    description: when set to false, prevents service from functioning correctly
    default: true
    type: boolean
`

func (s *CharmTestHelperSuite) TestConfigCharm(c *gc.C) {
	config, err := charm.ReadConfig(bytes.NewBuffer([]byte(configYaml)))
	c.Assert(err, jc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		meta := chd.Meta()
		metrics := chd.Metrics()

		ch := s.AddConfigCharm(c, name, configYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, 123)
	})
}

var actionsYaml = `
actions:
   dump:
      description: Dump the database to STDOUT.
      params:
         redirect-file:
            description: Redirect to a log file.
            type: string
`

func (s *CharmTestHelperSuite) TestActionsCharm(c *gc.C) {
	actions, err := charm.ReadActionsYaml(bytes.NewBuffer([]byte(actionsYaml)))
	c.Assert(err, jc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		ch := s.AddActionsCharm(c, name, actionsYaml, 123)
		c.Assert(ch.Actions(), gc.DeepEquals, actions)
	})
}

var metricsYaml = `
metrics:
  blips:
    description: A custom metric.
    type: gauge
`

func (s *CharmTestHelperSuite) TestMetricsCharm(c *gc.C) {
	metrics, err := charm.ReadMetrics(bytes.NewBuffer([]byte(metricsYaml)))
	c.Assert(err, jc.ErrorIsNil)

	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		meta := chd.Meta()
		config := chd.Config()

		ch := s.AddMetricsCharm(c, name, metricsYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, 123)
	})
}

var metaYamlSnippet = `
summary: blah
description: blah blah
`

func (s *CharmTestHelperSuite) TestMetaCharm(c *gc.C) {
	forEachStandardCharm(c, func(name string) {
		chd := testcharms.Repo.CharmDir(name)
		config := chd.Config()
		metrics := chd.Metrics()
		metaYaml := "name: " + name + metaYamlSnippet
		meta, err := charm.ReadMeta(bytes.NewBuffer([]byte(metaYaml)))
		c.Assert(err, jc.ErrorIsNil)

		ch := s.AddMetaCharm(c, name, metaYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, metrics, 123)
	})
}

func (s *CharmTestHelperSuite) TestTestingCharm(c *gc.C) {
	added := s.AddTestingCharm(c, "metered")
	c.Assert(added.Metrics(), gc.NotNil)

	chd := testcharms.Repo.CharmDir("metered")
	c.Assert(chd.Metrics(), gc.DeepEquals, added.Metrics())
}
