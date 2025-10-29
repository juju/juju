// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

// AgentFinderControllerState defines the interface for interacting with the
// controller state.
type AgentFinderControllerState interface {
	// GetControllerTargetVersion returns the target controller version in use by the
	// cluster.
	GetControllerTargetVersion(ctx context.Context) (semversion.Number, error)

	// HasAgentBinariesForVersionArchitecturesAndStream returns whether the agents
	// are supported for the given version, architectures, and stream.
	HasAgentBinariesForVersionArchitecturesAndStream(
		context context.Context,
		version semversion.Number,
		architectures []agentbinary.Architecture,
		stream agentbinary.Stream,
	) (map[agentbinary.Architecture]bool, error)

	// GetAgentVersionsWithStream is responsible for searching the available
	// agent versions that are cached in the controller.
	GetAgentVersionsWithStream(
		ctx context.Context,
		stream *agentbinary.Stream,
	) ([]semversion.Number, error)
}

// AgentFinderControllerModelState defines the interface for interacting with the
// underlying model that hosts the current controller(s).
type AgentFinderControllerModelState interface {
	// HasAgentBinariesForVersionAndArchitectures returns whether the agents
	// are supported for the given version and architectures.
	HasAgentBinariesForVersionAndArchitectures(
		context context.Context,
		version semversion.Number,
		architectures []agentbinary.Architecture,
	) (map[agentbinary.Architecture]bool, error)

	// GetModelAgentStream returns the currently used stream for the agent.
	GetModelAgentStream(ctx context.Context) (agentbinary.Stream, error)

	// GetAgentVersionsWithStream is responsible for searching the available
	// agent versions that are cached in the model.
	GetAgentVersionsWithStream(
		ctx context.Context,
		stream *agentbinary.Stream,
	) ([]semversion.Number, error)
}

type GetPreferredSimpleStreamsFunc func(
	vers *semversion.Number,
	forceDevel bool,
	stream string,
) []string

type AgentBinaryFilterFunc func(
	ctx context.Context,
	ss envtools.SimplestreamsFetcher,
	env environs.BootstrapEnviron,
	majorVersion,
	minorVersion int,
	streams []string,
	filter coretools.Filter,
) (coretools.List, error)

type GetProviderFunc func(ctx context.Context) (environs.BootstrapEnviron, error)

// SimpleStreamsAgentFinder provides helper methods for fetching
// the binaries from simplestreams. This abstraction helps us mock the dependencies in tests.
type SimpleStreamsAgentFinder interface {
	// GetPreferredSimpleStreams returns the preferred streams for the given version and stream.
	GetPreferredSimpleStreams(
		vers *semversion.Number,
		forceDevel bool,
		stream string,
	) []string

	// AgentBinaryFilter filters agent binaries based on the given parameters.
	// It returns a list of agent binaries that match the filter criteria.
	AgentBinaryFilter(
		ctx context.Context,
		ss envtools.SimplestreamsFetcher,
		env environs.BootstrapEnviron,
		majorVersion,
		minorVersion int,
		streams []string,
		filter coretools.Filter,
	) (coretools.List, error)

	GetProvider(ctx context.Context) (environs.BootstrapEnviron, error)
}

// AgentFinder is a wrapper to expose helper functions for fetching
// the binaries from simplestreams. It conforms to [SimpleStreamsAgentFinder].
type AgentFinder struct {
	GetPreferredSimpleStreamsFn GetPreferredSimpleStreamsFunc
	AgentBinaryFilterFn         AgentBinaryFilterFunc
	GetProviderFn               GetProviderFunc
}

// NewAgentFinder returns a new AgentFinder that exposes helper methods.
func NewAgentFinder(
	getPreferredSimpleStreamsFn GetPreferredSimpleStreamsFunc,
	agentBinaryFilterFn AgentBinaryFilterFunc,
	getProviderFn GetProviderFunc,
) *AgentFinder {
	return &AgentFinder{
		GetPreferredSimpleStreamsFn: getPreferredSimpleStreamsFn,
		AgentBinaryFilterFn:         agentBinaryFilterFn,
		GetProviderFn:               getProviderFn,
	}
}

// GetPreferredSimpleStreams returns the streams to use when fetching the agents from simplestreams.
func (a AgentFinder) GetPreferredSimpleStreams(vers *semversion.Number, forceDevel bool, stream string) []string {
	return a.GetPreferredSimpleStreamsFn(vers, forceDevel, stream)
}

