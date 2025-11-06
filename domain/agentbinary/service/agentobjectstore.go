// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"io"
	"slices"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	intobjectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

// AgentObjectQuerierStore provides read only access to an agent binary store in
// the controller for getting available agent versions.
type AgentObjectQuerierStore struct {
	state AgentObjectStoreState
}

type AgentObjectStore struct {
	AgentObjectQuerierStore

	logger            logger.Logger
	state             AgentObjectStoreState
	objectStoreGetter objectstore.NamespacedObjectStoreGetter
}

type AgentObjectStoreState interface {
	// CheckAgentBinarySHA256Exists checks that the given sha256 sum exists as
	// an agent binary in the object store. This sha256 sum could exist as an
	// object in the object store but unless the association has been made this
	// will always return false.
	CheckAgentBinarySHA256Exists(ctx context.Context, sha256Sum string) (bool, error)

	// GetAllAgentStoreBinariesForStream returns all agent binaries that are
	// available in the controller store for a given stream. If no agent
	// binaries exist for the stream, an empty slice is returned.
	GetAllAgentStoreBinariesForStream(
		context.Context, agentbinary.Stream,
	) ([]agentbinary.AgentBinary, error)

	// GetObjectUUID returns the object store UUID for the given object path.
	// The following errors can be returned:
	// - [agentbinaryerrors.ObjectNotFound] when no object exists that matches this path.
	GetObjectUUID(ctx context.Context, path string) (objectstore.UUID, error)

	// RegisterAgentBinary registers a new agent binary's metadata to the database.
	// [agentbinaryerrors.AlreadyExists] when the provided agent binary already
	// exists.
	// [agentbinaryerrors.ObjectNotFound] when no object exists that matches
	// this agent binary.
	// [coreerrors.NotSupported] if the architecture is not supported by the
	// state layer.
	RegisterAgentBinary(ctx context.Context, arg agentbinary.RegisterAgentBinaryArg) error
}

// NewAgentObjectQuerierStore returns a new instance of
// [AgentObjectQuerierStore] that can be used for querying available agent
// binaries available in one of the controllers object stores.
func NewAgentObjectQuerierStore(
	st AgentObjectStoreState,
) *AgentObjectQuerierStore {
	return &AgentObjectQuerierStore{
		state: st,
	}
}

// NewAgentObjectStore returns a new instance of [AgentObjectStore] that can be
// used for adding, querying and getting agent binaries from one of the
// controller's respective stores.
func NewAgentObjectStore(
	st AgentObjectStoreState,
	logger logger.Logger,
	objectStoreGetter objectstore.NamespacedObjectStoreGetter,
) *AgentObjectStore {
	return &AgentObjectStore{
		AgentObjectQuerierStore: AgentObjectQuerierStore{
			state: st,
		},
		state:             st,
		logger:            logger,
		objectStoreGetter: objectStoreGetter,
	}
}

// GetAvailableForVersionInStream returns the available agent binaries for
// the provided version and stream in the store.
//
// The following errors may be returned:
// - [coreerrors.NotValid] if the stream value is not valid.
func (s *AgentObjectQuerierStore) GetAvailableForVersionInStream(
	ctx context.Context, ver semversion.Number, stream agentbinary.Stream,
) ([]agentbinary.AgentBinary, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if ver == semversion.Zero {
		return nil, errors.New("invalid version number").Add(coreerrors.NotValid)
	}
	if !stream.IsValid() {
		return nil, errors.New("agent stream is not valid").Add(coreerrors.NotValid)
	}

	storeList, err := s.state.GetAllAgentStoreBinariesForStream(ctx, stream)
	if err != nil {
		return nil, errors.Capture(err)
	}

	retVal := slices.DeleteFunc(
		storeList, agentbinary.AgentBinaryNotMatchingVersion(ver),
	)
	return retVal, nil
}

// GetAvailablePatchVersions returns a slice of [agentbinary.AgentBinary]s
// that are available from store that share the the same major and minor
// version as that of the supplied version.
//
// The following errors may be returned:
// - [coreerrors.NotValid] if the stream value is not valid.
func (s *AgentObjectQuerierStore) GetAvailablePatchVersionsInStream(
	ctx context.Context, ver semversion.Number, stream agentbinary.Stream,
) ([]agentbinary.AgentBinary, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if ver == semversion.Zero {
		return nil, errors.New("invalid version number").Add(coreerrors.NotValid)
	}
	if !stream.IsValid() {
		return nil, errors.New("agent stream is not valid").Add(coreerrors.NotValid)
	}

	storeList, err := s.state.GetAllAgentStoreBinariesForStream(ctx, stream)
	if err != nil {
		return nil, errors.Capture(err)
	}

	retVal := slices.DeleteFunc(
		storeList, agentbinary.AgentBinaryNotWithinPatchOfVersion(ver),
	)
	return retVal, nil
}

