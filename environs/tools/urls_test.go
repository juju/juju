// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/environs"
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
	attrs := dummy.SampleConfig
	if toolsMetadataURL != "" {
		attrs = attrs.Merge(testing.Attrs{
			"tools-url": toolsMetadataURL,
		})
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, gc.IsNil)
	return env
}

func (s *URLsSuite) TestToolsURLsNoConfigURL(c *gc.C) {
	urls, err := tools.GetMetadataURLs(s.env(c, ""))
	c.Assert(err, gc.IsNil)
	c.Assert(urls, gc.DeepEquals, []string{
		"dummy-tools-url", "http://juju.canonical.com/tools"})
}

func (s *URLsSuite) TestToolsURLs(c *gc.C) {
	urls, err := tools.GetMetadataURLs(s.env(c, "config-tools-url"))
	c.Assert(err, gc.IsNil)
	c.Assert(urls, gc.DeepEquals, []string{
		"config-tools-url", "dummy-tools-url", "http://juju.canonical.com/tools"})
}
