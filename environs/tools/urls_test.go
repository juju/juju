// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	sstesting "launchpad.net/juju-core/environs/simplestreams/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/testing"
)

type URLsSuite struct {
	home *testing.FakeHome
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) SetUpTest(c *gc.C) {
	s.home = testing.MakeEmptyFakeHome(c)
}

func (s *URLsSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
}

func (s *URLsSuite) env(c *gc.C, toolsMetadataURL string) environs.Environ {
	attrs := map[string]interface{}{
		"name":            "only",
		"type":            "dummy",
		"authorized-keys": "foo",
		"state-server":    true,
		"ca-cert":         testing.CACert,
		"ca-private-key":  testing.CAKey,
	}
	if toolsMetadataURL != "" {
		attrs["tools-url"] = toolsMetadataURL
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, gc.IsNil)
	env, err = environs.Prepare(env.Config())
	c.Assert(err, gc.IsNil)
	return env
}

func (s *URLsSuite) TestToolsURLsNoConfigURL(c *gc.C) {
	env := s.env(c, "")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("tools")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		privateStorageURL, "https://juju.canonical.com/tools/"})
}

func (s *URLsSuite) TestToolsSources(c *gc.C) {
	env := s.env(c, "config-tools-url")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("tools")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-tools-url/", privateStorageURL, "https://juju.canonical.com/tools/"})
}
