// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"slices"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/agentbinary"
	domainagenterrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/internal/errors"
)

// ModelState defines the interface required to get model specific agent binary
// information.
type ModelState interface {
	// GetAgentStream returns the stream currently in use by the model.
	GetAgentStream(ctx context.Context) (agentbinary.Stream, error)
}

// AgentBinaryService provides the API for working with agent binaries.
// It is used to list agent binaries from the controller and model states.
// The service is used to provide a unified view of the agent binaries
// across the controller and model states.
type AgentBinaryService struct {
	// externalStores defines in order the set of external stores to use get
	// getting agent binaries that don't exist in the
	// [AgentBinaryService.putableAgentStore].
	externalStores []AgentBinaryGetterStore

	// logger is used to high light warnings around fetching and caching agent
	// binaries that can not be surfaced to a caller.
	logger logger.Logger

	// putableAgentStore defines the store attached to this service where agent
	// binary caching and storing can occur.
	putableAgentStore AgentBinaryStore

	// state provides access to the underlying model's state.
	state ModelState
}

// NewService returns a new instance of AgentBinaryService.
// It takes two states: the controller state and the model state to aggregate the
// agent binaries from both states.
func NewService(
	state ModelState,
	logger logger.Logger,
	putableAgentStore AgentBinaryStore,
	externalStores ...AgentBinaryGetterStore,
) *AgentBinaryService {
	return &AgentBinaryService{
		externalStores:    externalStores,
		logger:            logger,
		putableAgentStore: putableAgentStore,
		state:             state,
	}
}

// GetAgentBinary retrieves the agent binary for the specified version from the
// underlying agent store. The returned reader provides access to the
// verified binary contents. The caller MUST close the provided stream under non
// error conditions.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied version is not valid.
// - [domainagenterrors.NotFound] if the agent binary does not exist in the
// store.
func (s *AgentBinaryService) GetAgentBinaryForVersion(
	ctx context.Context, ver agentbinary.Version,
) (io.ReadCloser, int64, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := ver.Validate(); err != nil {
		return nil, 0, errors.Capture(err)
	}

	stream, err := s.state.GetAgentStream(ctx)
	if err != nil {
		return nil, 0, errors.Errorf(
			"getting agent stream for model: %w",
			err,
		)
	}

	reader, size, _, err := s.putableAgentStore.GetAgentBinaryForVersionStreamSHA256(
		ctx, ver, stream,
	)
	if err != nil {
		return nil, 0, errors.Errorf("getting agent binary from store: %w", err)
	}

	return reader, size, nil
}