// AgentBinaryFilter returns the agents from the provided streams that match the
// supplied major and minor versions. It further narrows the results using the given filter.
func (a AgentFinder) AgentBinaryFilter(ctx context.Context, ss envtools.SimplestreamsFetcher, env environs.BootstrapEnviron, majorVersion, minorVersion int, streams []string, filter coretools.Filter) (coretools.List, error) {
	return a.AgentBinaryFilterFn(ctx, ss, env, majorVersion, minorVersion, streams, filter)
}

// GetProvider returns a provider.
func (a AgentFinder) GetProvider(ctx context.Context) (environs.BootstrapEnviron, error) {
	return a.GetProviderFn(ctx)
}

// StreamAgentBinaryFinder exposes helper methods for fetching
// the binaries from simplestreams.
type StreamAgentBinaryFinder struct {
	ctrlSt  AgentFinderControllerState
	modelSt AgentFinderControllerModelState

	agentFinder SimpleStreamsAgentFinder
}

// NewStreamAgentBinaryFinder returns a new StreamAgentBinaryFinder to assist
// in looking up agent binaries.
func NewStreamAgentBinaryFinder(
	ctrlSt AgentFinderControllerState,
	modelSt AgentFinderControllerModelState,
	agentFinder SimpleStreamsAgentFinder,
) *StreamAgentBinaryFinder {
	return &StreamAgentBinaryFinder{
		ctrlSt:      ctrlSt,
		modelSt:     modelSt,
		agentFinder: agentFinder,
	}
}

// getMissingArchitectures narrows down the given architectures that are missing.
func (a *StreamAgentBinaryFinder) getMissingArchitectures(
	architectures map[agentbinary.Architecture]bool,
) []agentbinary.Architecture {
	missingArchs := make([]agentbinary.Architecture, 0)
	for arch, exist := range architectures {
		if !exist {
			missingArchs = append(missingArchs, arch)
		}
	}
	return missingArchs
}

// getBinaryAgentInStreams returns a slice of tools that matches the given version, stream, and filter.
// If a stream is passed as nil, it will fallback to use default streams.
func (a *StreamAgentBinaryFinder) getBinaryAgentInStreams(
	ctx context.Context,
	version semversion.Number,
	stream *agentbinary.Stream,
	filter coretools.Filter,
) (coretools.List, error) {
	provider, err := a.agentFinder.GetProvider(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return coretools.List{}, errors.Errorf("getting provider for agent binary finder %w", coreerrors.NotSupported)
	}
	if err != nil {
		return coretools.List{}, errors.Capture(err)
	}

	var streams []string

	if stream == nil {
		cfg := provider.Config()
		streams = a.agentFinder.GetPreferredSimpleStreams(&version, cfg.Development(), cfg.AgentStream())
	} else {
		streams = []string{stream.String()}
	}

	ssFetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	tools, err := a.agentFinder.AgentBinaryFilter(ctx, ssFetcher, provider, version.Major, version.Minor, streams, filter)
	if err != nil {
		return coretools.List{}, errors.Errorf("getting agent binary from simplestreams: %w", err)
	}

	return tools, nil
}

// getHighestPatchVersionAvailableForStream returns a version with the highest patch given a stream.
// It grabs the current version from the controller state in which it is used to get the binaries
// matching the major and minor number of that version. It sorts the versions by the patch and
// returns the highest patch.
func (a *StreamAgentBinaryFinder) getHighestPatchVersionAvailableForStream(
	ctx context.Context,
	stream *agentbinary.Stream,
) (semversion.Number, error) {
	ctrlVersion, err := a.ctrlSt.GetControllerTargetVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}
	binaries, err := a.getBinaryAgentInStreams(ctx, ctrlVersion, stream, coretools.Filter{})
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}
	// If binaries are empty an error is returned above, but perform this check to be safe.
	if len(binaries) == 0 {
		return semversion.Zero, errors.Errorf("no binary agent found for version %s", ctrlVersion.String())
	}

	slices.SortStableFunc(binaries, func(a, b *coretools.Tools) int {
		return a.Version.ToPatch().Compare(b.Version.ToPatch())
	})
	highestPatchVersion := binaries[len(binaries)-1].Version.Number
	return highestPatchVersion, nil
}