// generatePath generates the path for the agent binary in the object store.
// The path is in the format of "agent-binaries/{version}-{arch}-{sha384}".
// We don't want to generate the path using the String() of the version
// because it may change in the future.
func generatePath(version agentbinary.Version, sha384 string) string {
	num := version.Number
	numberStr := fmt.Sprintf("%d.%d.%d", num.Major, num.Minor, num.Patch)
	if num.Tag != "" {
		numberStr = fmt.Sprintf("%d.%d-%s%d", num.Major, num.Minor, num.Tag, num.Patch)
	}
	if num.Build > 0 {
		numberStr += fmt.Sprintf(".%d", num.Build)
	}
	return fmt.Sprintf(
		"agent-binaries/%s-%s-%s",
		numberStr,
		version.Architecture.String(),
		sha384,
	)
}

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
func (s *AgentObjectStore) AddAgentBinaryWithSHA256(
	ctx context.Context, r io.Reader,
	version agentbinary.Version,
	size int64, sha256 string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := version.Validate(); err != nil {
		return errors.Errorf("agent version %q is not valid: %w", version, err)
	}

	shaReader, shaCalc := computeSHA256andSHA384(r)
	cacheReader, err := newStrictCacher(shaReader, size)
	if errors.Is(err, ErrorReaderDesync) {
		return errors.New(
			"supplied agent binary stream does not match the size",
		)
	} else if err != nil {
		return errors.Errorf(
			"caching agent binary before sending to object store: %w", err,
		)
	}

	defer func() {
		err := cacheReader.Close()
		if err != nil {
			s.logger.Errorf(
				ctx,
				"closing cache reader when stream agent binary to object store: %w",
				err,
			)
		}
	}()

	encoded256, encoded384 := shaCalc()

	if sha256 != encoded256 {
		return errors.Errorf(
			"SHA256 mismatch for agent binary %q: expected %q, got %q",
			version, sha256, encoded256,
		).Add(agentbinaryerrors.HashMismatch)
	}

	return s.add(ctx, cacheReader, version, size, encoded384)
}

// We use sha384 for validating the binary later in the concrete implementations.
func (s *AgentObjectStore) add(
	ctx context.Context, r io.Reader,
	version agentbinary.Version,
	size int64, sha384 string,
) (resultErr error) {
	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return errors.Errorf("getting object store for agent binary: %w", err)
	}

	path := generatePath(version, sha384)
	uuid, err := objectStore.PutAndCheckHash(ctx, path, r, size, sha384)
	switch {
	// Happens when the agent binary data already exists in the object store.
	// We still need the unique uuid so we can make sure it is registered
	// against this path.
	case errors.Is(err, objectstoreerrors.ErrHashAndSizeAlreadyExists):
		existingObjectUUID, err := s.state.GetObjectUUID(ctx, path)
		if err != nil {
			return errors.Errorf("getting object store UUID for %q: %w", path, err)
		}
		uuid = objectstore.UUID(existingObjectUUID.String())

	// Happens when the computed hash is different to that of what we expected.
	case errors.Is(err, objectstore.ErrHashMismatch):
		return errors.New("agent binary data does not match expected hash").Add(agentbinaryerrors.HashMismatch)

	// All other errors
	case err != nil:
		return errors.Errorf("putting agent binary of %q with hash %q in the object store: %w", version, sha384, err)
	}

	s.logger.Debugf(
		ctx,
		"adding agent binary %q with arch %q to agent binary store",
		version.Number,
		version.Architecture.String(),
	)

	err = s.state.RegisterAgentBinary(ctx, agentbinary.RegisterAgentBinaryArg{
		Version:         version.Number.String(),
		Architecture:    version.Architecture,
		ObjectStoreUUID: uuid,
	})
	if errors.IsOneOf(err,
		agentbinaryerrors.AgentBinaryImmutable,
		agentbinaryerrors.ObjectNotFound,
		coreerrors.NotSupported) {
		// We need to clean up the newly added binary from the object store.
		// But we don't want to accidentally remove an existing binary if any unexpected errors occur.
		// The best we can do is to clean up the binary for certain unknown errors.
		// If there is a retry, the uploaded binary will be picked up again and recorded in the database.
		if err := objectStore.Remove(ctx, path); err != nil && !errors.Is(err, objectstoreerrors.ErrNotFound) {
			s.logger.Errorf(ctx,
				"saving agent binary metadata %q failed, removing the binary from object store: %v",
				path, err,
			)
		}
	}
	if err != nil {
		return errors.Errorf("saving agent binary metadata for %q: %w", path, err)
	}
	return nil
}

