// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/environs"
	config "github.com/juju/juju/environs/config"
	envtools "github.com/juju/juju/environs/tools"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	mockModelState                    *MockModelState
	mockControllerState               *MockControllerState
	mockAgentBinaryDiscoverableStore1 *MockAgentBinaryDiscoverableStore
	mockAgentBinaryDiscoverableStore2 *MockAgentBinaryDiscoverableStore
	mockAgentBinaryStoreState         *MockAgentBinaryStoreState
	mockAgentBinaryLocalStore         *MockAgentBinaryLocalStore
	mockProvider                      *MockProviderForAgentBinaryFinder
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelState = NewMockModelState(ctrl)
	s.mockControllerState = NewMockControllerState(ctrl)
	s.mockAgentBinaryStoreState = NewMockAgentBinaryStoreState(ctrl)
	s.mockAgentBinaryDiscoverableStore1 = NewMockAgentBinaryDiscoverableStore(ctrl)
	s.mockAgentBinaryDiscoverableStore2 = NewMockAgentBinaryDiscoverableStore(ctrl)
	s.mockProvider = NewMockProviderForAgentBinaryFinder(ctrl)
	s.mockAgentBinaryLocalStore = NewMockAgentBinaryLocalStore(ctrl)
	return ctrl
}

func (s *serviceSuite) TestListAgentBinaries(c *tc.C) {
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

	agentBinaryFilter := func(
		_ context.Context,
		_ envtools.SimplestreamsFetcher,
		_ environs.BootstrapEnviron,
		majorVersion,
		minorVersion int,
		streams []string,
		filter tools.Filter,
	) (tools.List, error) {
		c.Assert(majorVersion, tc.Equals, 4)
		c.Assert(minorVersion, tc.Equals, 0)
		c.Assert(streams, tc.DeepEquals, []string{"stream1", "stream2"})
		c.Assert(filter, tc.DeepEquals, tools.Filter{Arch: "amd64"})
		return tools.List{
			{Version: semversion.MustParseBinary("4.0.1-ubuntu-amd64")},
		}, nil
	}

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		getPreferredSimpleStreams, agentBinaryFilter,
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)

	result, err := svc.ListAgentBinaries(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.SameContents, expected)
}

func (s *serviceSuite) TestGetEnvironAgentBinariesFinder(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelAttrs := internaltesting.FakeConfig().Merge(internaltesting.Attrs{
		"agent-stream": "stream2",
		"development":  true,
	})
	modelCfg, err := config.New(config.NoDefaults, modelAttrs)
	c.Assert(err, tc.ErrorIsNil)
	s.mockProvider.EXPECT().Config().Return(modelCfg)

	called := 0
	getPreferredSimpleStreams := func(
		vers *semversion.Number,
		forceDevel bool,
		stream string,
	) []string {
		called++
		c.Assert(*vers, tc.DeepEquals, semversion.MustParse("4.0.1"))
		c.Assert(forceDevel, tc.Equals, true)
		c.Assert(stream, tc.Equals, "stream2")
		return []string{"stream1", "stream2"}
	}

	expected := tools.List{
		{Version: semversion.MustParseBinary("4.0.1-ubuntu-amd64")},
	}
	agentBinaryFilter := func(
		_ context.Context,
		_ envtools.SimplestreamsFetcher,
		_ environs.BootstrapEnviron,
		majorVersion,
		minorVersion int,
		streams []string,
		filter tools.Filter,
	) (tools.List, error) {
		called++
		c.Assert(majorVersion, tc.Equals, 4)
		c.Assert(minorVersion, tc.Equals, 0)
		c.Assert(streams, tc.DeepEquals, []string{"stream1", "stream2"})
		c.Assert(filter, tc.DeepEquals, tools.Filter{Arch: "amd64"})
		return expected, nil
	}

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		getPreferredSimpleStreams, agentBinaryFilter,
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)
	finder := svc.GetEnvironAgentBinariesFinder()
	result, err := finder(c.Context(),
		4, 0, semversion.MustParse("4.0.1"), "", tools.Filter{Arch: "amd64"},
	)
	c.Assert(called, tc.Equals, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *serviceSuite) TestGetAgentBinary(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelState.EXPECT().GetAgentStream(gomock.Any()).Return(agentbinary.AgentStreamTesting, nil)

	payload := []byte("hello-agent-binary")
	size := int64(len(payload))
	sha256hex := fmt.Sprintf("%x", sha256.Sum256(payload))
	reader := io.NopCloser(bytes.NewReader(payload))

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	}

	s.mockAgentBinaryLocalStore.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		reader, size, sha256hex, nil,
	)

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		s.getDefaultPreferredSimpleStreams(c), s.getDefaultAgentBinaryFilter(c),
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)
	data, size, err := svc.GetAgentBinary(c.Context(), ver)
	c.Assert(data, tc.Equals, reader)
	c.Assert(size, tc.Equals, int64(len(payload)))
	c.Assert(err, tc.IsNil)
	c.Assert(data.Close(), tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetAgentBinaryNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelState.EXPECT().GetAgentStream(gomock.Any()).Return(agentbinary.AgentStreamTesting, nil)
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	}

	s.mockAgentBinaryLocalStore.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		nil, 0, "", agentbinaryerrors.NotFound,
	)

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		s.getDefaultPreferredSimpleStreams(c), s.getDefaultAgentBinaryFilter(c),
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)
	data, size, err := svc.GetAgentBinary(c.Context(), ver)
	c.Assert(err, tc.ErrorIs, agentbinaryerrors.NotFound)
	c.Assert(data, tc.IsNil)
	c.Assert(size, tc.Equals, int64(0))
}

