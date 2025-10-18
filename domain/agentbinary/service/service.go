// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"bytes"
	"context"
	"io"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

// AgentBinaryModelState defines the interface for accessing agent binary records
// associated with a model. It provides methods for listing and retrieving
// stored agent binaries and their metadata.
type AgentBinaryModelState interface {
	// ListAgentBinaries lists all agent binaries in the state.
	// It returns a slice of agent binary metadata.
	// An empty slice is returned if no agent binaries are found.
	ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error)

	// GetAgentBinarySHA256 retrieves the SHA256 value for the specified agent binary version.
	// It returns false and an empty string if no matching record exists.
	GetAgentBinarySHA256(ctx context.Context, ver coreagentbinary.Version) (bool, string, error)

	// GetAgentStream returns the stream used by the current model.
	GetAgentStream(ctx context.Context) (modelagent.AgentStream, error)
}

// AgentBinaryControllerState defines the interface for accessing agent binary records
// associated with a controller. It provides methods for listing and retrieving
// stored agent binaries and their metadata.
type AgentBinaryControllerState interface {
	// ListAgentBinaries lists all agent binaries in the state.
	// It returns a slice of agent binary metadata.
	// An empty slice is returned if no agent binaries are found.
	ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error)

	// GetAgentBinarySHA256 retrieves the SHA256 value for the specified agent binary version.
	// It returns false and an empty string if no matching record exists.
	GetAgentBinarySHA256(ctx context.Context, ver coreagentbinary.Version, stream modelagent.AgentStream) (bool, string, error)
}

// ControllerStore defines an interface for retrieving agent binaries
// from a controller store.
// It provides access to the binary content, its size and its associated SHA256 hash.
type ControllerStore interface {
	// GetAgentBinary retrieves the agent binary corresponding to the given version
	// and stream from the controller's object store.
	//
	// The function first queries controller state to check whether a SHA256 record
	// exists for the requested version and stream. If no such record is found,
	// agentbinaryerrors.NotFound is returned.
	//
	// If a valid SHA256 is recorded, the corresponding binary blob is fetched from
	// the controller's object store and returned as an io.ReadCloser together with
	// its size and SHA256 string.
	//
	// The caller is responsible for closing the returned reader.
	GetAgentBinary(ctx context.Context, ver coreagentbinary.Version, stream modelagent.AgentStream) (io.ReadCloser, int64, string, error)
}

// SimpleStreamStore defines an interface for retrieving agent binaries
// from a controller store.
// It provides access to the binary content, its size and its associated SHA256 hash.
type SimpleStreamStore interface {
	// GetAgentBinary retrieves the agent binary corresponding to the given version
	// and stream from the controller's object store.
	//
	// The function first queries controller state to check whether a SHA256 record
	// exists for the requested version and stream. If no such record is found,
	// agentbinaryerrors.NotFound is returned.
	//
	// If a valid SHA256 is recorded, the corresponding binary blob is fetched from
	// the controller's object store and returned as an io.ReadCloser together with
	// its size and SHA256 string.
	//
	// The caller is responsible for closing the returned reader.
	GetAgentBinary(ctx context.Context, ver coreagentbinary.Version, stream modelagent.AgentStream) (io.ReadCloser, int64, string, error)

	// GetEnvironAgentBinariesFinder returns a function that can be used to find
	// agent binaries from the simplestreams data sources.
	GetEnvironAgentBinariesFinder() EnvironAgentBinariesFinderFunc
}

// ModelStore defines an interface for retrieving agent binaries
// from the model store.
// It provides access to the binary content, its size and its associated SHA256 hash.
type ModelStore interface {
	// GetAgentBinary retrieves the agent binary corresponding to the given version
	// and stream from the model's object store.
	//
	// The function first queries model state to check whether a SHA256 record
	// exists for the requested version and stream. If no such record is found,
	// agentbinaryerrors.NotFound is returned.
	//
	// If a valid SHA256 is recorded, the corresponding binary blob is fetched from
	// the model's object store and returned as an io.ReadCloser together with
	// its size.
	//
	// The caller is responsible for closing the returned reader.
	GetAgentBinary(ctx context.Context, ver coreagentbinary.Version) (io.ReadCloser, int64, error)

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
}

