// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

// AgentBinaryState describes the interface that the agent binary state must
// implement. It is used to list agent binaries.
type AgentBinaryState interface {
	// ListAgentBinaries lists all agent binaries in the state.
	// It returns a slice of agent binary metadata.
	// An empty slice is returned if no agent binaries are found.
	ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error)
}

// PreferredSimpleStreamsFunc is a function that returns the preferred streams
// for the given version and stream.
type PreferredSimpleStreamsFunc func(
	vers *semversion.Number,
	forceDevel bool,
	stream string,
) []string

// AgentBinaryFilter is a function that filters agent binaries based on the
// given parameters. It returns a list of agent binaries that match the filter
// criteria.
type AgentBinaryFilter func(
	ctx context.Context,
	ss envtools.SimplestreamsFetcher,
	env environs.BootstrapEnviron,
	majorVersion,
	minorVersion int,
	streams []string,
	filter coretools.Filter,
) (coretools.List, error)

// ProviderForAgentBinaryFinder is a subset of cloud provider.
type ProviderForAgentBinaryFinder interface {
	environs.BootstrapEnviron
}

// AgentBinaryService provides the API for working with agent binaries.
// It is used to list agent binaries from the controller and model states.
// The service is used to provide a unified view of the agent binaries
// across the controller and model states.
type AgentBinaryService struct {
	controllerState AgentBinaryState
	modelState      AgentBinaryState

	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder]

	getPreferredSimpleStreams PreferredSimpleStreamsFunc
	agentBinaryFilter         AgentBinaryFilter
}

// NewAgentBinaryService returns a new instance of AgentBinaryService.
// It takes two states: the controller state and the model state to aggregate the
// agent binaries from both states.
func NewAgentBinaryService(
	controllerState AgentBinaryState,
	modelState AgentBinaryState,
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder],
	getPreferredSimpleStreams PreferredSimpleStreamsFunc,
	agentBinaryFilter AgentBinaryFilter,
) *AgentBinaryService {
	return &AgentBinaryService{
		controllerState:              controllerState,
		modelState:                   modelState,
		providerForAgentBinaryFinder: providerForAgentBinaryFinder,
		getPreferredSimpleStreams:    getPreferredSimpleStreams,
		agentBinaryFilter:            agentBinaryFilter,
	}
}

// ListAgentBinaries lists all agent binaries in the controller and model stores.
// It merges the two lists of agent binaries, with the model agent binaries
// taking precedence over the controller agent binaries.
// It returns a slice of agent binary metadata. The order of the metadata is not guaranteed.
// An empty slice is returned if no agent binaries are found.
func (s *AgentBinaryService) ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error) {
	controllerAgentBinaries, err := s.controllerState.ListAgentBinaries(ctx)
	if err != nil {
		return nil, errors.Errorf("listing agent binaries from controller state: %w", err)
	}
	modelAgentBinaries, err := s.modelState.ListAgentBinaries(ctx)
	if err != nil {
		return nil, errors.Errorf("listing agent binaries from model state: %w", err)
	}

	// Merge the two lists of agent binaries. The model agent binaries
	// take precedence over the controller agent binaries.
	allAgentBinaries := make(map[string]agentbinary.Metadata)
	for _, ab := range controllerAgentBinaries {
		allAgentBinaries[ab.SHA256] = ab
	}
	for _, ab := range modelAgentBinaries {
		allAgentBinaries[ab.SHA256] = ab
	}
	// Convert the map back to a slice.
	allAgentBinariesSlice := make([]agentbinary.Metadata, 0, len(allAgentBinaries))
	for _, ab := range allAgentBinaries {
		allAgentBinariesSlice = append(allAgentBinariesSlice, ab)
	}
	return allAgentBinariesSlice, nil
}

// EnvironAgentBinariesFinderFunc is a function that can be used to find agent binaries
// from the simplestreams data sources.
type EnvironAgentBinariesFinderFunc func(
	ctx context.Context,
	major,
	minor int,
	version semversion.Number,
	requestedStream string,
	filter coretools.Filter,
) (coretools.List, error)

// GetEnvironAgentBinariesFinder returns a function that can be used to find
// agent binaries from the simplestreams data sources.
func (s *AgentBinaryService) GetEnvironAgentBinariesFinder() EnvironAgentBinariesFinderFunc {
	return func(
		ctx context.Context,
		major,
		minor int,
		version semversion.Number,
		requestedStream string,
		filter coretools.Filter,
	) (coretools.List, error) {
		provider, err := s.providerForAgentBinaryFinder(ctx)
		if errors.Is(err, coreerrors.NotSupported) {
			return nil, errors.Errorf("getting provider for agent binary finder %w", coreerrors.NotSupported)
		}
		if err != nil {
			return nil, errors.Capture(err)
		}
		cfg := provider.Config()
		if requestedStream == "" {
			requestedStream = cfg.AgentStream()
		}

		streams := s.getPreferredSimpleStreams(&version, cfg.Development(), requestedStream)
		ssFetcher := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
		return s.agentBinaryFilter(ctx, ssFetcher, provider, major, minor, streams, filter)
	}
}
