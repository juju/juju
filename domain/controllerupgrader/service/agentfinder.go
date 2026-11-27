// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	domainagentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/domain/controllerupgrader/internal"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	internaltools "github.com/juju/juju/internal/tools"
)

// AgentFinderControllerState defines the interface for interacting with the
// controller state.
type AgentFinderControllerState interface {
	// GetAllAgentStoreBinaries returns all binaries in the controller store,
	// which does not record stream membership.
	GetAllAgentStoreBinaries(ctx context.Context) ([]internal.AgentStoreBinary, error)

	// GetControllerTargetVersion returns the target controller version in use by the
	// cluster.
	GetControllerTargetVersion(ctx context.Context) (semversion.Number, error)
}

// AgentFinderControllerModelState defines the interface for interacting with the
// underlying model that hosts the current controller(s).
type AgentFinderControllerModelState interface {
	// GetAllAgentStoreBinariesForStream returns model store binaries when the
	// requested stream matches the model's target stream.
	GetAllAgentStoreBinariesForStream(
		context.Context, agentbinary.Stream,
	) ([]agentbinary.AgentBinary, error)

	// GetModelAgentStream returns the currently used stream for the agent.
	GetModelAgentStream(ctx context.Context) (agentbinary.Stream, error)
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
	filter internaltools.Filter,
) (internaltools.List, error)

type GetProviderFunc func(ctx context.Context) (environs.BootstrapEnviron, error)

// SimpleStreamsAgentFinder provides helper methods for fetching
// the binaries from simplestreams. This abstraction helps us mock the dependencies in tests.
type SimpleStreamsAgentFinder interface {
	// AgentBinaryFilter filters agent binaries based on the given parameters.
	// It returns a list of agent binaries that match the filter criteria.
	AgentBinaryFilter(
		ctx context.Context,
		ss envtools.SimplestreamsFetcher,
		env environs.BootstrapEnviron,
		majorVersion,
		minorVersion int,
		streams []string,
		filter internaltools.Filter,
	) (internaltools.List, error)

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

// AgentBinaryFilter returns the agents from the provided streams that match the
// supplied major and minor versions. It further narrows the results using the given filter.
func (a AgentFinder) AgentBinaryFilter(
	ctx context.Context, ss envtools.SimplestreamsFetcher, env environs.BootstrapEnviron, majorVersion, minorVersion int, streams []string, filter internaltools.Filter) (internaltools.List, error) {
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

// getSimpleStreamsPatchVersionsForController returns a slice of
// [agentbinary.AgentBinary]s that are available from simple streams that share
// the the same major and minor version as that of the supplied version.
func (a *StreamAgentBinaryFinder) getSimpleStreamsPatchVersions(
	ctx context.Context,
	version semversion.Number,
	stream agentbinary.Stream,
) ([]agentbinary.AgentBinary, error) {
	provider, err := a.agentFinder.GetProvider(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, errors.Errorf(
			"environ provider does not support simple streams",
		).Add(coreerrors.NotSupported)
	} else if err != nil {
		return nil, errors.Errorf(
			"getting environ provider for use with simple streams: %w", err,
		)
	}

	ssFetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	tools, err := a.agentFinder.AgentBinaryFilter(
		ctx,
		ssFetcher,
		provider,
		version.Major,
		version.Minor,
		[]string{stream.String()},
		internaltools.Filter{},
	)
	// If no tools exists in simple streams this is not an error.
	if err != nil && !errors.Is(err, internaltools.ErrNoMatches) {
		return nil, errors.Errorf(
			"getting simple streams agent binary patch versions for controller: %w",
			err,
		)
	}

	retVal := make([]agentbinary.AgentBinary, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			// The [internaltools.List] may contain nil pointers.
			continue
		}

		arch, converted := agentbinary.ArchitectureFromString(tool.Version.Arch)
		if !converted {
			// If we don't understand the architecture this result is thrown
			// away.
			continue
		}

		retVal = append(retVal, agentbinary.AgentBinary{
			Architecture: arch,
			Version:      tool.Version.Number,
			Stream:       stream,
		})
	}

	return retVal, nil
}

// getSimpleStreamsAgentBinariesForVersionStream consults simplestreams for all
// agent binary versions that exist for a given version and stream. If no agent
// binaries exist, an empty slice is returned.
func (a *StreamAgentBinaryFinder) getSimpleStreamsAgentBinariesForVersionStream(
	ctx context.Context,
	version semversion.Number,
	stream agentbinary.Stream,
) ([]agentbinary.AgentBinary, error) {
	provider, err := a.agentFinder.GetProvider(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, errors.Errorf(
			"environ provider does not support simple streams",
		).Add(coreerrors.NotSupported)
	} else if err != nil {
		return nil, errors.Errorf(
			"getting environ provider for use with simple streams: %w", err,
		)
	}

	ssFetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	tools, err := a.agentFinder.AgentBinaryFilter(
		ctx,
		ssFetcher,
		provider,
		version.Major,
		version.Minor,
		[]string{stream.String()},
		internaltools.Filter{
			Number: version,
		},
	)

	// If no tools exists in simple streams this is not an error.
	if err != nil && !errors.Is(err, internaltools.ErrNoMatches) {
		return nil, errors.Errorf(
			"getting simple streams agent binary patch versions for controller: %w",
			err,
		)
	}

	retVal := make([]agentbinary.AgentBinary, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			// The [internaltools.List] may contain nil pointers.
			continue
		}

		arch, converted := agentbinary.ArchitectureFromString(tool.Version.Arch)
		if !converted {
			// If we don't understand the architecture this result is thrown
			// away.
			continue
		}

		retVal = append(retVal, agentbinary.AgentBinary{
			Architecture: arch,
			Version:      tool.Version.Number,
			Stream:       stream,
		})
	}

	return retVal, nil
}

