// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/changestream"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/objectstore"
	"github.com/juju/utils/v3"
)

// State describes retrieval and persistence methods for the coreobjectstore.
type State interface {
	// GetMetadata returns the persistence metadata for the specified path.
	GetMetadata(ctx context.Context, path string) (objectstore.Metadata, error)
	// PutMetadata adds a new specified path for the persistence metadata.
	PutMetadata(ctx context.Context, metadata objectstore.Metadata) error
	// RemoveMetadata removes the specified path for the persistence metadata.
	RemoveMetadata(ctx context.Context, path string) error
	// InitialWatchStatement returns the initial watch statement for the
	// persistence path.
	InitialWatchStatement() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, string) (watcher.StringsWatcher, error)
}

// Service provides the API for working with the coreobjectstore.
type Service struct {
	st             State
	watcherFactory WatcherFactory
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, watcherFactory WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: watcherFactory,
	}
}

// GetMetadata returns the persistence metadata for the specified path.
func (s *Service) GetMetadata(ctx context.Context, path string) (coreobjectstore.Metadata, error) {
	metadata, err := s.st.GetMetadata(ctx, path)
	if err != nil {
		return coreobjectstore.Metadata{}, fmt.Errorf("retrieving metadata %s: %w", path, domain.CoerceError(err))
	}
	return coreobjectstore.Metadata{
		Path: metadata.Path,
		Hash: metadata.Hash,
		Size: metadata.Size,
	}, nil
}

// PutMetadata adds a new specified path for the persistence metadata.
func (s *Service) PutMetadata(ctx context.Context, metadata coreobjectstore.Metadata) error {
	uuid, err := utils.NewUUID()
	if err != nil {
		return err
	}

	err = s.st.PutMetadata(ctx, objectstore.Metadata{
		UUID: uuid.String(),
		Hash: metadata.Hash,
		Path: metadata.Path,
		Size: metadata.Size,
	})
	if err != nil {
		return fmt.Errorf("adding path %s: %w", metadata.Path, err)
	}
	return nil
}

// RemoveMetadata removes the specified path for the persistence metadata.
func (s *Service) RemoveMetadata(ctx context.Context, path string) error {
	err := s.st.RemoveMetadata(ctx, path)
	if err != nil {
		return fmt.Errorf("removing path %s: %w", path, err)
	}
	return nil
}

// Watch returns a watcher that emits the path changes that either have been
// added or removed.
func (s *Service) Watch() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceWatcher(
		"objectstore",
		changestream.All,
		s.st.InitialWatchStatement(),
	)
}
