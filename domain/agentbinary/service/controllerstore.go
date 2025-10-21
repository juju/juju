// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/domain/modelagent"
	domainobjectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

// ControllerStoreState defines the state-facing interface used by
// the controller to query the presence and metadata of agent binaries.
type ControllerStoreState interface {
	// CheckAgentBinarySHA256Exists that the given sha256 sum exists as an agent
	// binary in the object store. This sha256 sum could exist as an object in
	// the object store but unless the association has been made this will
	// always return false.
	CheckAgentBinarySHA256Exists(context.Context, string) (bool, error)

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

	// GetAgentBinarySHA256 retrieves the SHA256 value for the specified agent binary version.
	// It returns false and an empty string if no matching record exists.
	GetAgentBinarySHA256(ctx context.Context, ver coreagentbinary.Version, stream modelagent.AgentStream) (bool, string, error)
}

type ControllerAgentBinaryStore struct {
	state             ControllerStoreState
	logger            logger.Logger
	objectStoreGetter objectstore.NamespacedObjectStoreGetter
}

// NewControllerAgentBinaryStore returns a new instance of ControllerAgentBinaryStore.
func NewControllerAgentBinaryStore(
	state ControllerStoreState,
	logger logger.Logger,
	objectStoreGetter objectstore.NamespacedObjectStoreGetter,
) *ControllerAgentBinaryStore {
	return &ControllerAgentBinaryStore{
		state:             state,
		logger:            logger,
		objectStoreGetter: objectStoreGetter,
	}
}

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
func (s *ControllerAgentBinaryStore) GetAgentBinary(
	ctx context.Context,
	ver coreagentbinary.Version,
	stream modelagent.AgentStream,
) (io.ReadCloser, int64, string, error) {
	hasAgentBinary, sha256Str, err := s.state.GetAgentBinarySHA256(ctx, ver, stream)
	if err != nil {
		return nil, 0, "", errors.Errorf("checking availability of agent binary in controller store: %w", err)
	}
	if !hasAgentBinary {
		return nil, 0, "", errors.New("agent binary not found in controller store").Add(agentbinaryerrors.NotFound)
	}

	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return nil, 0, "", errors.Errorf("getting controller object store: %w", err)
	}

	reader, size, err := store.GetBySHA256(ctx, sha256Str)
	if errors.Is(err, objectstoreerrors.ObjectNotFound) {
		return nil, 0, "", errors.New("agent binary not found in controller store").Add(agentbinaryerrors.NotFound)
	} else if err != nil {
		return nil, 0, "", errors.Errorf("getting agent binary with sha %q from controller object store: %w", sha256Str, err)

	}
	return reader, size, sha256Str, nil
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
func (s *ControllerAgentBinaryStore) AddAgentBinaryWithSHA256(
	ctx context.Context, r io.Reader,
	version coreagentbinary.Version,
	size int64, sha256 string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := version.Validate(); err != nil {
		return errors.Errorf("agent version %q is not valid: %w", version, err)
	}

	// Ideally, we should pass the SHA256 hash to the object store
	// and let it verify the hash. But the object store doesn't support
	// this yet. So we have to calculate the hash ourselves.
	data, encoded256, encoded384, err := tmpCacheAndHash(r, size)
	if err != nil {
		return errors.Errorf("generating SHA for agent binary %q: %w", version, err)
	}
	defer func() { _ = data.Close() }()

	if sha256 != encoded256 {
		return errors.Errorf(
			"SHA256 mismatch for agent binary %q: expected %q, got %q",
			version, sha256, encoded256,
		).Add(agentbinaryerrors.HashMismatch)
	}
	return s.add(ctx, data, version, size, encoded384)
}

func (s *ControllerAgentBinaryStore) add(
	ctx context.Context, r io.Reader,
	version coreagentbinary.Version,
	size int64, sha384 string,
) (resultErr error) {
	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return errors.Errorf("getting object store: %w", err)
	}

	path := generatePath(version, sha384)
	uuid, err := objectStore.PutAndCheckHash(ctx, path, r, size, sha384)
	switch {
	// Happens when the agent binary data already exists in the object store.
	case errors.Is(err, domainobjectstoreerrors.ErrHashAndSizeAlreadyExists):
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
		version.Arch,
	)

	err = s.state.RegisterAgentBinary(ctx, agentbinary.RegisterAgentBinaryArg{
		Version:         version.Number.String(),
		Arch:            version.Arch,
		ObjectStoreUUID: uuid,
	})
	if errors.IsOneOf(err,
		agentbinaryerrors.AgentBinaryImmutable,
		agentbinaryerrors.ObjectNotFound,
		coreerrors.NotSupported) {
		// We need to cleanup the newly added binary from the object store.
		// But we don't want to accidentally remove an existing binary if any unexpected errors occur.
		// The best we can do is to cleanup the binary for certain unknown errors.
		// If there is a retry, the uploaded binary will be picked up again and recorded in the database.
		if err := objectStore.Remove(ctx, path); err != nil && !errors.Is(err, domainobjectstoreerrors.ErrNotFound) {
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
