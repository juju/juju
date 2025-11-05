// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	domainagentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type agentFinderSuite struct {
	testhelpers.IsolationSuite

	ctrlSt  *MockAgentFinderControllerState
	modelSt *MockAgentFinderControllerModelState

	ctrlStore         *MockAgentBinaryQuerierStore
	simplestreamStore *MockAgentBinaryQuerierStore

	bootstrapEnv *MockBootstrapEnviron
}

// TestAgentFinderSuite runs the test methods in agentFinderSuite.
func TestAgentFinderSuite(t *testing.T) {
	tc.Run(t, &agentFinderSuite{})
}

// setupMocks instantiates the mocked dependencies.
func (s *agentFinderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.ctrlSt = NewMockAgentFinderControllerState(ctrl)
	s.modelSt = NewMockAgentFinderControllerModelState(ctrl)
	s.ctrlStore = NewMockAgentBinaryQuerierStore(ctrl)
	s.simplestreamStore = NewMockAgentBinaryQuerierStore(ctrl)
	s.bootstrapEnv = NewMockBootstrapEnviron(ctrl)

	c.Cleanup(func() {
		s.ctrlSt = nil
		s.modelSt = nil
		s.ctrlStore = nil
		s.simplestreamStore = nil
		s.bootstrapEnv = nil
	})

	return ctrl
}

