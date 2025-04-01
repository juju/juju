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
	"github.com/juju/juju/domain/agentbinary"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes the interface that the cache state must implement.
type State interface {
	// Add adds a new agent binary's metadata to the database.
	// [agentbinaryerrors.AlreadyExists] when the provided agent binary already
	// exists.
	// [agentbinaryerrors.ObjectNotFound] when no object exists that matches
	// this agent binary.
	// [coreerrors.NotSupported] if the architecture is not supported by the
	// state layer.
	Add(ctx context.Context, metadata agentbinary.Metadata) error
}

// AgentBinaryStore provides the API for working with agent binaries.
type AgentBinaryStore struct {
	st                State
	logger            logger.Logger
	objectStoreGetter objectstore.ModelObjectStoreGetter
}

// NewAgentBinaryStore returns a new instance of AgentBinaryStore.
func NewAgentBinaryStore(
	st State,
	logger logger.Logger,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
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

// Add adds a new agent binary to the object store and saves its metadata to the
// database. The following errors can be returned:
// - [coreerrors.NotSupported] if the architecture is not supported.
// - [github.com/juju/juju/domain/agentbinary/errors.AlreadyExists] if an agent
// binary already exists for this version and architecture.
// - [github.com/juju/juju/domain/agentbinary/errors.ObjectNotFound] if there
// was a problem referencing the agent binary metadata with the previously saved
// binary object. This error should be considered an internal problem. It is
// discussed here to make the caller aware of future problems.
// - [coreerrors.NotValid] when the agent version is not considered valid.
func (s *AgentBinaryStore) Add(
	ctx context.Context,
	r io.Reader,
	version coreagentbinary.Version,
	size int64,
	sha384 string,
) (resultErr error) {
	if err := version.Validate(); err != nil {
		return errors.Errorf("agent version %q is not valid: %w", version, err)
	}
	return s.add(ctx, r, version, size, sha384)
}

func (s *AgentBinaryStore) add(
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
	defer func() {
		if resultErr == nil ||
			// We don't want to remove the pre-existing binary if the error is
			// AlreadyExists.
			errors.Is(resultErr, agentbinaryerrors.AlreadyExists) {
			return
		}
		// We need to remove the binary from the object store if any error occurs.
		// We need to defer this before the below error checck is because the agent binary
		// might be saved already but the metadata or path saving failed.
		// In that case, we need to remove the binary from the object store.
		if err := objectStore.Remove(ctx, path); err != nil && !errors.Is(err, objectstoreerrors.ErrNotFound) {
			s.logger.Errorf(ctx, "saving agent binary metadata %q failed, removing the binary from object store: %v", path, err)
		}
	}()
	if errors.Is(err, objectstoreerrors.ErrPathAlreadyExistsDifferentHash) {
		// This means that the binary already exists in the object store.
		return errors.Errorf(
			"agent binary of %q already exists in the object store", version,
		).Add(
			agentbinaryerrors.AlreadyExists,
		)
	}
	if err != nil {
		return errors.Errorf("putting agent binary %q: %w", path, err)
	}
	if err := s.st.Add(ctx, agentbinary.Metadata{
		Version:         version.Number.String(),
		Arch:            version.Arch,
		ObjectStoreUUID: uuid,
	}); err != nil {
		return errors.Errorf("saving agent binary metadata for %q: %w", path, err)
	}
	return nil
}

// AddWithSHA256 adds a new agent binary to the object store and saves its metadata to the database.
// It always overwrites the binary in the store and the metadata in the database for the
// given version and arch if it already exists.
// It accepts the SHA256 hash of the binary.
// The following errors can be returned:
// - [coreerrors.NotSupported] if the architecture is not supported.
// - [github.com/juju/juju/domain/agentbinary/errors.AlreadyExists] if an agent
// binary already exists for this version and architecture.
// - [github.com/juju/juju/domain/agentbinary/errors.ObjectNotFound] if there
// was a problem referencing the agent binary metadata with the previously saved
// binary object. This error should be considered an internal problem. It is
// discussed here to make the caller aware of future problems.
// - [coreerrors.NotValid] if the agent version is not valid or the SHA256 hash doesn't match the generated hash.
func (s *AgentBinaryStore) AddWithSHA256(
	ctx context.Context, r io.Reader,
	version coreagentbinary.Version,
	size int64, sha256 string,
) (resultErr error) {
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
		).Add(coreerrors.NotValid)
	}
	return s.add(ctx, data, version, size, encoded384)
}

type cleanupCloser struct {
	io.ReadCloser
	cleanupFunc func()
}

func (c *cleanupCloser) Close() error {
	err := c.ReadCloser.Close()
	if c.cleanupFunc != nil {
		c.cleanupFunc()
	}
	return err
}

func tmpCacheAndHash(r io.Reader, size int64) (_ io.ReadCloser, _ string, _ string, err error) {
	tmpFile, err := os.CreateTemp("", "jujuagentbinaries*.tmp")
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
		return nil, "", "", errors.Capture(err)
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
	return &cleanupCloser{tmpFile, cleanupFunc}, encoded256, encoded384, nil
}
