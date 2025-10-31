// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/internal/testhelpers"
	coretools "github.com/juju/juju/internal/tools"
)

var (
	released            = domainagentbinary.AgentStreamReleased
	agentStreamReleased = &released
)

type agentFinderSuite struct {
	testhelpers.IsolationSuite

	ctrlSt                    *MockAgentFinderControllerState
	modelSt                   *MockAgentFinderControllerModelState
	ssAgentFinder             *MockAgentFinder
	ctrlModelCacheAgentFinder *MockAgentFinder
	bootstrapEnv              *MockBootstrapEnviron
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
	s.ssAgentFinder = NewMockAgentFinder(ctrl)
	s.ctrlModelCacheAgentFinder = NewMockAgentFinder(ctrl)
	s.bootstrapEnv = NewMockBootstrapEnviron(ctrl)

	s.ssAgentFinder.EXPECT().Name().Return("SimpleStreamsAgentFinder")
	s.ctrlModelCacheAgentFinder.EXPECT().Name().
		Return("ControllerAndModelCacheAgentFinder")

	c.Cleanup(func() {
		s.ctrlSt = nil
		s.modelSt = nil
		s.ssAgentFinder = nil
		s.ctrlModelCacheAgentFinder = nil
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
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().HasAgentBinariesForVersionAndArchitectures(gomock.Any(), version, architectures).
		Return(map[domainagentbinary.Architecture]bool{domainagentbinary.AMD64: true}, nil)

	ok, err := binaryFinder.HasBinariesForVersionAndArchitectures(
		c.Context(),
		version,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
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
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.ARM64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().HasAgentBinariesForVersionAndArchitectures(gomock.Any(), version, architectures).
		Return(map[domainagentbinary.Architecture]bool{
			domainagentbinary.AMD64: true,
			domainagentbinary.ARM64: false,
		}, nil)
	s.ctrlSt.EXPECT().HasAgentBinariesForVersionArchitecturesAndStream(
		gomock.Any(),
		version,
		[]domainagentbinary.Architecture{domainagentbinary.ARM64},
		domainagentbinary.AgentStreamReleased,
	).Return(map[domainagentbinary.Architecture]bool{
		domainagentbinary.ARM64: true,
	}, nil)

	ok, err := binaryFinder.HasBinariesForVersionAndArchitectures(
		c.Context(),
		version,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

// TestHasBinariesForVersionAndArchitecturesFallbackToSimpleStreams tests fetching the agents
// from all three sources because the agent doesn't exist in both model and controller DB so it falls
// back to simple streams.
func (s *agentFinderSuite) TestHasBinariesForVersionAndArchitecturesFallbackToSimpleStreams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.ARM64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	// Model state doesn't have the agents we're looking for.
	s.modelSt.EXPECT().HasAgentBinariesForVersionAndArchitectures(gomock.Any(), version, architectures).
		Return(map[domainagentbinary.Architecture]bool{
			domainagentbinary.AMD64: false,
			domainagentbinary.ARM64: false,
		}, nil)
	// Sadly the controller state doesn't have them as well.
	s.ctrlSt.EXPECT().HasAgentBinariesForVersionArchitecturesAndStream(
		gomock.Any(),
		version,
		gomock.InAnyOrder(architectures),
		domainagentbinary.AgentStreamReleased,
	).
		Return(map[domainagentbinary.Architecture]bool{
			domainagentbinary.AMD64: false,
			domainagentbinary.ARM64: false,
		}, nil)

	// We now have to resort to simplestreams.
	gomock.InOrder(
		// Look for amd64 agent.
		s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
			agentStreamReleased, coretools.Filter{Number: version, Arch: "amd64"}).
			Return([]semversion.Number{version}, nil),

		// Look for arm64 agent.
		s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
			agentStreamReleased, coretools.Filter{Number: version, Arch: "arm64"}).
			Return([]semversion.Number{version}, nil),
	)

	ok, err := binaryFinder.HasBinariesForVersionAndArchitectures(
		c.Context(),
		version,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

// TestHasBinariesForVersionAndArchitecturesNoneAvailable tests the unfortunate circumstance
// when the agent doesn't exist in all three source of truths. It returns false without any errors.
func (s *agentFinderSuite) TestHasBinariesForVersionAndArchitecturesNoneAvailable(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	// Model state doesn't have the agents we're looking for.
	s.modelSt.EXPECT().HasAgentBinariesForVersionAndArchitectures(gomock.Any(), version, architectures).
		Return(map[domainagentbinary.Architecture]bool{
			domainagentbinary.AMD64: false,
		}, nil)
	// Sadly the controller state doesn't have them as well.
	s.ctrlSt.EXPECT().HasAgentBinariesForVersionArchitecturesAndStream(
		gomock.Any(),
		version,
		gomock.InAnyOrder(architectures),
		domainagentbinary.AgentStreamReleased,
	).Return(map[domainagentbinary.Architecture]bool{
		domainagentbinary.AMD64: false,
	}, nil)

	// We now have to resort to simplestreams.
	// Look for amd64 agent. Unfortunately, it doesn't exist here.
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{Number: version, Arch: "amd64"}).
		Return([]semversion.Number{}, nil)

	ok, err := binaryFinder.HasBinariesForVersionAndArchitectures(
		c.Context(),
		version,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsFalse)
}

// TestHasBinariesForVersionStreamAndArchitectures is similar to TestHasBinariesForVersionAndArchitectures
// but here we supply a stream in the function under test.
func (s *agentFinderSuite) TestHasBinariesForVersionStreamAndArchitectures(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().HasAgentBinariesForVersionAndArchitectures(gomock.Any(), version, architectures).
		Return(map[domainagentbinary.Architecture]bool{domainagentbinary.AMD64: true}, nil)

	ok, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

// TestHasBinariesForVersionStreamAndArchitecturesWithMissingArchsInModel is similar to
// TestHasBinariesForVersionAndArchitecturesWithMissingArchsInModel but here we supply a stream
// in the function under test.
func (s *agentFinderSuite) TestHasBinariesForVersionStreamAndArchitecturesWithMissingArchsInModel(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.ARM64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	s.modelSt.EXPECT().HasAgentBinariesForVersionAndArchitectures(gomock.Any(), version, architectures).
		Return(map[domainagentbinary.Architecture]bool{
			domainagentbinary.AMD64: true,
			domainagentbinary.ARM64: false,
		}, nil)
	s.ctrlSt.EXPECT().HasAgentBinariesForVersionArchitecturesAndStream(
		gomock.Any(),
		version,
		[]domainagentbinary.Architecture{domainagentbinary.ARM64},
		domainagentbinary.AgentStreamReleased,
	).Return(map[domainagentbinary.Architecture]bool{
		domainagentbinary.ARM64: true,
	}, nil)

	ok, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

// TestHasBinariesForVersionStreamAndArchitecturesWithDifferentStream tests fetching the agents
// from controller DB because the supplied stream is different to the stream in use.
func (s *agentFinderSuite) TestHasBinariesForVersionStreamAndArchitecturesWithDifferentStream(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.ARM64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamDevel, nil)
	s.ctrlSt.EXPECT().HasAgentBinariesForVersionArchitecturesAndStream(
		gomock.Any(),
		version,
		gomock.InAnyOrder(architectures),
		domainagentbinary.AgentStreamReleased,
	).Return(map[domainagentbinary.Architecture]bool{
		domainagentbinary.AMD64: true,
		domainagentbinary.ARM64: true,
	}, nil)

	ok, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

// TestHasBinariesForVersionStreamAndArchitecturesFallbackToSimpleStreams is similar to
// TestHasBinariesForVersionAndArchitecturesFallbackToSimpleStreams but here we supply a stream
// in the function under test.
func (s *agentFinderSuite) TestHasBinariesForVersionStreamAndArchitecturesFallbackToSimpleStreams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64, domainagentbinary.ARM64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	// Model state doesn't have the agents we're looking for.
	s.modelSt.EXPECT().HasAgentBinariesForVersionAndArchitectures(gomock.Any(), version, gomock.InAnyOrder(architectures)).
		Return(map[domainagentbinary.Architecture]bool{
			domainagentbinary.AMD64: false,
			domainagentbinary.ARM64: false,
		}, nil)
	// Sadly the controller state doesn't have them as well.
	s.ctrlSt.EXPECT().HasAgentBinariesForVersionArchitecturesAndStream(
		gomock.Any(),
		version,
		gomock.InAnyOrder(architectures),
		domainagentbinary.AgentStreamReleased,
	).Return(map[domainagentbinary.Architecture]bool{
		domainagentbinary.AMD64: false,
		domainagentbinary.ARM64: false,
	}, nil)

	// We now have to resort to simplestreams.
	gomock.InOrder(
		// Look for amd64 agent.
		s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
			agentStreamReleased, coretools.Filter{Number: version, Arch: "amd64"}).
			Return([]semversion.Number{version}, nil),

		// Look for arm64 agent.
		s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
			agentStreamReleased, coretools.Filter{Number: version, Arch: "arm64"}).
			Return([]semversion.Number{version}, nil),
	)

	ok, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsTrue)
}

// TestTestHasBinariesForVersionStreamAndArchitecturesNoneAvailable is similar to
// TestHasBinariesForVersionAndArchitecturesNoneAvailable but here we supply a stream
// in the function under test.
func (s *agentFinderSuite) TestTestHasBinariesForVersionStreamAndArchitecturesNoneAvailable(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)

	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)
	architectures := []domainagentbinary.Architecture{domainagentbinary.AMD64}

	s.modelSt.EXPECT().GetModelAgentStream(gomock.Any()).
		Return(domainagentbinary.AgentStreamReleased, nil)
	// Model state doesn't have the agents we're looking for.
	s.modelSt.EXPECT().HasAgentBinariesForVersionAndArchitectures(gomock.Any(), version, architectures).
		Return(map[domainagentbinary.Architecture]bool{
			domainagentbinary.AMD64: false,
		}, nil)
	// Sadly the controller state doesn't have them as well.
	s.ctrlSt.EXPECT().HasAgentBinariesForVersionArchitecturesAndStream(
		gomock.Any(),
		version,
		gomock.InAnyOrder(architectures),
		domainagentbinary.AgentStreamReleased,
	).Return(map[domainagentbinary.Architecture]bool{
		domainagentbinary.AMD64: false,
	}, nil)

	// We now have to resort to simplestreams.
	// Look for amd64 agent. Unfortunately, it doesn't exist here.
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{Number: version, Arch: "amd64"}).
		Return([]semversion.Number{}, nil)

	ok, err := binaryFinder.HasBinariesForVersionStreamAndArchitectures(
		c.Context(),
		version,
		domainagentbinary.AgentStreamReleased,
		architectures,
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ok, tc.IsFalse)
}