// HasBinariesForVersionAndArchitectures returns true if an agent binary exists
// for a given version and architectures. It shares an implementation with
// [StreamAgentBinaryFinder.HasBinariesForVersionStreamAndArchitectures]
// but rather than forcing the client to supply a stream for this top level
// function, we use the current controllers model agent stream.
func (a *StreamAgentBinaryFinder) HasBinariesForVersionAndArchitectures(
	ctx context.Context,
	version semversion.Number,
	architectures []agentbinary.Architecture,
) (bool, error) {
	stream, err := a.modelSt.GetModelAgentStream(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	return a.HasBinariesForVersionStreamAndArchitectures(
		ctx, version, stream, architectures,
	)
}

// HasBinariesForVersionStreamAndArchitectures consults
//
// three source of truths to check
// that an agent exists for a given version, stream, and architectures. First it
// consults the model DB if the agent exists.
// If there are missing architectures in the model DB, it then consults the
// missing architectures in the controller DB.
// In the unfortunate circumstances that the controller DB doesn't store them,
// we resort to consulting to simplestreams.
func (a *StreamAgentBinaryFinder) HasBinariesForVersionStreamAndArchitectures(
	ctx context.Context,
	version semversion.Number,
	stream agentbinary.Stream,
	architectures []agentbinary.Architecture,
) (bool, error) {
	if len(architectures) == 0 {
		// We can't find architectures for an empty slice.
		return false, nil
	}
	// Dedupe architectures.
	slices.Sort(architectures)
	architectures = slices.Compact(architectures)

	modelBinaries, err := a.modelSt.GetAllAgentStoreBinariesForStream(ctx, stream)
	if err != nil {
		return false, errors.Errorf(
			"getting available agent binaries in model for stream %q: %w",
			stream, err,
		)
	}

	// Filter out all agent binaries which are not for the version we care
	// about.
	modelBinaries = slices.DeleteFunc(
		modelBinaries, agentbinary.AgentBinaryNotMatchingVersion(version),
	)
	foundArchs := slices.AppendSeq(
		[]agentbinary.Architecture{},
		agentbinary.AgentBinaryArchitectures(modelBinaries),
	)
	architectures = agentbinary.ArchitectureNotIn(architectures, foundArchs)
	if len(architectures) == 0 {
		// Found all the architectures in model state.
		return true, nil
	}

	controllerBinaries, err := a.ctrlSt.GetAllAgentStoreBinaries(ctx)
	if err != nil {
		return false, errors.Errorf(
			"getting available agent binaries in controller store: %w", err,
		)
	}

	for _, binary := range controllerBinaries {
		if binary.Version.Compare(version) == 0 {
			foundArchs = append(foundArchs, binary.Architecture)
		}
	}
	architectures = agentbinary.ArchitectureNotIn(architectures, foundArchs)
	if len(architectures) == 0 {
		// Found all the architectures in model state and controller state.
		return true, nil
	}

	ssBinaries, err := a.getSimpleStreamsAgentBinariesForVersionStream(
		ctx, version, stream,
	)
	if err != nil {
		return false, errors.Errorf(
			"getting available agent binaries in controller for stream %q: %w",
			stream, err,
		)
	}

	foundArchs = slices.AppendSeq(
		foundArchs,
		agentbinary.AgentBinaryArchitectures(ssBinaries),
	)
	architectures = agentbinary.ArchitectureNotIn(architectures, foundArchs)

	if len(architectures) != 0 {
		// We still have architectures left that we couldn't find.
		return false, nil
	}

	return true, nil
}

// GetHighestPatchVersionAvailable returns the highest patch version of the current controller.
func (a *StreamAgentBinaryFinder) GetHighestPatchVersionAvailable(
	ctx context.Context,
) (semversion.Number, error) {
	stream, err := a.modelSt.GetModelAgentStream(ctx)
	if err != nil {
		return semversion.Number{}, errors.Errorf(
			"getting the controller models current agent stream: %w", err,
		)
	}

	return a.GetHighestPatchVersionAvailableForStream(ctx, stream)
}

// GetHighestPatchVersionAvailableForStream returns the highest patch version of the current
// controller given a stream.
func (a *StreamAgentBinaryFinder) GetHighestPatchVersionAvailableForStream(
	ctx context.Context,
	stream agentbinary.Stream,
) (semversion.Number, error) {
	if !stream.IsValid() {
		return semversion.Number{}, errors.Errorf(
			"agent binary stream %q is not valid", stream,
		).Add(coreerrors.NotValid)
	}

	ctrlVersion, err := a.ctrlSt.GetControllerTargetVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting current controller target agent version: %w", err,
		)
	}

	storeBinaries, err := a.ctrlSt.GetAllAgentStoreBinaries(ctx)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting all controller agent store binaries: %w", err,
		)
	}
	ssBinaries, err := a.getSimpleStreamsPatchVersions(ctx, ctrlVersion, stream)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting available simplestream patch versions for controller: %w",
			err,
		)
	}

	slices.SortFunc(ssBinaries, agentbinary.AgentBinaryCompareOnVersion)
	ssBinaries = slices.CompactFunc(
		ssBinaries, agentbinary.AgentBinaryCompactOnVersion,
	)

	var highestStoreVersion semversion.Number
	for _, binary := range storeBinaries {
		if binary.Version.Major == ctrlVersion.Major &&
			binary.Version.Minor == ctrlVersion.Minor &&
			binary.Version.Compare(highestStoreVersion) > 0 {
			highestStoreVersion = binary.Version
		}
	}

	var highestSSBinary agentbinary.AgentBinary
	if len(ssBinaries) != 0 {
		highestSSBinary = slices.MaxFunc(ssBinaries, agentbinary.AgentBinaryHighestVersion)
	}

	recVersion := highestSSBinary.Version
	if highestStoreVersion.Compare(highestSSBinary.Version) > 0 {
		recVersion = highestStoreVersion
	}

	if recVersion == semversion.Zero {
		return semversion.Zero, errors.New(
			"unable to find highest patch version for controller",
		).Add(domainagentbinaryerrors.NotFound)
	}

	return recVersion, nil
}