// TestHasBinariesForVersionAndArchitectures tests determining an agent
// exists with the supplied params without any errors.
func (s *agentFinderSuite) TestHasBinariesForVersionAndArchitectures(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(
		gomock.Any(), domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	has, err := binaryFinder.HasBinariesForVersionAndArchitectures(
		c.Context(),
		version,
		[]domainagentbinary.Architecture{
			domainagentbinary.AMD64,
			domainagentbinary.ARM64,
			domainagentbinary.S390X,
		},
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(has, tc.IsTrue)
}

// TestHasBinariesForVersionAndArchitectures tests determining an agent
// exists with the supplied params without any errors.
// An architecture doesn't exist in the model DB so it consults to find the missing one
// in the controller DB.
func (s *agentFinderSuite) TestHasBinariesForVersionAndArchitecturesWithMissingArchsInModel(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	// Model state is missing S390X.
	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(
		gomock.Any(), domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	s.ctrlStore.EXPECT().GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	has, err := binaryFinder.HasBinariesForVersionAndArchitectures(
		c.Context(),
		version,
		[]domainagentbinary.Architecture{
			domainagentbinary.AMD64,
			domainagentbinary.ARM64,
			domainagentbinary.S390X,
		},
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(has, tc.IsTrue)
}

// TestHasBinariesForVersionAndArchitecturesFallbackToSimpleStreams tests fetching the agents
// from all three sources because the agent doesn't exist in both model and controller DB so it falls
// back to simple streams.
func (s *agentFinderSuite) TestHasBinariesForVersionAndArchitecturesFallbackToSimpleStreams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	// Model state are missing arm64 and s390x archs.
	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(gomock.Any(),
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)
	// Sadly the controller store doesn't have s390x.
	s.ctrlStore.EXPECT().GetAvailableForVersionInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	// We now have to resort to simplestreams.
	s.simplestreamStore.EXPECT().GetAvailableForVersionInStream(gomock.Any(),
		version, domainagentbinary.AgentStreamReleased).
		Return([]domainagentbinary.AgentBinary{
			{
				Architecture: domainagentbinary.S390X,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
		}, nil)

	has, err := binaryFinder.HasBinariesForVersionAndArchitectures(
		c.Context(),
		version,
		[]domainagentbinary.Architecture{domainagentbinary.AMD64,
			domainagentbinary.ARM64, domainagentbinary.S390X},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsTrue)
}

// TestHasBinariesForVersionAndArchitecturesNoneAvailable tests the unfortunate
// circumstance when the agent doesn't exist in all three source of truths.
// It returns false without any errors.
func (s *agentFinderSuite) TestHasBinariesForVersionAndArchitecturesNoneAvailable(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)

	// Model state doesn't have amd64 arch we're looking for.
	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(gomock.Any(),
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.PPC64EL,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)
	// Sadly the controller state doesn't have it as well.
	s.ctrlStore.EXPECT().GetAvailableForVersionInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	// We now have to resort to simplestreams. Unfortunately, simplestream only has
	// s390x.
	s.simplestreamStore.EXPECT().GetAvailableForVersionInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	has, err := binaryFinder.HasBinariesForVersionAndArchitectures(
		c.Context(),
		version,
		[]domainagentbinary.Architecture{domainagentbinary.AMD64},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsFalse)
}

// TestHasBinariesForVersionStreamAndArchitectures is similar to
// [agentFinderSuite].TestHasBinariesForVersionAndArchitectures but here we
// supply a stream in the function under test.
func (s *agentFinderSuite) TestHasBinariesForVersionStreamAndArchitectures(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(
		gomock.Any(), domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	has, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		[]domainagentbinary.Architecture{
			domainagentbinary.AMD64,
			domainagentbinary.ARM64,
			domainagentbinary.S390X,
		},
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(has, tc.IsTrue)
}

// TestHasBinariesForVersionStreamAndArchitecturesWithMissingArchsInModel is similar to
// TestHasBinariesForVersionAndArchitecturesWithMissingArchsInModel but here we supply a stream
// in the function under test.
func (s *agentFinderSuite) TestHasBinariesForVersionStreamAndArchitecturesWithMissingArchsInModel(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	// Model state is missing S390X.
	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(
		gomock.Any(), domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	s.ctrlStore.EXPECT().GetAvailableForVersionInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
		{
			Architecture: domainagentbinary.S390X,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	has, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		[]domainagentbinary.Architecture{
			domainagentbinary.AMD64,
			domainagentbinary.ARM64,
			domainagentbinary.S390X,
		},
	)

	c.Check(err, tc.ErrorIsNil)
	c.Check(has, tc.IsTrue)
}

// TestHasBinariesForVersionStreamAndArchitecturesWithDifferentStream tests
// fetching the agents from controller DB because the model DB returned an empty
// slice which may happen if the supplied stream is different from what the model has.
func (s *agentFinderSuite) TestHasBinariesForVersionStreamAndArchitecturesWithDifferentStream(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(gomock.Any(),
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{},
		nil)
	s.ctrlStore.EXPECT().GetAvailableForVersionInStream(
		gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased,
	).Return(
		[]domainagentbinary.AgentBinary{
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
			{
				Architecture: domainagentbinary.ARM64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
		}, nil)

	has, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		[]domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.ARM64},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsTrue)
}

// TestHasBinariesForVersionStreamAndArchitecturesFallbackToSimpleStreams is similar to
// TestHasBinariesForVersionAndArchitecturesFallbackToSimpleStreams but here we supply a stream
// in the function under test.
func (s *agentFinderSuite) TestHasBinariesForVersionStreamAndArchitecturesFallbackToSimpleStreams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	// Model state doesn't have the agents we're looking for.
	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(gomock.Any(),
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.AMD64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)
	// Sadly the controller state doesn't have them as well.
	s.ctrlStore.EXPECT().GetAvailableForVersionInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	// We now have to resort to simplestreams.
	s.simplestreamStore.EXPECT().GetAvailableForVersionInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).
		Return([]domainagentbinary.AgentBinary{
			{
				Architecture: domainagentbinary.S390X,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
		}, nil)

	has, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		[]domainagentbinary.Architecture{domainagentbinary.AMD64,
			domainagentbinary.ARM64, domainagentbinary.S390X},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsTrue)
}

// TestTestHasBinariesForVersionStreamAndArchitecturesNoneAvailable is similar to
// TestHasBinariesForVersionAndArchitecturesNoneAvailable but here we supply a stream
// in the function under test.
func (s *agentFinderSuite) TestTestHasBinariesForVersionStreamAndArchitecturesNoneAvailable(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)

	version := tc.Must1(c, semversion.Parse, "4.0.7")

	// Model state doesn't have the agents we're looking for.
	s.modelSt.EXPECT().GetAllAgentStoreBinariesForStream(gomock.Any(),
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.PPC64EL,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)
	// Sadly the controller state doesn't have them as well.
	s.ctrlStore.EXPECT().GetAvailableForVersionInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{
		{
			Architecture: domainagentbinary.ARM64,
			Stream:       domainagentbinary.AgentStreamReleased,
			Version:      version,
		},
	}, nil)

	// We now have to resort to simplestreams.
	s.simplestreamStore.EXPECT().GetAvailableForVersionInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).
		Return([]domainagentbinary.AgentBinary{
			{
				Architecture: domainagentbinary.S390X,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
		}, nil)

	has, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		[]domainagentbinary.Architecture{domainagentbinary.AMD64},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(has, tc.IsFalse)
}