// TestGetHighestPatchVersionAvailable tests getting the highest patch version.
// When simplestreams return multiple versions, our function will pick
// the highest patch one.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailable(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)
	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	anotherVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	highestVersion, err := semversion.Parse("4.0.10")
	c.Assert(err, tc.ErrorIsNil)
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		nil, coretools.Filter{}).Return(
		[]semversion.Number{
			version,
			// This one is the highest patch version.
			highestVersion,
			anotherVersion,
		}, nil)
	s.ctrlModelCacheAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		nil, coretools.Filter{}).Return(
		[]semversion.Number{
			version,
		}, nil)

	highestPatch, err := binaryFinder.GetHighestPatchVersionAvailable(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(highestPatch, tc.DeepEquals, highestVersion)
}

// TestGetHighestPatchVersionAvailableFromSimpleStreams tests getting the highest patch version
// from simplestreams when controllerandmodel cache returns empty.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailableFromSimpleStreams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)
	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	anotherVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	highestVersion, err := semversion.Parse("4.0.10")
	c.Assert(err, tc.ErrorIsNil)
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		nil, coretools.Filter{}).Return(
		[]semversion.Number{
			version,
			// This one is the highest patch version.
			highestVersion,
			anotherVersion,
		}, nil)
	s.ctrlModelCacheAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		nil, coretools.Filter{}).Return(
		[]semversion.Number{}, nil)

	highestPatch, err := binaryFinder.GetHighestPatchVersionAvailable(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(highestPatch, tc.DeepEquals, highestVersion)
}

