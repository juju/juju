// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"net/url"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
)

type CharmSuite struct {
	ConnSuite
	curl *charm.URL
}

var _ = Suite(&CharmSuite{})

func (s *CharmSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	added := s.AddTestingCharm(c, "dummy")
	s.curl = added.URL()
}

func (s *CharmSuite) TestCharm(c *C) {
	dummy, err := s.State.Charm(s.curl)
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())
	c.Assert(dummy.Revision(), Equals, 1)
	bundleURL, err := url.Parse("http://bundles.example.com/series-dummy-1")
	c.Assert(err, IsNil)
	c.Assert(dummy.BundleURL(), DeepEquals, bundleURL)
	c.Assert(dummy.BundleSha256(), Equals, "series-dummy-1-sha256")
	meta := dummy.Meta()
	c.Assert(meta.Name, Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
}

func (s *CharmSuite) TestCharmNotFound(c *C) {
	curl := charm.MustParseURL("local:anotherseries/dummy-1")
	_, err := s.State.Charm(curl)
	c.Assert(err, ErrorMatches, `charm "local:anotherseries/dummy-1" not found`)
	c.Assert(err, checkers.Satisfies, errors.IsNotFoundError)
}

type CharmTestHelperSuite struct {
	ConnSuite
}

var _ = Suite(&CharmTestHelperSuite{})

func assertCustomCharm(c *C, ch *state.Charm, series string, meta *charm.Meta, config *charm.Config, revision int) {
	// Check Charm interface method results.
	c.Assert(ch.Meta(), DeepEquals, meta)
	c.Assert(ch.Config(), DeepEquals, config)
	c.Assert(ch.Revision(), DeepEquals, revision)

	// Test URL matches charm and expected series.
	url := ch.URL()
	c.Assert(url.Series, Equals, series)
	c.Assert(url.Revision, Equals, ch.Revision())

	// Ignore the BundleURL and BundleSHA256 methods, they're irrelevant.
}

func assertStandardCharm(c *C, ch *state.Charm, series string) {
	chd := testing.Charms.Dir(ch.Meta().Name)
	assertCustomCharm(c, ch, series, chd.Meta(), chd.Config(), chd.Revision())
}

func forEachStandardCharm(c *C, f func(name string)) {
	for _, name := range []string{
		"logging", "mysql", "riak", "wordpress",
	} {
		c.Logf("checking %s", name)
		f(name)
	}
}

func (s *CharmTestHelperSuite) TestSimple(c *C) {
	forEachStandardCharm(c, func(name string) {
		chd := testing.Charms.Dir(name)
		meta := chd.Meta()
		config := chd.Config()
		revision := chd.Revision()

		ch := s.AddTestingCharm(c, name)
		assertCustomCharm(c, ch, "series", meta, config, revision)

		ch = s.AddSeriesCharm(c, name, "anotherseries")
		assertCustomCharm(c, ch, "anotherseries", meta, config, revision)
	})
}

var configYaml = `
options:
  working:
    description: when set to false, prevents service from functioning correctly
    default: true
    type: boolean
`

func (s *CharmTestHelperSuite) TestConfigCharm(c *C) {
	config, err := charm.ReadConfig(bytes.NewBuffer([]byte(configYaml)))
	c.Assert(err, IsNil)

	forEachStandardCharm(c, func(name string) {
		chd := testing.Charms.Dir(name)
		meta := chd.Meta()

		ch := s.AddConfigCharm(c, name, configYaml, 123)
		assertCustomCharm(c, ch, "series", meta, config, 123)
	})
}

var metaYamlSnippet = `
summary: blah
description: blah blah
`

func (s *CharmTestHelperSuite) TestMetaCharm(c *C) {
	forEachStandardCharm(c, func(name string) {
		chd := testing.Charms.Dir(name)
		config := chd.Config()
		metaYaml := "name: " + name + metaYamlSnippet
		meta, err := charm.ReadMeta(bytes.NewBuffer([]byte(metaYaml)))
		c.Assert(err, IsNil)

		ch := s.AddMetaCharm(c, name, metaYaml, 123)
		assertCustomCharm(c, ch, "series", meta, config, 123)
	})
}