func (s *serviceSuite) TestGetExternalAgentBinaryFromFirstDiscoverableStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelState.EXPECT().GetAgentStream(gomock.Any()).Return(agentbinary.AgentStreamTesting, nil)
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	}

	payload := []byte("hello-agent-binary")
	extSize := int64(len(payload))
	extSHA256 := fmt.Sprintf("%x", sha256.Sum256(payload))
	extReader := io.NopCloser(bytes.NewReader(payload))

	s.mockAgentBinaryDiscoverableStore1.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		extReader, extSize, extSHA256, nil,
	)

	s.mockAgentBinaryLocalStore.EXPECT().AddAgentBinaryWithSHA384(gomock.Any(), gomock.Any(), ver, extSize, gomock.Any()).Return(nil)

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		s.getDefaultPreferredSimpleStreams(c), s.getDefaultAgentBinaryFilter(c),
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)

	s.mockAgentBinaryLocalStore.EXPECT().GetAgentBinaryUsingSHA256(gomock.Any(), extSHA256).Return(extReader, extSize, nil)

	data, size, _, err := svc.GetExternalAgentBinary(c.Context(), ver)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, extReader)
	c.Assert(size, tc.Equals, extSize)
	c.Assert(data.Close(), tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetExternalAgentBinaryFromFirstDiscoverableStoreError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelState.EXPECT().GetAgentStream(gomock.Any()).Return(agentbinary.AgentStreamTesting, nil)
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	}

	s.mockAgentBinaryDiscoverableStore1.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		nil, 0, "", internalerrors.New("error with store1"),
	)

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		s.getDefaultPreferredSimpleStreams(c), s.getDefaultAgentBinaryFilter(c),
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)

	data, size, _, err := svc.GetExternalAgentBinary(c.Context(), ver)
	c.Assert(err, tc.ErrorMatches, `attempted fetching agent binary "4.0.1-amd64" from external store 0: error with store1`)
	c.Assert(data, tc.IsNil)
	c.Assert(size, tc.Equals, int64(0))
}

func (s *serviceSuite) TestGetExternalAgentBinaryFromFirstDiscoverableStoreHashMismatchError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelState.EXPECT().GetAgentStream(gomock.Any()).Return(agentbinary.AgentStreamTesting, nil)
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	}

	payload := []byte("hello-agent-binary")
	extSize := int64(len(payload))
	mismatchedExtSHA256 := fmt.Sprintf("%x", sha256.Sum256([]byte("mismatch")))
	extReader := io.NopCloser(bytes.NewReader(payload))
	s.mockAgentBinaryDiscoverableStore1.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		extReader, extSize, mismatchedExtSHA256, nil,
	)

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		s.getDefaultPreferredSimpleStreams(c), s.getDefaultAgentBinaryFilter(c),
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)

	data, size, _, err := svc.GetExternalAgentBinary(c.Context(), ver)
	c.Assert(err, tc.ErrorIs, agentbinaryerrors.HashMismatch)
	c.Assert(data, tc.IsNil)
	c.Assert(size, tc.Equals, int64(0))
}

func (s *serviceSuite) TestGetExternalAgentBinaryFromSecondDiscoverableStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelState.EXPECT().GetAgentStream(gomock.Any()).Return(agentbinary.AgentStreamTesting, nil)
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	}

	payload := []byte("hello-agent-binary")
	extSize := int64(len(payload))
	extSHA256 := fmt.Sprintf("%x", sha256.Sum256(payload))
	extReader := io.NopCloser(bytes.NewReader(payload))

	s.mockAgentBinaryDiscoverableStore1.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		nil, 0, "", agentbinaryerrors.NotFound,
	)

	s.mockAgentBinaryDiscoverableStore2.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		extReader, extSize, extSHA256, nil,
	)

	s.mockAgentBinaryLocalStore.EXPECT().AddAgentBinaryWithSHA384(gomock.Any(), gomock.Any(), ver, extSize, gomock.Any()).Return(nil)
	s.mockAgentBinaryLocalStore.EXPECT().GetAgentBinaryUsingSHA256(gomock.Any(), extSHA256).Return(extReader, extSize, nil)

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		s.getDefaultPreferredSimpleStreams(c), s.getDefaultAgentBinaryFilter(c),
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)

	data, size, _, err := svc.GetExternalAgentBinary(c.Context(), ver)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(data, tc.DeepEquals, extReader)
	c.Assert(size, tc.Equals, extSize)
	c.Assert(data.Close(), tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetExternalAgentBinaryFromSecondDiscoverableStoreError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelState.EXPECT().GetAgentStream(gomock.Any()).Return(agentbinary.AgentStreamTesting, nil)
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	}

	payload := []byte("hello-agent-binary")
	extSize := int64(len(payload))
	extSHA256 := fmt.Sprintf("%x", sha256.Sum256(payload))
	extReader := io.NopCloser(bytes.NewReader(payload))

	s.mockAgentBinaryDiscoverableStore1.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		nil, 0, "", agentbinaryerrors.NotFound,
	)

	s.mockAgentBinaryDiscoverableStore2.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		extReader, extSize, extSHA256, internalerrors.New("error with store2"),
	)

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		s.getDefaultPreferredSimpleStreams(c), s.getDefaultAgentBinaryFilter(c),
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)

	data, size, _, err := svc.GetExternalAgentBinary(c.Context(), ver)
	c.Assert(err, tc.ErrorMatches, `attempted fetching agent binary "4.0.1-amd64" from external store 1: error with store2`)
	c.Assert(data, tc.IsNil)
	c.Assert(size, tc.Equals, int64(0))
}

