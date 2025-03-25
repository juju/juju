// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"io"

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

// Service provides the API for working with agent binaries.
type Service struct {
	st                State
	logger            logger.Logger
	objectStoreGetter objectstore.ModelObjectStoreGetter
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	logger logger.Logger,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
) *Service {
	return &Service{
		st:                st,
		logger:            logger,
		objectStoreGetter: objectStoreGetter,
	}
}

// Add adds a new agent binary to the object store and saves its metadata to the database.
// It always overwrites the binary in the store and the metadata in the database for the
// given version and arch if it already exists.
// It returns [coreerrors.NotSupported] if the architecture is not found in the database.
// It returns [coreerrors.NotFound] if object store UUID is not found in the database.
// It returns [coreerrors.NotValid] if the agent version is not valid.
func (s *Service) Add(ctx context.Context, r io.Reader, metadata Metadata) (resultErr error) {
	if err := metadata.Version.Validate(); err != nil {
		return errors.Errorf("agent version %q is not valid: %w", metadata.Version, err)
	}

	objectStore, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return errors.Errorf("getting object store: %w", err)
	}

	// TODO: do we wanna to use 384 instead of 256 for binary. It will be implemented in JUJU-7734.
	path := fmt.Sprintf("tools/%s-%s-%s", metadata.Number, metadata.Arch, metadata.SHA384)
	// The object store ignores the already exist error, and always overwrites the object for the given path.
	// We just need to save the new object store UUID to the database.
	uuid, err := objectStore.PutAndCheckHash(ctx, path, r, metadata.Size, metadata.SHA384)
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
		Version:         metadata.Version.Number.String(),
		Arch:            metadata.Arch,
		ObjectStoreUUID: uuid,
	}); err != nil {
		return errors.Errorf("saving agent binary metadata for %q: %w", path, err)
	}
	return nil
}