// GetAndCacheExternalAgentBinary attempts to retrieve the specified agent
// binary from one of the available external agent binary stores.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied version is not valid.
// - [domainagenterrors.NotFound] if the agent binary does not exist in any of
// the available external stores.
// - [domainagenterrors.HashMismatch] if the external agent binary store=
// supplied data that did not meet the expected hash.
//
// or more configured external stores. It validates the integrity of the fetched
// binary via SHA256 and SHA384 comparison, then caches and persists it into the
// local store for subsequent faster retrieval. If the binary cannot be found in
// any external store or fails hash verification, an appropriate error is
// returned. The returned reader provides the verified binary content along with
// its size and SHA384 checksum.
func (s *AgentBinaryService) GetAndCacheExternalAgentBinary(
	ctx context.Context, ver agentbinary.Version,
) (io.ReadCloser, int64, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := ver.Validate(); err != nil {
		return nil, 0, errors.Capture(err)
	}

	stream, err := s.state.GetAgentStream(ctx)
	if err != nil {
		return nil, 0, errors.Errorf(
			"getting agent stream for model: %w",
			err,
		)
	}

	var (
		extReader io.ReadCloser
		extSize   int64
		extSHA256 string
	)
	for i, store := range s.externalStores {
		var err error
		extReader, extSize, extSHA256, err = store.GetAgentBinaryForVersionStreamSHA256(
			ctx, ver, stream,
		)
		if errors.Is(err, domainagenterrors.NotFound) {
			// External store does not have the requested agent binary, try
			// the next one.
			continue
		} else if err != nil {
			// Return any unknown err early since we are not sure if continuing
			// is safe.
			return nil, 0, errors.Errorf(
				"fetching agent binary %q from external store %d: %w",
				ver, i, err,
			)
		}

		// External store has what we are looking for, let's get out of here!
		break
	}

	// if extReader is nil then no external agent binary was found
	if extReader == nil {
		return nil, 0, errors.Errorf(
			"agent binary %q not available in external stores", ver,
		).Add(domainagenterrors.NotFound)
	}

	// Make sure we close the external reader.
	defer func(extReader io.ReadCloser) {
		err := extReader.Close()
		if err != nil {
			s.logger.Errorf(
				ctx, "closing external agent binary store stream: %s", err.Error(),
			)
		}
	}(extReader)

	rSHA, shaCalc := computeSHA256andSHA384(extReader)
	cacheR, err := newStrictCacher(rSHA, extSize)
	if errors.Is(err, ErrorReaderDesync) {
		// This error happens when the number of bytes reported by the external
		// store does not match what was provided.
		return nil, 0, errors.Errorf(
			"agent binary received from external store did not match the expected number of bytes %d",
			extSize,
		)
	} else if err != nil {
		return nil, 0, errors.Errorf(
			"caching agent binary from external store for processing: %w", err,
		)
	}

	defer func() {
		err := cacheR.Close()
		if err != nil {
			s.logger.Errorf(
				ctx, "closing external agent binary store cache: %s", err.Error(),
			)
		}
	}()

	sha256Calc, sha384Calc := shaCalc()
	if sha256Calc != extSHA256 {
		return nil, 0, errors.Errorf(
			"computed sha from external store does not match expected value",
		).Add(domainagenterrors.HashMismatch)
	}

	// Add the external agent binary to our store for faster access next time.
	err = s.putableAgentStore.AddAgentBinaryWithSHA384(
		ctx, cacheR, ver, extSize, sha384Calc,
	)
	if errors.Is(err, domainagenterrors.AlreadyExists) {
		// This is fine, it means the effort was wasted but the end result has
		// been achieved.
		s.logger.Debugf(ctx, "external agent binary has already been cached in store")
	} else if err != nil {
		return nil, 0, errors.Errorf(
			"storing external agent binary in store for caching: %w", err,
		)
	}

	r, size, err := s.putableAgentStore.GetAgentBinaryForSHA384(ctx, sha384Calc)
	if err != nil {
		return nil, 0, errors.Errorf(
			"retrieving agent binary from store after caching: %w", err,
		)
	}

	return r, size, nil
}

// GetAvailableAgentBinaryiesForVersion returns a list of all agent binaries
// available for the specified version across all architectures supported.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied version is not valid.
func (s *AgentBinaryService) GetAvailableAgentBinaryiesForVersion(
	ctx context.Context, ver semversion.Number,
) ([]agentbinary.AgentBinary, error) {
	if ver == semversion.Zero {
		return nil, errors.New("version cannot be zero").Add(coreerrors.NotValid)
	}

	stream, err := s.state.GetAgentStream(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model's current agent stream: %w", err,
		)
	}

	archsToFind := agentbinary.SupportedArchitectures()
	retVal := make([]agentbinary.AgentBinary, 0, len(archsToFind))

	available, err := s.putableAgentStore.GetAvailableForVersionInStream(
		ctx, ver, stream,
	)
	if err != nil {
		return nil, errors.Errorf(
			"getting available agent binaries in store for version %q in stream %q: %w",
			ver, stream, err,
		)
	}

	available = slices.DeleteFunc(
		available, agentbinary.AgentBinaryNotMatchingArchitectures(archsToFind),
	)
	archsToFind = agentbinary.ArchitectureNotIn(
		archsToFind,
		slices.Collect(agentbinary.AgentBinaryArchitectures(available)),
	)
	retVal = append(retVal, available...)

	if len(archsToFind) == 0 {
		// Found all agent binaries nothing more to do.
		return retVal, nil
	}

	for i, store := range s.externalStores {
		available, err := store.GetAvailableForVersionInStream(
			ctx, ver, stream,
		)
		if err != nil {
			return nil, errors.Errorf(
				"getting available agent binaries in external store %d for version %q in stream %q: %w",
				i, ver, stream, err,
			)
		}

		available = slices.DeleteFunc(
			available, agentbinary.AgentBinaryNotMatchingArchitectures(archsToFind),
		)
		archsToFind = agentbinary.ArchitectureNotIn(
			archsToFind,
			slices.Collect(agentbinary.AgentBinaryArchitectures(available)),
		)
		retVal = append(retVal, available...)

		if len(archsToFind) == 0 {
			// Found all agent binaries nothing more to do.
			return retVal, nil
		}
	}

	return retVal, nil
}
