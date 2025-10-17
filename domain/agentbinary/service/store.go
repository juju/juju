// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	cryptosha256 "crypto/sha256"
	cryptosha512 "crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	intobjectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

type AgentBinaryStore struct {
	logger            logger.Logger
	st                AgentBinaryStoreState
	objectStoreGetter objectstore.NamespacedObjectStoreGetter
}

type AgentBinaryStoreState interface {
	// CheckAgentBinarySHA256Exists checks that the given sha256 sum exists as an agent
	// binary in the object store. This sha256 sum could exist as an object in
	// the object store but unless the association has been made this will
	// always return false.
	CheckAgentBinarySHA256Exists(ctx context.Context, sha256Sum string) (bool, error)

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
	GetAgentBinarySHA256(ctx context.Context, version coreagentbinary.Version, stream agentbinary.Stream) (bool, string, error)
}

// NewAgentBinaryStore returns a new instance of AgentBinaryStore.
func NewAgentBinaryStore(
	st AgentBinaryStoreState,
	logger logger.Logger,
	objectStoreGetter objectstore.NamespacedObjectStoreGetter,
) *AgentBinaryStore {
	return &AgentBinaryStore{
		st:                st,
		logger:            logger,
		objectStoreGetter: objectStoreGetter,
	}
}

// generatePath generates the path for the agent binary in the object store.
// The path is in the format of "agent-binaries/{version}-{arch}-{sha384}".
// We don't want to generate the path using the String() of the version
// because it may change in the future.
func generatePath(version coreagentbinary.Version, sha384 string) string {
	num := version.Number
	numberStr := fmt.Sprintf("%d.%d.%d", num.Major, num.Minor, num.Patch)
	if num.Tag != "" {
		numberStr = fmt.Sprintf("%d.%d-%s%d", num.Major, num.Minor, num.Tag, num.Patch)
	}
	if num.Build > 0 {
		numberStr += fmt.Sprintf(".%d", num.Build)
	}
	return fmt.Sprintf("agent-binaries/%s-%s-%s", numberStr, version.Arch, sha384)
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
func (s *AgentBinaryStore) AddAgentBinaryWithSHA256(
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
func (s *AgentBinaryStore) AddAgentBinaryWithSHA384(
	ctx context.Context,
	r io.Reader,
	version coreagentbinary.Version,
	size int64,
	sha384 string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := version.Validate(); err != nil {
		return errors.Errorf("agent version %q is not valid: %w", version, err)
	}
	return s.add(ctx, r, version, size, sha384)
}

// We use sha384 for validating the binary later in the concrete implementations.
func (s *AgentBinaryStore) add(
	ctx context.Context, r io.Reader,
	version coreagentbinary.Version,
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
		existingObjectUUID, err := s.st.GetObjectUUID(ctx, path)
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

	err = s.st.RegisterAgentBinary(ctx, agentbinary.RegisterAgentBinaryArg{
		Version:         version.Number.String(),
		Arch:            version.Arch,
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

// GetAgentBinaryUsingSHA256 returns the agent binary associated with the given
// SHA256 sum. The following errors can be expected:
// - [agentbinaryerrors.NotFound] when no agent binaries exist for the provided
// sha.
func (s *AgentBinaryStore) GetAgentBinaryUsingSHA256(
	ctx context.Context,
	sha256Sum string,
) (io.ReadCloser, int64, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// We check that this sha256 exists in the database and is associated with
	// agent binaries. If we don't do this the possibility exists to leak other
	// non-related objects out of the store via this interface.
	exists, err := s.st.CheckAgentBinarySHA256Exists(ctx, sha256Sum)
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
		return nil, 0, errors.Errorf("getting object store for agent binary %q: %w", sha256Sum, err)
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

// GetAgentBinaryWithSHA256 retrieves the agent binary corresponding to the given version
// and stream from simple stream.
// The caller is responsible for closing the returned reader.
//
// The following errors may be returned:
// - [domainagenterrors.NotFound] if the agent binary metadata does not exist.
func (s *AgentBinaryStore) GetAgentBinaryWithSHA256(
	ctx context.Context,
	ver coreagentbinary.Version,
	stream agentbinary.Stream,
) (io.ReadCloser, int64, string, error) {
	s.logger.Debugf(ctx, "retrieving agent binary from agent binary store for ver %q and stream %q", ver.String(), stream.String())

	hasAgentBinary, sha256Sum, err := s.st.GetAgentBinarySHA256(ctx, ver, stream)
	if err != nil {
		return nil, 0, "", errors.Errorf("checking availability of agent binary in controller store: %w", err)
	}

	if !hasAgentBinary {
		return nil, 0, "", errors.Errorf("no agent binary found for version %q", ver.String())
	}

	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return nil, 0, "", errors.Errorf("getting object store for agent binary %q: %w", sha256Sum, err)
	}
	reader, size, err := store.GetBySHA256(ctx, sha256Sum)
	if errors.Is(err, intobjectstoreerrors.ObjectNotFound) {
		return nil, 0, "", errors.New("agent binary not found in controller store").Add(agentbinaryerrors.NotFound)
	} else if err != nil {
		return nil, 0, "", errors.Errorf("getting agent binary with sha %q from controller object store: %w", sha256Sum, err)

	}
	return reader, size, sha256Sum, nil
}

func tmpCacheAndHash(r io.Reader, size int64) (_ io.ReadCloser, _ string, _ string, err error) {
	tmpFile, err := os.CreateTemp("", "juju-agent-binary-rehash*.tmp")
	if err != nil {
		return nil, "", "", errors.Capture(err)
	}

	defer func() {
		if err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}
	}()

	hasher256 := cryptosha256.New()
	hasher384 := cryptosha512.New384()

	tr := io.TeeReader(r, io.MultiWriter(hasher256, hasher384))
	written, err := io.Copy(tmpFile, tr)
	if err != nil {
		return nil, "", "", errors.Errorf("writing agent binary to temp file for re-computing hash: %w", err)
	}

	if written != size {
		return nil, "", "", errors.Errorf(
			"agent binary size mismatch: expected %d, got %d", size, written,
		).Add(coreerrors.NotValid)
	}

	encoded256 := hex.EncodeToString(hasher256.Sum(nil))
	encoded384 := hex.EncodeToString(hasher384.Sum(nil))

	if _, err = tmpFile.Seek(0, io.SeekStart); err != nil {
		return nil, "", "", errors.Capture(err)
	}
	cleanupFunc := func() { _ = os.Remove(tmpFile.Name()) }
	return &cleanupReadCloser{
		ReadCloser: tmpFile,
		cleanup:    cleanupFunc,
	}, encoded256, encoded384, nil
}

type cleanupReadCloser struct {
	io.ReadCloser
	cleanup func()
}

func (c *cleanupReadCloser) Close() error {
	err := c.ReadCloser.Close()
	if c.cleanup != nil {
		c.cleanup()
	}
	return err
}
