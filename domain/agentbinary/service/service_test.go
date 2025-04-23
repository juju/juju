// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/environs"
	config "github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	internaltesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
)

type serviceSuite struct {
	testing.IsolationSuite

	mockModelState, mockControllerState *MockAgentBinaryState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelState = NewMockAgentBinaryState(ctrl)
	s.mockControllerState = NewMockAgentBinaryState(ctrl)
	return ctrl
}

// TestListAgentBinaries tests the ListAgentBinaries method of the
// AgentBinaryService. It verifies that the method correctly merges
// agent binaries from the controller and model stores, with the model
// binaries taking precedence over the controller binaries.
func (s *serviceSuite) TestListAgentBinaries(c *gc.C) {
	defer s.setupMocks(c).Finish()

	controllerBinaries := []agentbinary.Metadata{
		{
			Version: "4.0.0",
			Size:    1,
			SHA256:  "sha256hash-1",
		},
		{
			Version: "4.0.1",
			Size:    2,
			SHA256:  "sha256hash-2",
		},
	}
	modelBinaries := []agentbinary.Metadata{
		{
			Version: "4.0.1",
			Size:    222,
			// A same SHA should never have a different size, but this is just for testing the merge logic.
			SHA256: "sha256hash-2",
		},
		{
			Version: "4.0.2",
			Size:    3,
			SHA256:  "sha256hash-3",
		},
	}
	expected := []agentbinary.Metadata{
		{
			Version: "4.0.0",
			Size:    1,
			SHA256:  "sha256hash-1",
		},
		{
			Version: "4.0.1",
			Size:    222,
			SHA256:  "sha256hash-2",
		},
		{
			Version: "4.0.2",
			Size:    3,
			SHA256:  "sha256hash-3",
		},
	}
	s.mockControllerState.EXPECT().ListAgentBinaries(gomock.Any()).Return(controllerBinaries, nil)
	s.mockModelState.EXPECT().ListAgentBinaries(gomock.Any()).Return(modelBinaries, nil)

	svc := NewAgentBinaryService(s.mockControllerState, s.mockModelState, nil, nil, nil)
	result, err := svc.ListAgentBinaries(context.Background())
	c.Assert(err, gc.IsNil)
	c.Assert(result, jc.SameContents, expected)
}

func (s *serviceSuite) TestGetEnvironAgentBinariesFinder(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	provider := NewMockProviderForAgentBinaryFinder(ctrl)
	modelAttrs := internaltesting.FakeConfig().Merge(internaltesting.Attrs{
		"agent-stream": "stream2",
		"development":  true,
	})
	modelCfg, err := config.New(config.NoDefaults, modelAttrs)
	c.Assert(err, jc.ErrorIsNil)
	provider.EXPECT().Config().Return(modelCfg)

	called := 0
	getPreferredSimpleStreams := func(
		vers *semversion.Number,
		forceDevel bool,
		stream string,
	) []string {
		called++
		c.Assert(*vers, jc.DeepEquals, semversion.MustParse("4.0.1"))
		c.Assert(forceDevel, gc.Equals, true)
		c.Assert(stream, gc.Equals, "stream2")
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
		c.Assert(majorVersion, gc.Equals, 4)
		c.Assert(minorVersion, gc.Equals, 0)
		c.Assert(streams, gc.DeepEquals, []string{"stream1", "stream2"})
		c.Assert(filter, gc.DeepEquals, coretools.Filter{Arch: "amd64"})
		return expected, nil
	}

	svc := NewAgentBinaryService(s.mockControllerState, s.mockModelState,
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return provider, nil
		},
		getPreferredSimpleStreams, agentBinaryFilter,
	)
	finder := svc.GetEnvironAgentBinariesFinder()
	result, err := finder(context.Background(),
		4, 0, semversion.MustParse("4.0.1"), "", coretools.Filter{Arch: "amd64"},
	)
	c.Assert(called, gc.Equals, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)
}
