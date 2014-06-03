// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

type URLsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) TearDownTest(c *gc.C) {
	dummy.Reset()
	s.BaseSuite.TearDownTest(c)
}

func (s *URLsSuite) env(c *gc.C, toolsMetadataURL string) environs.Environ {
	attrs := dummy.SampleConfig()
	if toolsMetadataURL != "" {
		attrs = attrs.Merge(testing.Attrs{
			"tools-metadata-url": toolsMetadataURL,
		})
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := environs.Prepare(cfg, testing.Context(c), configstore.NewMem())
	c.Assert(err, gc.IsNil)
	return env
}

func (s *URLsSuite) TestToolsURLsNoConfigURL(c *gc.C) {
	env := s.env(c, "")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	// Put a file in tools since the dummy storage provider requires a
	// file to exist before the URL can be found. This is to ensure it behaves
	// the same way as MAAS.
	err = env.Storage().Put("tools/dummy", strings.NewReader("dummy"), 5)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("tools")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		privateStorageURL, "https://streams.canonical.com/juju/tools/"})
}

func (s *URLsSuite) TestToolsSources(c *gc.C) {
	env := s.env(c, "config-tools-metadata-url")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	// Put a file in tools since the dummy storage provider requires a
	// file to exist before the URL can be found. This is to ensure it behaves
	// the same way as MAAS.
	err = env.Storage().Put("tools/dummy", strings.NewReader("dummy"), 5)
	c.Assert(err, gc.IsNil)
	privateStorageURL, err := env.Storage().URL("tools")
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-tools-metadata-url/", privateStorageURL, "https://streams.canonical.com/juju/tools/"})
	haveExpectedSources := false
	for _, source := range sources {
		if allowRetry, ok := storage.TestingGetAllowRetry(source); ok {
			haveExpectedSources = true
			c.Assert(allowRetry, jc.IsFalse)
		}
	}
	c.Assert(haveExpectedSources, jc.IsTrue)
}

func (s *URLsSuite) TestToolsSourcesWithRetry(c *gc.C) {
	env := s.env(c, "")
	sources, err := tools.GetMetadataSourcesWithRetries(env, true)
	c.Assert(err, gc.IsNil)
	haveExpectedSources := false
	for _, source := range sources {
		if allowRetry, ok := storage.TestingGetAllowRetry(source); ok {
			haveExpectedSources = true
			c.Assert(allowRetry, jc.IsTrue)
		}
	}
	c.Assert(haveExpectedSources, jc.IsTrue)
	c.Assert(haveExpectedSources, jc.IsTrue)
}

func (s *URLsSuite) TestToolsURL(c *gc.C) {
	for source, expected := range map[string]string{
		"":           "",
		"foo":        "file://foo/tools",
		"/home/foo":  "file:///home/foo/tools",
		"file://foo": "file://foo",
		"http://foo": "http://foo",
	} {
		URL, err := tools.ToolsURL(source)
		c.Assert(err, gc.IsNil)
		c.Assert(URL, gc.Equals, expected)
	}
}
