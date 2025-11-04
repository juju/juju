// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/agentbinary"
	domainagenterrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

type AgentBinaryLocalStore interface {
	// AddAgentBinaryWithSHA256 adds a new agent binary to the object store and saves its metadata to the database.
	// The following errors can be returned:
	// - [coreerrors.NotSupported] if the architecture is not supported.
	// - [agentbinaryerrors.AlreadyExists] if an agent binary already exists for
	// this version and architecture.
	// - [agentbinaryerrors.ObjectNotFound] if there was a problem referencing the
	// agent binary metadata with the previously saved binary object. This error
	// should be considered an internal problem. It is discussed here to make the
	// caller aware of future problems.
	// - [coreerrors.NotValid] if the agent version is not valid.
	// - [agentbinaryerrors.HashMismatch] when the expected sha does not match that
	// which was computed against the binary data.
	AddAgentBinaryWithSHA256(
		ctx context.Context, r io.Reader,
		version coreagentbinary.Version,
		size int64, sha256 string,
	) error

	// GetAgentBinaryWithSHA256 retrieves the agent binary corresponding to the given version
	// and stream from an external store.
	// The caller is responsible for closing the returned reader.
	//
	// The following errors may be returned:
	// - [domainagenterrors.NotFound] if the agent binary metadata does not exist.
	GetAgentBinaryWithSHA256(
		context.Context,
		coreagentbinary.Version,
		agentbinary.Stream,
	) (io.ReadCloser, int64, string, error)

	// AddAgentBinaryWithSHA384 adds a new agent binary to the store and saves its
	// metadata to the database.
	//
	// The following errors can be returned:
	// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is
	// not supported.
	// - [github.com/juju/juju/domain/agentbinary/errors.AlreadyExists] if an
	// agent binary already exists for this version architecture and stream.
	// - [agentbinaryerrors.ObjectNotFound] if there was a problem referencing
	// the agent binary metadata with the previously saved binary object. This
	// error should be considered an internal problem. It is discussed here to
	// make the caller aware of future problems.
	// - [coreerrors.NotValid] when the agent version is not considered valid.
	// - [agentbinaryerrors.HashMismatch] when the expected sha does not match
	// that which was computed against the binary data.
	AddAgentBinaryWithSHA384(
		ctx context.Context,
		r io.Reader,
		version coreagentbinary.Version,
		size int64,
		sha384 string,
	) error

	// GetAgentBinaryUsingSHA256 returns the agent binary associated with the given
	// SHA256 sum. The following errors can be expected:
	// - [agentbinaryerrors.NotFound] when no agent binaries exist for the provided
	// sha.
	GetAgentBinaryUsingSHA256(
		ctx context.Context,
		sha256Sum string,
	) (io.ReadCloser, int64, error)
}

type ModelState interface {
	// GetAgentStream returns the stream currently in use by the model.
	GetAgentStream(ctx context.Context) (agentbinary.Stream, error)

	// ListAgentBinaries lists all agent binaries in the state.
	// It returns a slice of agent binary metadata.
	// An empty slice is returned if no agent binaries are found.
	ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error)
}

type ControllerState interface {
	// ListAgentBinaries lists all agent binaries in the state.
	// It returns a slice of agent binary metadata.
	// An empty slice is returned if no agent binaries are found.
	ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error)
}

// AgentBinaryService provides the API for working with agent binaries.
// It is used to list agent binaries from the controller and model states.
// The service is used to provide a unified view of the agent binaries
// across the controller and model states.
type AgentBinaryService struct {
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder]
	getPreferredSimpleStreams    PreferredSimpleStreamsFunc
	agentBinaryFilter            AgentBinaryFilter
	controllerState              ControllerState
	externalStores               []AgentBinaryGetterStore
	modelState                   ModelState
	store                        AgentBinaryLocalStore
}

// NewAgentBinaryService returns a new instance of AgentBinaryService.
// It takes two states: the controller state and the model state to aggregate the
// agent binaries from both states.
func NewAgentBinaryService(
	providerForAgentBinaryFinder providertracker.ProviderGetter[ProviderForAgentBinaryFinder],
	getPreferredSimpleStreams PreferredSimpleStreamsFunc,
	agentBinaryFilter AgentBinaryFilter,
	controllerState ControllerState,
	modelState ModelState,
	store AgentBinaryLocalStore,
	externalStores ...AgentBinaryGetterStore,
) *AgentBinaryService {
	return &AgentBinaryService{
		providerForAgentBinaryFinder: providerForAgentBinaryFinder,
		getPreferredSimpleStreams:    getPreferredSimpleStreams,
		agentBinaryFilter:            agentBinaryFilter,
		externalStores:               externalStores,
		controllerState:              controllerState,
		modelState:                   modelState,
		store:                        store,
	}
}

// GetAgentBinary retrieves the agent binary for the specified version from the
// model's configured store. The returned reader provides access to the
// verified binary contents.
func (s *AgentBinaryService) GetAgentBinary(ctx context.Context, ver coreagentbinary.Version) (io.ReadCloser, int64, error) {
	stream, err := s.modelState.GetAgentStream(ctx)
	if err != nil {
		return nil, 0, errors.Errorf(
			"getting agent stream from model state: %w",
			err,
		)
	}

	reader, size, _, err := s.store.GetAgentBinaryWithSHA256(ctx, ver, stream)
	if err != nil {
		return nil, 0, errors.Capture(err)
	}
	if reader == nil {
		return nil, 0, domainagenterrors.NotFound
	}

	return reader, size, nil
}