func (s *serviceSuite) TestGetExternalAgentBinaryFromBothDiscoverableStoresNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelState.EXPECT().GetAgentStream(gomock.Any()).Return(agentbinary.AgentStreamTesting, nil)
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.1"),
		Arch:   "amd64",
	}

	s.mockAgentBinaryDiscoverableStore1.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		nil, 0, "", agentbinaryerrors.NotFound,
	)

	s.mockAgentBinaryDiscoverableStore2.EXPECT().GetAgentBinaryWithSHA256(gomock.Any(), ver, agentbinary.AgentStreamTesting).Return(
		nil, 0, "", agentbinaryerrors.NotFound,
	)

	svc := NewAgentBinaryService(
		func(context.Context) (ProviderForAgentBinaryFinder, error) {
			return s.mockProvider, nil
		},
		s.getDefaultPreferredSimpleStreams(c), s.getDefaultAgentBinaryFilter(c),
		s.mockControllerState, s.mockModelState,
		s.mockAgentBinaryLocalStore, s.mockAgentBinaryDiscoverableStore1, s.mockAgentBinaryDiscoverableStore2,
	)

	data, size, _, err := svc.GetExternalAgentBinary(c.Context(), ver)
	c.Assert(err, tc.ErrorIs, agentbinaryerrors.NotFound)
	c.Assert(err, tc.ErrorMatches, `agent binary "4.0.1-amd64" does not exist in external stores: agent binary not found`)
	c.Assert(data, tc.IsNil)
	c.Assert(size, tc.Equals, int64(0))
}

// getDefaultAgentBinaryFilter returns a test helper that simulates an
// agent-binary filtering function used by the AgentBinaryService. It asserts
// that the expected major/minor version, stream list, and filter arguments are
// provided, and returns a predefined tools.List containing a single
// "4.0.1-ubuntu-amd64" entry.
//
// This function is intended for use as a default in unit tests to verify
// correct invocation of the agent-binary filter logic until it is removed soon.
// TODO(Alvin): Remove this after we remove agentBinaryFilter from service
func (s *serviceSuite) getDefaultAgentBinaryFilter(c *tc.C) func(
	_ context.Context,
	_ envtools.SimplestreamsFetcher,
	_ environs.BootstrapEnviron,
	majorVersion,
	minorVersion int,
	streams []string,
	filter tools.Filter,
) (tools.List, error) {
	return func(
		_ context.Context,
		_ envtools.SimplestreamsFetcher,
		_ environs.BootstrapEnviron,
		majorVersion,
		minorVersion int,
		streams []string,
		filter tools.Filter,
	) (tools.List, error) {
		c.Assert(majorVersion, tc.Equals, 4)
		c.Assert(minorVersion, tc.Equals, 0)
		c.Assert(streams, tc.DeepEquals, []string{"testing"})
		c.Assert(filter, tc.DeepEquals, tools.Filter{Arch: "amd64"})
		return tools.List{
			{Version: semversion.MustParseBinary("4.0.1-ubuntu-amd64")},
		}, nil
	}
}

// getDefaultPreferredSimpleStreams returns a test helper function that mimics the
// behavior of a preferred simple streams resolver used by the AgentBinaryService.
// The returned closure asserts that the version, development flag, and stream
// parameters match expected test values, and then returns a fixed slice containing
// the "testing" stream.
//
// This function is intended for use as a default in unit tests to verify
// correct invocation of the agent-binary preferred simple streams logic until it is removed soon.
// TODO(Alvin): Remove this after we remove preferred simple streams from service
func (s *serviceSuite) getDefaultPreferredSimpleStreams(c *tc.C) func(
	vers *semversion.Number,
	forceDevel bool,
	stream string,
) []string {
	return func(
		vers *semversion.Number,
		forceDevel bool,
		stream string,
	) []string {
		c.Assert(*vers, tc.DeepEquals, semversion.MustParse("4.0.1"))
		c.Assert(forceDevel, tc.Equals, true)
		c.Assert(stream, tc.Equals, "testing")
		return []string{"testing"}
	}
}