// GetAgentBinaryForSHA256 returns the agent binary associated with the given
// SHA256 sum.
//
// The following errors can be expected:
// - [agentbinaryerrors.NotFound] when no agent binaries exist for the
// provided sha.
func (s *AgentObjectStore) GetAgentBinaryForSHA256(
	ctx context.Context,
	sha256Sum string,
) (io.ReadCloser, int64, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// We check that this sha384 exists in the database and is associated with
	// agent binaries. If we don't do this the possibility exists to leak other
	// non-related objects out of the store via this interface.
	exists, err := s.state.CheckAgentBinarySHA256Exists(ctx, sha256Sum)
	if err != nil {
		return nil, 0, errors.Errorf(
			"checking if agent binaries exist for sha256 %q: %w", sha256Sum, err,
		)
	}

	if !exists {
		return nil, 0, errors.Errorf(
			"no agent binaries exist for sha256 %q", sha256Sum,
		).Add(agentbinaryerrors.NotFound)
	}

	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return nil, 0, errors.Errorf(
			"getting object store for agent binary %q: %w", sha256Sum, err,
		)
	}

	reader, size, err := store.GetBySHA256(ctx, sha256Sum)
	if errors.Is(err, intobjectstoreerrors.ObjectNotFound) {
		return nil, 0, errors.Errorf(
			"no agent binaries exist for sha256 %q", sha256Sum,
		).Add(agentbinaryerrors.NotFound)
	} else if err != nil {
		return nil, 0, errors.Errorf(
			"getting object with sha256 sum %q: %w", sha256Sum, err,
		)
	}

	return reader, size, nil
}

// GetAgentBinaryForVersionStream retrieves the agent binary
// corresponding to the given version and stream. If successfully found the
// the agent binary stream is returned along with its size and sha256 sum.
// It is the caller's responsibility to close the returned stream when no
// error condition exists.
//
// The following errors may be returned:
// - [github.com/juju/juju/domain/agentbinary/errors.NotFound] if the agent
// binary does not exist.
func (s *AgentObjectStore) GetAgentBinaryForVersionStreamSHA256(
	ctx context.Context,
	ver agentbinary.Version,
	stream agentbinary.Stream,
) (io.ReadCloser, int64, string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	s.logger.Debugf(
		ctx,
		"retrieving agent binary from agent binary store for ver %q, arch %q and stream %q",
		ver.Number.String(), ver.Architecture.String(), stream.String(),
	)

	storeBinaries, err := s.state.GetAllAgentStoreBinariesForStream(ctx, stream)
	if err != nil {
		return nil, 0, "", errors.Errorf(
			"getting agent binaries for stream %q in store: %w", stream.String(),
			err,
		)
	}

	storeBinaries = slices.DeleteFunc(
		storeBinaries, agentbinary.AgentBinaryNotMatchinAgentVersion(ver),
	)

	if len(storeBinaries) == 0 {
		return nil, 0, "", errors.Errorf(
			"no agent binary exists for version %q, architecture %q in stream %q",
			ver.Number.String(), ver.Architecture.String(), stream.String(),
		).Add(agentbinaryerrors.NotFound)
	}

	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return nil, 0, "", errors.Errorf(
			"getting object store to stream agent binary: %w", err,
		)
	}

	reader, size, err := store.GetBySHA256(ctx, storeBinaries[0].SHA256)
	if errors.Is(err, intobjectstoreerrors.ObjectNotFound) {
		return nil, 0, "", errors.Errorf(
			"agent binary is missing for version %q, architecture %q and stream %q in binary object store",
			ver.Number.String(), ver.Architecture.String(), stream.String(),
		).Add(agentbinaryerrors.NotFound)
	} else if err != nil {
		return nil, 0, "", errors.Errorf(
			"getting agent binary object stream for version %q, architecture %q and stream %q: %w",
			ver.Number.String(), ver.Architecture.String(), stream.String(), err,
		)
	}
	return reader, size, storeBinaries[0].SHA256, nil
}