// GetExternalAgentBinary attempts to retrieve the specified agent binary from one
// or more configured external stores. It validates the integrity of the fetched
// binary via SHA256 and SHA384 comparison, then caches and persists it into the
// local store for subsequent faster retrieval. If the binary cannot be found in
// any external store or fails hash verification, an appropriate error is
// returned. The returned reader provides the verified binary content along with
// its size and SHA384 checksum.
func (s *AgentBinaryService) GetExternalAgentBinary(ctx context.Context, ver coreagentbinary.Version) (io.ReadCloser, int64, string, error) {
	var extReader io.ReadCloser
	var extSize int64
	var extSHA256 string
	var err error

	stream, err := s.modelState.GetAgentStream(ctx)
	if err != nil {
		return nil, 0, "", errors.Errorf(
			"getting agent stream from model state: %w",
			err,
		)
	}

	for i, store := range s.externalStores {
		extReader, extSize, extSHA256, err = store.GetAgentBinaryWithSHA256(
			ctx, ver, stream,
		)
		if errors.Is(err, domainagenterrors.NotFound) {
			continue
		} else if err != nil {
			// Return any unknown err early since we are not sure if proceeding is safe.
			return nil, 0, "", errors.Errorf(
				"attempted fetching agent binary %q from external store %d: %w",
				ver, i, err,
			)
		}
		break
	}

	if extReader == nil {
		return nil, 0, "", errors.Errorf(
			"agent binary %q does not exist in external stores: %w",
			ver, err,
		)
	}

	defer func(extReader io.ReadCloser) {
		_ = extReader.Close()
	}(extReader)
	rSHA, shaCalc := computeSHA256andSHA384(extReader)
	cacheR, err := newStrictCacher(rSHA, extSize)

	if errors.Is(err, ErrorReaderDesync) {
		return nil, 0, "", errors.Errorf(
			"agent binary received from external store did not match the expected number of bytes %d",
			extSize,
		)
	} else if err != nil {
		return nil, 0, "", errors.Errorf(
			"caching agent binary from external store for processing: %w", err,
		)
	}
	defer func() { _ = cacheR.Close() }()

	sha256Calc, sha384Calc := shaCalc()
	if sha256Calc != extSHA256 {
		return nil, 0, "", errors.Errorf(
			"computed sha from external store does not match expected value",
		).Add(domainagenterrors.HashMismatch)
	}

	// Add the external agent binary to our store for faster access next time.
	err = s.store.AddAgentBinaryWithSHA384(ctx, cacheR, ver, extSize, sha384Calc)
	if err != nil {
		return nil, 0, "", errors.Errorf(
			"saving found agent binary from external store: %w", err,
		)
	}

	r, size, err := s.store.GetAgentBinaryUsingSHA256(ctx, sha256Calc)
	return r, size, sha384Calc, err
}

// TODO(Alvin): Remove these following after subsequent callers do not call this

// ListAgentBinaries lists all agent binaries in the controller and model stores.
// It merges the two lists of agent binaries, with the model agent binaries
// taking precedence over the controller agent binaries.
// It returns a slice of agent binary metadata. The order of the metadata is not guaranteed.
// An empty slice is returned if no agent binaries are found.
func (s *AgentBinaryService) ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error) {
	// Merge the two lists of agent binaries. The model agent binaries
	// take precedence over the controller agent binaries.

	allAgentBinaries := make(map[string]agentbinary.Metadata)

	modelAgentBinaries, err := s.modelState.ListAgentBinaries(ctx)
	if err != nil && !errors.Is(err, domainagenterrors.NotFound) {
		return nil, errors.Errorf("listing agent binaries from model state: %w", err)
	}
	controllerAgentBinaries, err := s.controllerState.ListAgentBinaries(ctx)
	if err != nil && !errors.Is(err, domainagenterrors.NotFound) {
		return nil, errors.Errorf("listing agent binaries from controller state: %w", err)
	}

	for _, ab := range controllerAgentBinaries {
		allAgentBinaries[ab.SHA256] = ab
	}

	for _, ab := range modelAgentBinaries {
		allAgentBinaries[ab.SHA256] = ab
	}

	allAgentBinariesSlice := make([]agentbinary.Metadata, 0, len(allAgentBinaries))
	for _, ab := range allAgentBinaries {
		allAgentBinariesSlice = append(allAgentBinariesSlice, ab)
	}
	return allAgentBinariesSlice, nil
}

// PreferredSimpleStreamsFunc is a function that returns the preferred streams
// for the given version and stream.
type PreferredSimpleStreamsFunc func(
	vers *semversion.Number,
	forceDevel bool,
	stream string,
) []string

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
	) (_ coretools.List, err error) {
		ctx, span := trace.Start(ctx, trace.NameFromFunc())
		defer span.End()

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
