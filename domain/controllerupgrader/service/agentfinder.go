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
	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	internaltools "github.com/juju/juju/internal/tools"
)

// AgentFinderControllerState defines the interface for interacting with the
// controller state.
type AgentFinderControllerState interface {
	// GetControllerTargetVersion returns the target controller version in use by the
	// cluster.
	GetControllerTargetVersion(ctx context.Context) (semversion.Number, error)
}

// AgentFinderControllerModelState defines the interface for interacting with the
// underlying model that hosts the current controller(s).
type AgentFinderControllerModelState interface {
	// GetAllAgentStoreBinariesForStream returns all agent binaries that are
	// available in the controller store for a given stream. If no agent
	// binaries exist for the stream, an empty slice is returned.
	GetAllAgentStoreBinariesForStream(
		context.Context, agentbinary.Stream,
	) ([]agentbinary.AgentBinary, error)

	// GetModelAgentStream returns the currently used stream for the agent.
	GetModelAgentStream(ctx context.Context) (agentbinary.Stream, error)
}

// AgentBinaryQuerierStore defines an agent binary store that can be queried for
// what is available.
type AgentBinaryQuerierStore interface {
	// GetAvailableForVersionInStream returns the available agent binaries for
	// the provided version and stream in the store.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] if the stream value is not valid.
	GetAvailableForVersionInStream(
		context.Context, semversion.Number, agentbinary.Stream,
	) ([]agentbinary.AgentBinary, error)

	// GetAvailablePatchVersionsInStream returns a slice of [agentbinary.AgentBinary]s
	// that are available from store that share the same major and minor
	// version as that of the supplied version.
	//
	// The following errors may be returned:
	// - [coreerrors.NotValid] if the stream value is not valid.
	GetAvailablePatchVersionsInStream(
		context.Context, semversion.Number, agentbinary.Stream,
	) ([]agentbinary.AgentBinary, error)
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

// StreamAgentBinaryFinder exposes helper methods for fetching
// the binaries from simplestreams.
type StreamAgentBinaryFinder struct {
	ctrlSt  AgentFinderControllerState
	modelSt AgentFinderControllerModelState

	controllerStore   AgentBinaryQuerierStore
	simplestreamStore AgentBinaryQuerierStore
}

// NewStreamAgentBinaryFinder returns a new StreamAgentBinaryFinder to assist
// in looking up agent binaries.
func NewStreamAgentBinaryFinder(
	ctrlSt AgentFinderControllerState,
	modelSt AgentFinderControllerModelState,
	controllerStore AgentBinaryQuerierStore,
	simplestreamStore AgentBinaryQuerierStore,
) *StreamAgentBinaryFinder {
	return &StreamAgentBinaryFinder{
		ctrlSt:            ctrlSt,
		modelSt:           modelSt,
		controllerStore:   controllerStore,
		simplestreamStore: simplestreamStore,
	}
}

// HasBinariesForVersionAndArchitectures returns true if agent binaries exist
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
		return false, errors.Errorf(
			"getting the controller's model agent stream: %w", err,
		)
	}

	return a.HasBinariesForVersionStreamAndArchitectures(
		ctx, version, stream, architectures,
	)
}

// HasBinariesForVersionStreamAndArchitectures consults three sources to check
// that agent binaries exist for a given version, stream, and architectures.
// First the model object store is consulted to see what agent binaries it can
// supply for the request.
// Any missing gaps are then progressed on to the controllers agent binary
// store.
// Finally if there are still gaps in what is available, simplestreams is
// consulted.
//
// The following errors may be returned:
// - [coreerrors.NotValid] if the supplied stream is not valid.
func (a *StreamAgentBinaryFinder) HasBinariesForVersionStreamAndArchitectures(
	ctx context.Context,
	version semversion.Number,
	stream agentbinary.Stream,
	architectures []agentbinary.Architecture,
) (bool, error) {
	if !stream.IsValid() {
		return false, errors.Errorf(
			"agent binary stream %q is not valid", stream,
		).Add(coreerrors.NotValid)
	}

	if len(architectures) == 0 {
		// We can't find architectures for an empty slice.
		return false, nil
	}
	// Dedupe architectures.
	architectures = slices.Compact(architectures)

	modelBinaries, err := a.modelSt.GetAllAgentStoreBinariesForStream(ctx, stream)
	if err != nil {
		return false, errors.Errorf(
			"getting available agent binaries in the controller's model object store for stream %q: %w",
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

	controllerBinaries, err := a.controllerStore.GetAvailableForVersionInStream(ctx, version, stream)
	if err != nil {
		return false, errors.Errorf(
			"getting available agent binaries in the controller's object store for stream %q: %w",
			stream, err,
		)
	}

	foundArchs = slices.AppendSeq(
		foundArchs,
		agentbinary.AgentBinaryArchitectures(controllerBinaries),
	)
	architectures = agentbinary.ArchitectureNotIn(architectures, foundArchs)
	if len(architectures) == 0 {
		// Found all the architectures in model state and controller state.
		return true, nil
	}

	ssBinaries, err := a.simplestreamStore.GetAvailableForVersionInStream(
		ctx,
		version,
		stream,
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

// GetHighestPatchVersionAvailable returns the highest patch version available
// relative to the controllers current version.
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

// GetHighestPatchVersionAvailableForStream returns the highest patch version
// available in the current stream relative to the controllers current version.
//
// The following errors may be returned:
// - [coreerrors.NotValid] if the supplied stream is not valid.
// - [domainagentbinaryerrors.NotFound] if no version could be found.
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

	storeBinaries, err := a.controllerStore.GetAvailablePatchVersionsInStream(
		ctx,
		ctrlVersion,
		stream,
	)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting available controller agent store patch versions for stream %q: %w",
			stream.String(), err,
		)
	}

	ssBinaries, err := a.simplestreamStore.GetAvailablePatchVersionsInStream(
		ctx,
		ctrlVersion,
		stream,
	)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting available simplestream patch versions for controller: %w",
			err,
		)
	}

	storeBinaries = slices.CompactFunc(
		storeBinaries, agentbinary.AgentBinaryCompactOnVersion,
	)
	ssBinaries = slices.CompactFunc(
		ssBinaries, agentbinary.AgentBinaryCompactOnVersion,
	)

	var highestStoreBinary agentbinary.AgentBinary
	if len(storeBinaries) != 0 {
		highestStoreBinary = slices.MaxFunc(storeBinaries, agentbinary.AgentBinaryHighestVersion)
	}

	var highestSSBinary agentbinary.AgentBinary
	if len(ssBinaries) != 0 {
		highestSSBinary = slices.MaxFunc(ssBinaries, agentbinary.AgentBinaryHighestVersion)
	}

	recVersion := highestSSBinary.Version
	if highestStoreBinary.Version.Compare(highestSSBinary.Version) > 0 {
		recVersion = highestStoreBinary.Version
	}

	if recVersion == semversion.Zero {
		return semversion.Zero, errors.New(
			"unable to find highest patch version for controller",
		).Add(domainagentbinaryerrors.NotFound)
	}

	return recVersion, nil
}