// TestGetHighestPatchVersionAvailable tests getting the highest patch version.
// When store binaries and simplestreams return multiple versions
// our function will pick the highest patch one.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailable(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)
	version := tc.Must1(c, semversion.Parse, "4.0.7")
	anotherVersion := tc.Must1(c, semversion.Parse, "4.0.9")
	highestVersion := tc.Must1(c, semversion.Parse, "4.0.10")

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	s.ctrlStore.EXPECT().GetAvailablePatchVersionsInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).
		Return([]domainagentbinary.AgentBinary{
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      anotherVersion,
			},
		}, nil)

	s.simplestreamStore.EXPECT().GetAvailablePatchVersionsInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).
		Return([]domainagentbinary.AgentBinary{
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      highestVersion,
			},
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      anotherVersion,
			},
		}, nil)

	highestPatch, err := binaryFinder.GetHighestPatchVersionAvailable(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(highestPatch, tc.DeepEquals, highestVersion)
}

// TestGetHighestPatchVersionAvailableNoBinariesFound tests returning an error when
// we cannot find an agent with the version that is currently in use.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailableNoBinariesFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)
	version := tc.Must1(c, semversion.Parse, "4.0.7")

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	s.ctrlStore.EXPECT().GetAvailablePatchVersionsInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{}, nil)
	s.simplestreamStore.EXPECT().GetAvailablePatchVersionsInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{}, nil)

	_, err := binaryFinder.GetHighestPatchVersionAvailable(c.Context())
	c.Assert(err, tc.ErrorIs, domainagentbinaryerrors.NotFound)
}

// TestGetHighestPatchVersionAvailableForStream is similar to TestGetHighestPatchVersionAvailable
// but here we supply a stream in the function under test.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailableForStream(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)
	version := tc.Must1(c, semversion.Parse, "4.0.7")
	anotherVersion := tc.Must1(c, semversion.Parse, "4.0.9")
	highestVersion := tc.Must1(c, semversion.Parse, "4.0.10")

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	s.ctrlStore.EXPECT().GetAvailablePatchVersionsInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).
		Return([]domainagentbinary.AgentBinary{
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      anotherVersion,
			},
		}, nil)

	s.simplestreamStore.EXPECT().GetAvailablePatchVersionsInStream(gomock.Any(),
		version, domainagentbinary.AgentStreamReleased).
		Return([]domainagentbinary.AgentBinary{
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      version,
			},
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      highestVersion,
			},
			{
				Architecture: domainagentbinary.AMD64,
				Stream:       domainagentbinary.AgentStreamReleased,
				Version:      anotherVersion,
			},
		}, nil)

	highestPatch, err := binaryFinder.GetHighestPatchVersionAvailableForStream(c.Context(), domainagentbinary.AgentStreamReleased)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(highestPatch, tc.DeepEquals, highestVersion)
}

// TestGetHighestPatchVersionAvailableForStreamNoBinariesFound is similar to
// TestGetHighestPatchVersionAvailableNoBinariesFound but here we supply a stream in the function under test.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailableForStreamNoBinariesFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ctrlStore,
		s.simplestreamStore,
	)
	version := tc.Must1(c, semversion.Parse, "4.0.7")

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	s.ctrlStore.EXPECT().GetAvailablePatchVersionsInStream(gomock.Any(),
		version,
		domainagentbinary.AgentStreamReleased).Return([]domainagentbinary.AgentBinary{}, nil)
	s.simplestreamStore.EXPECT().GetAvailablePatchVersionsInStream(
		gomock.Any(), version, domainagentbinary.AgentStreamReleased,
	).Return([]domainagentbinary.AgentBinary{}, nil)

	_, err := binaryFinder.GetHighestPatchVersionAvailableForStream(c.Context(), domainagentbinary.AgentStreamReleased)
	c.Assert(err, tc.ErrorIs, domainagentbinaryerrors.NotFound)
}
