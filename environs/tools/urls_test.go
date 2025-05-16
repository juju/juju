// Copyright 2012, 2013, 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
)

type URLsSuite struct {
	coretesting.BaseSuite
}

func TestURLsSuite(t *stdtesting.T) { tc.Run(t, &URLsSuite{}) }
func (s *URLsSuite) env(c *tc.C, toolsMetadataURL string) environs.Environ {
	attrs := coretesting.FakeConfig()
	if toolsMetadataURL != "" {
		attrs = attrs.Merge(coretesting.Attrs{
			"agent-metadata-url": toolsMetadataURL,
		})
	}
	env, err := bootstrap.PrepareController(false, envtesting.BootstrapTestContext(c),
		jujuclient.NewMemStore(),
		bootstrap.PrepareParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			ControllerName:   attrs["name"].(string),
			ModelConfig:      attrs,
			Cloud:            coretesting.FakeCloudSpec(),
			AdminSecret:      "admin-secret",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	return env.(environs.Environ)
}

func (s *URLsSuite) TestToolsURLsNoConfigURL(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	env := s.env(c, "")
	sources, err := tools.GetMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{{"https://streams.canonical.com/juju/tools/", keys.JujuPublicKey, true}})
}

func (s *URLsSuite) TestToolsSources(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	env := s.env(c, "config-tools-metadata-url")
	sources, err := tools.GetMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"config-tools-metadata-url/", keys.JujuPublicKey, false},
		{"https://streams.canonical.com/juju/tools/", keys.JujuPublicKey, true},
	})
}

func (s *URLsSuite) TestToolsMetadataURLsRegisteredFuncs(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	tools.RegisterToolsDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		return ss.NewDataSource(simplestreams.Config{
			Description:          "id0",
			BaseURL:              "betwixt/releases",
			HostnameVerification: false,
			Priority:             simplestreams.DEFAULT_CLOUD_DATA}), nil
	})
	tools.RegisterToolsDataSourceFunc("id1", func(environs.Environ) (simplestreams.DataSource, error) {
		return ss.NewDataSource(simplestreams.Config{
			Description:          "id1",
			BaseURL:              "yoink",
			HostnameVerification: false,
			Priority:             simplestreams.SPECIFIC_CLOUD_DATA}), nil
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
	sources, err := tools.GetMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	sstesting.AssertExpectedSources(c, sources, []sstesting.SourceDetails{
		{"config-tools-metadata-url/", keys.JujuPublicKey, false},
		{"betwixt/releases/", "", false},
		{"https://streams.canonical.com/juju/tools/", keys.JujuPublicKey, true},
	})
}

func (s *URLsSuite) TestToolsMetadataURLsRegisteredFuncsError(c *tc.C) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	tools.RegisterToolsDataSourceFunc("id0", func(environs.Environ) (simplestreams.DataSource, error) {
		// Non-NotSupported errors cause GetMetadataSources to fail.
		return nil, errors.New("oyvey!")
	})
	defer tools.UnregisterToolsDataSourceFunc("id0")

	env := s.env(c, "config-tools-metadata-url")
	_, err := tools.GetMetadataSources(env, ss)
	c.Assert(err, tc.ErrorMatches, "oyvey!")
}

func (s *URLsSuite) TestToolsURL(c *tc.C) {
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
	}, {
		in:          "/home/foo",
		expected:    "file:///home/foo/tools",
		expectedErr: nil,
	}, {
		in:          "/home/foo/tools",
		expected:    "file:///home/foo/tools",
		expectedErr: nil,
	}}

	for i, t := range toolsTests {
		c.Logf("Test %d:", i)

		out, err := tools.ToolsURL(t.in)
		c.Assert(err, tc.DeepEquals, t.expectedErr)
		c.Assert(out, tc.Equals, t.expected)
	}
}
