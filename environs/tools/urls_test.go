// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

type URLsSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&URLsSuite{})

func (s *URLsSuite) TearDownTest(c *gc.C) {
	dummy.Reset(c)

	s.BaseSuite.TearDownTest(c)
}

func (s *URLsSuite) env(c *gc.C, toolsMetadataURL string) environs.Environ {
	attrs := dummy.SampleConfig()
	if toolsMetadataURL != "" {
		attrs = attrs.Merge(testing.Attrs{
			"agent-metadata-url": toolsMetadataURL,
		})
	}
	env, err := bootstrap.PrepareController(false, envtesting.BootstrapContext(c),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			ControllerName:   attrs["name"].(string),
			ModelConfig:      attrs,
			Cloud:            dummy.SampleCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	return env.(environs.Environ)
}

func (s *URLsSuite) TestToolsURLsNoConfigURL(c *gc.C) {
	env := s.env(c, "")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{{"https://streams.canonical.com/juju/tools/", keys.JujuPublicKey}})
}

func (s *URLsSuite) TestToolsSources(c *gc.C) {
	env := s.env(c, "config-tools-metadata-url")
	sources, err := tools.GetMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"config-tools-metadata-url/", keys.JujuPublicKey},
		{"https://streams.canonical.com/juju/tools/", keys.JujuPublicKey},
	})
}

func (s *URLsSuite) TestToolsMetadataURLsRegisteredFuncs(c *gc.C) {
	tools.RegisterToolsDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		return simplestreams.NewURLDataSource("id0", "betwixt/releases", utils.NoVerifySSLHostnames, simplestreams.DEFAULT_CLOUD_DATA, false), nil
	})
	tools.RegisterToolsDataSourceFunc("id1", func(environs.Environ) (simplestreams.DataSource, error) {
		return simplestreams.NewURLDataSource("id1", "yoink", utils.NoVerifySSLHostnames, simplestreams.SPECIFIC_CLOUD_DATA, false), nil
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
	c.Assert(err, jc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"config-tools-metadata-url/", keys.JujuPublicKey},
		{"betwixt/releases/", ""},
		{"https://streams.canonical.com/juju/tools/", keys.JujuPublicKey},
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
	var toolsTests = []struct {
		in          string
		expected    string
		expectedErr error
	}{{
		in:          "",
		expected:    "",
		expectedErr: nil,
	}, {
		in:          "file://foo",
		expected:    "file://foo",
		expectedErr: nil,
	}, {
		in:          "http://foo",
		expected:    "http://foo",
		expectedErr: nil,
	}, {
		in:          "foo",
		expected:    "",
		expectedErr: fmt.Errorf("foo is not an absolute path"),
	}}
	toolsTests = append(toolsTests, toolsTestsPlatformSpecific...)
	for i, t := range toolsTests {
		c.Logf("Test %d:", i)

		out, err := tools.ToolsURL(t.in)
		c.Assert(err, gc.DeepEquals, t.expectedErr)
		c.Assert(out, gc.Equals, t.expected)
	}
}
