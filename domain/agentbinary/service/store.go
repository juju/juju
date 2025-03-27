// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"io"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/agentbinary"
	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes the interface that the cache state must implement.
type State interface {
	// Add adds a new agent binary's metadata to the database.
	// It always overwrites the metadata for the given version and arch if it already exists.
	// It returns [coreerrors.NotSupported] if the architecture is not found in the database.
	// It returns [coreerrors.NotFound] if object store UUID is not found in the database.
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

// Add adds a new agent binary to the object store and saves its metadata to the database.
// It always overwrites the binary in the store and the metadata in the database for the
// given version and arch if it already exists.
// It returns [coreerrors.NotSupported] if the architecture is not found in the database.
// It returns [coreerrors.NotFound] if object store UUID is not found in the database.
// It returns [coreerrors.NotValid] if the agent version is not valid.
func (s *AgentBinaryStore) Add(
	ctx context.Context, r io.Reader, version coreagentbinary.Version, size int64, sha384 string,
) (resultErr error) {
	if err := version.Validate(); err != nil {
		return errors.Errorf("agent version %q is not valid: %w", version, err)
	}

	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return errors.Errorf("getting object store: %w", err)
	}

	path := generatePath(version, sha384)
	// The object store ignores the already exist error, and always overwrites the object for the given path.
	// We just need to save the new object store UUID to the database.
	uuid, err := objectStore.PutAndCheckHash(ctx, path, r, size, sha384)
	if err != nil {
		return errors.Errorf("putting agent binary %q: %w", path, err)
	}
	defer func() {
		if resultErr == nil {
			return
		}
		if err := objectStore.Remove(ctx, path); err != nil && !errors.Is(err, objectstoreerrors.ErrNotFound) {
			s.logger.Errorf(ctx, "saving agent binary metadata %q failed, removing the binary from object store: %v", path, err)
		}
	}()
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
// It returns [coreerrors.NotSupported] if the architecture is not found in the database.
// It returns [coreerrors.NotFound] if object store UUID is not found in the database.
// It returns [coreerrors.NotValid] if the agent version is not valid.
func (s *AgentBinaryStore) AddWithSHA256(
	ctx context.Context, r io.Reader, version coreagentbinary.Version, size int64, sha256 string,
) (resultErr error) {
	// TODO: validates the sha256 and generate a new SHA384 in the store. It will be implemented in JUJU-7734.
	sha384 := sha256
	return s.Add(ctx, r, version, size, sha384)
}