// hasBinaryAgentInStreams consults simplestreams to check whether a binary agent with the given version, stream, and filter exists.
// We know it exists when there is at least one agent returned from simplestreams.
func (a *StreamAgentBinaryFinder) hasBinaryAgentInStreams(
	ctx context.Context,
	number semversion.Number,
	stream *agentbinary.Stream,
	filter coretools.Filter,
) (bool, error) {
	tools, err := a.getBinaryAgentInStreams(ctx, number, stream, filter)
	if err != nil {
		return false, errors.Capture(err)
	}
	return tools.Len() > 0, nil
}

// HasBinariesForVersionAndArchitectures returns true if an agent exists for a given
// version and architectures. It shares an implementation with HasBinariesForVersionStreamAndArchitectures
// but rather than forcing the client to supply a stream for this top level function,
// we use the currently model agent stream to supply to HasBinariesForVersionStreamAndArchitectures.
// Return false otherwise.
func (a *StreamAgentBinaryFinder) HasBinariesForVersionAndArchitectures(
	ctx context.Context,
	version semversion.Number,
	architectures []agentbinary.Architecture,
) (bool, error) {
	stream, err := a.modelSt.GetModelAgentStream(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	return a.HasBinariesForVersionStreamAndArchitectures(ctx, version, stream, architectures)
}

// HasBinariesForVersionStreamAndArchitectures consults three source of truths to check
// that an agent exists for a given version, stream, and architectures. First if the given
// stream matches with the current stream in use for the model agent, it consults
// the model DB if the agent exists.
// If it doesn't exist in the model DB, it then consults the missing agent(s) in the controller DB.
// In the unfortunate circumstances that the controller DB doesn't store them, we resort to
// consulting to simplestreams.
func (a *StreamAgentBinaryFinder) HasBinariesForVersionStreamAndArchitectures(
	ctx context.Context,
	version semversion.Number,
	stream agentbinary.Stream,
	architectures []agentbinary.Architecture,
) (bool, error) {
	streamInModel, err := a.modelSt.GetModelAgentStream(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}
	// If the supplied stream matches with the one in the model state, we can consult the
	// model state for the binaries.
	// Otherwise, we have to look for the binaries in the controller state.
	hasStream := streamInModel == stream
	missingArchsInModel := architectures
	if hasStream {
		archsInModel, err := a.modelSt.HasAgentBinariesForVersionAndArchitectures(ctx, version, architectures)
		if err != nil {
			return false, errors.Capture(err)
		}

		missingArchsInModel = a.getMissingArchitectures(archsInModel)
		if len(missingArchsInModel) == 0 {
			return true, nil
		}
	}

	// The stream in the model state doesn't match the given stream OR
	// some binaries don't exist in model state so we check if
	// the missing ones exist in controller state.
	archsInController, err := a.ctrlSt.HasAgentBinariesForVersionArchitecturesAndStream(ctx, version, missingArchsInModel, stream)
	if err != nil {
		return false, errors.Capture(err)
	}
	missingArchsInController := a.getMissingArchitectures(archsInController)
	if len(missingArchsInController) == 0 {
		return true, nil
	}

	// Sort it to have a stable ordering in tests.
	slices.SortStableFunc(missingArchsInController, func(a, b agentbinary.Architecture) int {
		if a.String() < b.String() {
			return -1
		} else if a.String() == b.String() {
			return 0
		}
		return 1
	})

	// Woops, now we fall back to finding the agent binary in simplestreams because
	// some binaries don't exist in both model and controller state.
	for _, arch := range missingArchsInController {
		filter := coretools.Filter{Number: version, Arch: arch.String()}
		found, err := a.hasBinaryAgentInStreams(ctx, version, &stream, filter)
		if err != nil {
			return false, errors.Capture(err)
		}
		if !found {
			return false, nil
		}
	}

	return true, nil
}

// GetHighestPatchVersionAvailable returns the highest patch version of the current controller.
func (a *StreamAgentBinaryFinder) GetHighestPatchVersionAvailable(ctx context.Context) (semversion.Number, error) {
	return a.getHighestPatchVersionAvailableForStream(ctx, nil)
}

// GetHighestPatchVersionAvailableForStream returns the highest patch version of the current
// controller given a stream.
func (a *StreamAgentBinaryFinder) GetHighestPatchVersionAvailableForStream(
	ctx context.Context,
	stream agentbinary.Stream,
) (semversion.Number, error) {
	return a.getHighestPatchVersionAvailableForStream(ctx, &stream)
}
