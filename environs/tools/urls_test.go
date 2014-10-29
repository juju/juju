// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"github.com/juju/errors"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
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
			"agent-metadata-url": toolsMetadataURL,
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
	sstesting.AssertExpectedSources(c, sources, []string{"https://streams.canonical.com/juju/tools/"})
}

func (s *URLsSuite) TestToolsSources(c *gc.C) {
	env := s.env(c, "config-tools-metadata-url")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-tools-metadata-url/", "https://streams.canonical.com/juju/tools/"})
}

func (s *URLsSuite) TestToolsMetadataURLsRegisteredFuncs(c *gc.C) {
	tools.RegisterToolsDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		return simplestreams.NewURLDataSource("id0", "betwixt/releases", utils.NoVerifySSLHostnames), nil
	})
	tools.RegisterToolsDataSourceFunc("id1", func(environs.Environ) (simplestreams.DataSource, error) {
		return simplestreams.NewURLDataSource("id1", "yoink", utils.NoVerifySSLHostnames), nil
	})
	// overwrite the one previously registered against id1
	tools.RegisterToolsDataSourceFunc("id1", func(environs.Environ) (simplestreams.DataSource, error) {
		// NotSupported errors do not cause GetMetadataSources to fail,
		// they just cause the datasource function to be ignored.
		return nil, errors.NewNotSupported(nil, "oyvey")
	})
	defer tools.UnregisterToolsDataSourceFunc("id0")
	defer tools.UnregisterToolsDataSourceFunc("id1")

	env := s.env(c, "config-tools-metadata-url")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	sstesting.AssertExpectedSources(c, sources, []string{
		"config-tools-metadata-url/",
		"betwixt/releases/",
		"https://streams.canonical.com/juju/tools/",
	})
}

func (s *URLsSuite) TestToolsMetadataURLsRegisteredFuncsError(c *gc.C) {
	tools.RegisterToolsDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		// Non-NotSupported errors cause GetMetadataSources to fail.
		return nil, errors.New("oyvey!")
	})
	defer tools.UnregisterToolsDataSourceFunc("id0")

	env := s.env(c, "config-tools-metadata-url")
	_, err := tools.GetMetadataSources(env)
	c.Assert(err, gc.ErrorMatches, "oyvey!")
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