// TestGetHighestPatchVersionAvailableFromCache tests getting the highest patch version
// from controllerandmodel cache when simplestreams returns empty.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailableFromCache(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)
	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	anotherVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	highestVersion, err := semversion.Parse("4.0.10")
	c.Assert(err, tc.ErrorIsNil)
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		nil, coretools.Filter{}).Return(
		[]semversion.Number{}, nil)
	s.ctrlModelCacheAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		nil, coretools.Filter{}).Return(
		[]semversion.Number{
			version,
			// This one is the highest patch version.
			highestVersion,
			anotherVersion,
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
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)
	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(
		gomock.Any(), version, nil, coretools.Filter{},
	).Return([]semversion.Number{}, nil)
	s.ctrlModelCacheAgentFinder.EXPECT().SearchForAgentVersions(
		gomock.Any(), version, nil, coretools.Filter{},
	).Return([]semversion.Number{}, nil)

	_, err = binaryFinder.GetHighestPatchVersionAvailable(c.Context())
	c.Assert(err, tc.ErrorMatches, "no binary agent found for version 4.0.7")
}

// TestGetHighestPatchVersionAvailableForStream is similar to TestGetHighestPatchVersionAvailable
// but here we supply a stream in the function under test.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailableForStream(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)
	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	anotherVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	highestVersion, err := semversion.Parse("4.0.10")
	c.Assert(err, tc.ErrorIsNil)
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{}).
		Return([]semversion.Number{version, highestVersion, anotherVersion}, nil)
	s.ctrlModelCacheAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{}).
		Return([]semversion.Number{anotherVersion}, nil)

	highestPatch, err := binaryFinder.GetHighestPatchVersionAvailableForStream(c.Context(), domainagentbinary.AgentStreamReleased)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(highestPatch, tc.DeepEquals, highestVersion)
}