// AgentBinaryService provides the API for working with agent binaries.
// It is used to list agent binaries from the controller and model states.
// The service is used to provide a unified view of the agent binaries
// across the controller and model states.
type AgentBinaryService struct {
	modelState        AgentBinaryModelState
	modelStore        ModelStore
	controllerState   AgentBinaryControllerState
	controllerStore   ControllerStore
	simpleStreamStore SimpleStreamStore
}

// NewAgentBinaryService returns a new instance of AgentBinaryService.
// It takes two states: the controller state and the model state to aggregate the
// agent binaries from both states.
func NewAgentBinaryService(
	modelState AgentBinaryModelState,
	modelStore ModelStore,
	controllerState AgentBinaryControllerState,
	controllerStore ControllerStore,
	simpleStreamStore SimpleStreamStore,
) *AgentBinaryService {
	return &AgentBinaryService{
		modelState:        modelState,
		modelStore:        modelStore,
		controllerState:   controllerState,
		controllerStore:   controllerStore,
		simpleStreamStore: simpleStreamStore,
	}
}

// ListAgentBinaries lists all agent binaries in the controller and model stores.
// It merges the two lists of agent binaries, with the model agent binaries
// taking precedence over the controller agent binaries.
// It returns a slice of agent binary metadata. The order of the metadata is not guaranteed.
// An empty slice is returned if no agent binaries are found.
func (s *AgentBinaryService) ListAgentBinaries(ctx context.Context) ([]agentbinary.Metadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
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

func (s *AgentBinaryService) GetAgentBinary(ctx context.Context, ver coreagentbinary.Version) (io.ReadCloser, int64, error) {
	reader, size, err := s.modelStore.GetAgentBinary(ctx, ver)

	if err != nil {
		return nil, 0, errors.Capture(err)
	}
	if reader == nil {
		return nil, 0, agentbinaryerrors.NotFound
	}

	defer func(reader io.ReadCloser) {
		_ = reader.Close()
	}(reader)

	b := bytes.NewBuffer(make([]byte, 0, size))

	return io.NopCloser(b), size, nil
}

func (s *AgentBinaryService) GetExternalAgentBinary(ctx context.Context, ver coreagentbinary.Version) (io.ReadCloser, int64, error) {
	var reader io.ReadCloser
	var size int64
	var sha256sum string
	var err error

	stream, err := s.modelState.GetAgentStream(ctx)
	if err != nil {
		return nil, 0, errors.Errorf("getting agent stream from model state: %w", err)
	}

	reader, size, sha256sum, err = s.controllerStore.GetAgentBinary(ctx, ver, stream)
	if errors.Is(err, agentbinaryerrors.NotFound) {
		reader, size, sha256sum, err = s.simpleStreamStore.GetAgentBinary(ctx, ver, stream)
		if err != nil {
			return nil, 0, errors.Capture(err)
		}
	} else if err != nil {
		return nil, 0, errors.Capture(err)
	}
	if reader == nil {
		return nil, 0, agentbinaryerrors.NotFound
	}

	defer func(reader io.ReadCloser) {
		_ = reader.Close()
	}(reader)

	b := bytes.NewBuffer(make([]byte, 0, size))
	tr := io.TeeReader(reader, b)

	err = s.modelStore.AddAgentBinaryWithSHA256(ctx, tr, ver, size, sha256sum)
	if err != nil {
		return nil, 0, errors.Errorf("adding external agent binary to model store: %w", err)
	}
	return io.NopCloser(b), size, nil
}

// GetEnvironAgentBinariesFinder returns a function that can be used to find
// agent binaries from the simplestreams data sources.
func (s *AgentBinaryService) GetEnvironAgentBinariesFinder() EnvironAgentBinariesFinderFunc {
	return s.simpleStreamStore.GetEnvironAgentBinariesFinder()
}
