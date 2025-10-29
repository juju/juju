// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

// SimpleStreamsAgentFinder is a wrapper to expose helper functions for fetching
// the binaries from simplestreams. It conforms to [AgentFinder].
type SimpleStreamsAgentFinder struct {
	GetPreferredStreamsFn GetPreferredSimpleStreamsFunc
	AgentBinaryFilterFn   AgentBinaryFilterFunc
	GetProviderFn         GetProviderFunc

	ssFetcher envtools.SimplestreamsFetcher
}

// NewSimpleStreamsAgentFinder returns a new SimpleStreamsAgentFinder that exposes helper methods.
func NewSimpleStreamsAgentFinder(
	getPreferredSimpleStreamsFn GetPreferredSimpleStreamsFunc,
	agentBinaryFilterFn AgentBinaryFilterFunc,
	getProviderFn GetProviderFunc,
	ssFetcher envtools.SimplestreamsFetcher,
) *SimpleStreamsAgentFinder {
	return &SimpleStreamsAgentFinder{
		GetPreferredStreamsFn: getPreferredSimpleStreamsFn,
		AgentBinaryFilterFn:   agentBinaryFilterFn,
		GetProviderFn:         getProviderFn,
		ssFetcher:             ssFetcher,
	}
}

// Name returns the identifier for the [SimpleStreamsAgentFinder] struct.
func (a SimpleStreamsAgentFinder) Name() string {
	return "SimpleStreamsAgentFinder"
}

// SearchForAgentVersions returns the agent versions from the provided streams
// that match the supplied major and minor versions.
// It further narrows the results using the given filter.
func (a SimpleStreamsAgentFinder) SearchForAgentVersions(
	ctx context.Context,
	version semversion.Number,
	stream *agentbinary.Stream,
	filter coretools.Filter,
) ([]semversion.Number, error) {
	provider, err := a.GetProviderFn(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return []semversion.Number{}, errors.Errorf("getting provider for "+
			"agent binary finder %w", coreerrors.NotSupported)
	}
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}

	var streams []string
	if stream == nil {
		cfg := provider.Config()
		streams = a.GetPreferredStreamsFn(&version, cfg.Development(),
			cfg.AgentStream())
	} else {
		streams = []string{stream.String()}
	}

	tools, err := a.AgentBinaryFilterFn(ctx, a.ssFetcher, provider, version.Major,
		version.Minor, streams, filter)
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}
	versions := make([]semversion.Number, 0, tools.Len())
	for _, tool := range tools {
		versions = append(versions, tool.Version.Number)
	}
	return versions, nil
}