// TestGetHighestPatchVersionAvailableForStreamFromSimpleStreams is similar to
// TestGetHighestPatchVersionAvailableFromSimpleStreams
// but here we supply a stream in the function under test.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailableForStreamFromSimpleStreams(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)
	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	anotherVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	highestVersion, err := semversion.Parse("4.0.10")
	c.Assert(err, tc.ErrorIsNil)
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{}).
		Return([]semversion.Number{version, highestVersion, anotherVersion}, nil)
	s.ctrlModelCacheAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{}).
		Return([]semversion.Number{}, nil)

	highestPatch, err := binaryFinder.GetHighestPatchVersionAvailableForStream(c.Context(), domainagentbinary.AgentStreamReleased)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(highestPatch, tc.DeepEquals, highestVersion)
}

// TestGetHighestPatchVersionAvailableForStreamFromCache is similar to
// TestGetHighestPatchVersionAvailableFromCache
// but here we supply a stream in the function under test.
func (s *agentFinderSuite) TestGetHighestPatchVersionAvailableForStreamFromCache(c *tc.C) {
	defer s.setupMocks(c).Finish()
	binaryFinder := NewStreamAgentBinaryFinder(
		s.ctrlSt,
		s.modelSt,
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)
	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	anotherVersion, err := semversion.Parse("4.0.9")
	c.Assert(err, tc.ErrorIsNil)
	highestVersion, err := semversion.Parse("4.0.10")
	c.Assert(err, tc.ErrorIsNil)
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{}).
		Return([]semversion.Number{}, nil)
	s.ctrlModelCacheAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{}).
		Return([]semversion.Number{version, highestVersion, anotherVersion}, nil)

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
		s.ssAgentFinder,
		s.ctrlModelCacheAgentFinder,
	)
	version, err := semversion.Parse("4.0.7")
	c.Assert(err, tc.ErrorIsNil)

	s.ctrlSt.EXPECT().GetControllerTargetVersion(gomock.Any()).
		Return(version, nil)
	s.ssAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{}).
		Return([]semversion.Number{}, nil)
	s.ctrlModelCacheAgentFinder.EXPECT().SearchForAgentVersions(gomock.Any(), version,
		agentStreamReleased, coretools.Filter{}).
		Return([]semversion.Number{}, nil)

	_, err = binaryFinder.GetHighestPatchVersionAvailableForStream(c.Context(), domainagentbinary.AgentStreamReleased)
	c.Assert(err, tc.ErrorMatches, "no binary agent found for version 4.0.7")
}
