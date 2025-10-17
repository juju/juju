// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/semversion"
	domainagenterrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/testhelpers"
	internaltesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

type simplestreamStoreSuite struct {
	testhelpers.IsolationSuite

	mockProviderForAgentBinaryFinder *MockProviderForAgentBinaryFinder
	getPreferredSimpleStreams        PreferredSimpleStreamsFunc
	agentBinaryFilter                AgentBinaryFilter
	httpClient                       *MockHTTPClient
}

func TestSimplestreamStoreSuite(t *testing.T) {
	tc.Run(t, &simplestreamStoreSuite{})
}

func (s *simplestreamStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockProviderForAgentBinaryFinder = NewMockProviderForAgentBinaryFinder(ctrl)

	modelAttrs := internaltesting.FakeConfig().Merge(internaltesting.Attrs{
		"agent-stream": "stream2",
		"development":  true,
	})
	modelCfg, err := config.New(config.NoDefaults, modelAttrs)
	c.Assert(err, tc.ErrorIsNil)
	s.mockProviderForAgentBinaryFinder.EXPECT().Config().Return(modelCfg)
	return ctrl
}

func (s *simplestreamStoreSuite) TestGetEnvironAgentBinariesFinder(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelAttrs := internaltesting.FakeConfig().Merge(internaltesting.Attrs{
		"agent-stream": "stream2",
		"development":  true,
	})
	modelCfg, err := config.New(config.NoDefaults, modelAttrs)
	c.Assert(err, tc.ErrorIsNil)
	s.mockProviderForAgentBinaryFinder.EXPECT().Config().Return(modelCfg)

	called := 0
	getPreferredSimpleStreams := func(
		vers *semversion.Number,
		forceDevel bool,
		stream string,
	) []string {
		c.Assert(*vers, tc.DeepEquals, semversion.MustParse("4.0.1"))
		c.Assert(forceDevel, tc.Equals, true)
		c.Assert(stream, tc.Equals, "stream2")
		return []string{"stream1", "stream2"}
	}

	expected := coretools.List{
		{Version: semversion.MustParseBinary("4.0.1-ubuntu-amd64")},
	}
	agentBinaryFilter := func(
		_ context.Context,
		_ envtools.SimplestreamsFetcher,
		_ environs.BootstrapEnviron,
		majorVersion,
		minorVersion int,
		streams []string,
		filter coretools.Filter,
	) (coretools.List, error) {
		called++
		c.Assert(majorVersion, tc.Equals, 4)
		c.Assert(minorVersion, tc.Equals, 0)
		c.Assert(streams, tc.DeepEquals, []string{"stream1", "stream2"})
		c.Assert(filter, tc.DeepEquals, coretools.Filter{Arch: "amd64"})
		return expected, nil
	}
	simpleStreamStore := NewSimpleStreamAgentBinaryStore(func(context.Context) (ProviderForAgentBinaryFinder, error) {
		return s.mockProviderForAgentBinaryFinder, nil
	}, getPreferredSimpleStreams, agentBinaryFilter, s.httpClient)

	finder := simpleStreamStore.GetEnvironAgentBinariesFinder()
	result, err := finder(c.Context(),
		4, 0, semversion.MustParse("4.0.1"), "", coretools.Filter{Arch: "amd64"},
	)

	c.Assert(called, tc.Equals, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *simplestreamStoreSuite) initialiseDefaultEnvironAgentBinariesFinder(c *tc.C) {
	getPreferredSimpleStreams := func(
		vers *semversion.Number,
		forceDevel bool,
		stream string,
	) []string {
		c.Assert(*vers, tc.DeepEquals, semversion.MustParse("4.0.1"))
		c.Assert(forceDevel, tc.Equals, true)
		c.Assert(stream, tc.Equals, "stream2")
		return []string{"stream1", "stream2"}
	}
	s.getPreferredSimpleStreams = getPreferredSimpleStreams

	agentBinaryFilter := func(
		_ context.Context,
		_ envtools.SimplestreamsFetcher,
		_ environs.BootstrapEnviron,
		majorVersion,
		minorVersion int,
		streams []string,
		filter coretools.Filter,
	) (coretools.List, error) {
		c.Assert(majorVersion, tc.Equals, 4)
		c.Assert(minorVersion, tc.Equals, 0)
		c.Assert(streams, tc.DeepEquals, []string{"stream1", "stream2"})
		c.Assert(filter, tc.DeepEquals, coretools.Filter{Arch: "amd64"})
		return coretools.List{
			{Version: semversion.MustParseBinary("4.0.1-ubuntu-amd64")},
		}, nil
	}
	s.agentBinaryFilter = agentBinaryFilter
}

func (s *simplestreamStoreSuite) TestGetAgentBinary(c *tc.C) {
	s.initialiseDefaultEnvironAgentBinariesFinder(c)
	simpleStreamStore := NewSimpleStreamAgentBinaryStore(func(context.Context) (ProviderForAgentBinaryFinder, error) {
		return s.mockProviderForAgentBinaryFinder, nil
	}, s.getPreferredSimpleStreams, s.agentBinaryFilter, s.httpClient)

	s.httpClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("test")),
	}, nil)
	ver := agentbinary.Version{Number: semversion.MustParse("4.0.1"), Arch: "amd64"}
	data, size, sha256Sum, err := simpleStreamStore.GetAgentBinary(c.Context(), ver, modelagent.AgentStreamDevel)
	c.Assert(err, tc.IsNil)

	defer func() {
		err := data.Close()
		c.Assert(err, tc.IsNil)
	}()

	// SHA256 of "test"
	const expectedSHA = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	c.Assert(sha256Sum, tc.Equals, expectedSHA)

	// Read and verify content
	dataBytes, readErr := io.ReadAll(data)
	c.Assert(readErr, tc.IsNil)
	c.Assert(string(dataBytes), tc.Equals, "test")
	c.Assert(size, tc.Equals, 4)
}

