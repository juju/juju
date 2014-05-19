// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"bytes"
	"net/url"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
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
	c.Assert(err, gc.IsNil)
	c.Assert(dummy.URL().String(), gc.Equals, s.curl.String())
	c.Assert(dummy.Revision(), gc.Equals, 1)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/quantal-dummy-1")
	c.Assert(err, gc.IsNil)
	c.Assert(dummy.BundleURL(), gc.DeepEquals, bundleURL)
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

func assertCustomCharm(c *gc.C, ch *state.Charm, series string, meta *charm.Meta, config *charm.Config, revision int) {
	// Check Charm interface method results.
	c.Assert(ch.Meta(), gc.DeepEquals, meta)
	c.Assert(ch.Config(), gc.DeepEquals, config)
	c.Assert(ch.Revision(), gc.DeepEquals, revision)

	// Test URL matches charm and expected series.
	url := ch.URL()
	c.Assert(url.Series, gc.Equals, series)
	c.Assert(url.Revision, gc.Equals, ch.Revision())

	// Ignore the BundleURL and BundleSHA256 methods, they're irrelevant.
}

func assertStandardCharm(c *gc.C, ch *state.Charm, series string) {
	chd := testing.Charms.Dir(ch.Meta().Name)
	assertCustomCharm(c, ch, series, chd.Meta(), chd.Config(), chd.Revision())
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
		chd := testing.Charms.Dir(name)
		meta := chd.Meta()
		config := chd.Config()
		revision := chd.Revision()

		ch := s.AddTestingCharm(c, name)
		assertCustomCharm(c, ch, "quantal", meta, config, revision)

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

func (s *CharmTestHelperSuite) TestConfigCharm(c *gc.C) {
	config, err := charm.ReadConfig(bytes.NewBuffer([]byte(configYaml)))
	c.Assert(err, gc.IsNil)

	forEachStandardCharm(c, func(name string) {
		chd := testing.Charms.Dir(name)
		meta := chd.Meta()

		ch := s.AddConfigCharm(c, name, configYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, 123)
	})
}

var metaYamlSnippet = `
summary: blah
description: blah blah
`

func (s *CharmTestHelperSuite) TestMetaCharm(c *gc.C) {
	forEachStandardCharm(c, func(name string) {
		chd := testing.Charms.Dir(name)
		config := chd.Config()
		metaYaml := "name: " + name + metaYamlSnippet
		meta, err := charm.ReadMeta(bytes.NewBuffer([]byte(metaYaml)))
		c.Assert(err, gc.IsNil)

		ch := s.AddMetaCharm(c, name, metaYaml, 123)
		assertCustomCharm(c, ch, "quantal", meta, config, 123)
	})
}