func (s *simplestreamStoreSuite) TestGetAgentBinaryNotFound(c *tc.C) {
	s.initialiseDefaultEnvironAgentBinariesFinder(c)
	simpleStreamStore := NewSimpleStreamAgentBinaryStore(func(context.Context) (ProviderForAgentBinaryFinder, error) {
		return s.mockProviderForAgentBinaryFinder, nil
	}, s.getPreferredSimpleStreams, s.agentBinaryFilter, s.httpClient)

	s.httpClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("test")),
	}, nil)
	ver := agentbinary.Version{Number: semversion.MustParse("3.0.1"), Arch: "amd64"}
	_, size, sha256Sum, err := simpleStreamStore.GetAgentBinary(c.Context(), ver, modelagent.AgentStreamDevel)
	c.Assert(err, tc.ErrorIs, domainagenterrors.NotFound)
	c.Assert(size, tc.Equals, 0)
	c.Assert(sha256Sum, tc.Equals, "")
}

func (s *simplestreamStoreSuite) TestGetAgentBinaryHTTPClientError(c *tc.C) {
	s.initialiseDefaultEnvironAgentBinariesFinder(c)
	simpleStreamStore := NewSimpleStreamAgentBinaryStore(func(context.Context) (ProviderForAgentBinaryFinder, error) {
		return s.mockProviderForAgentBinaryFinder, nil
	}, s.getPreferredSimpleStreams, s.agentBinaryFilter, s.httpClient)

	s.httpClient.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil)
	ver := agentbinary.Version{Number: semversion.MustParse("4.0.1"), Arch: "amd64"}
	_, size, sha256Sum, err := simpleStreamStore.GetAgentBinary(c.Context(), ver, modelagent.AgentStreamDevel)
	c.Assert(err, tc.ErrorMatches, "bad HTTP response with status: 404")
	c.Assert(size, tc.Equals, 0)
	c.Assert(sha256Sum, tc.Equals, "")
}
