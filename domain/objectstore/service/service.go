// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
)

// State describes retrieval and persistence methods for the objectstore.
type State interface {
	// GetMetadata returns the persistence metadata for the specified key.
	GetMetadata(ctx context.Context, path string) (objectstore.Metadata, error)
	// GetAllMetadata returns the list of persistence metadata.
	GetAllMetadata(ctx context.Context) (map[string]objectstore.Metadata, error)
	// PutMetadata adds a new specified key for the persistence metadata.
	PutMetadata(ctx context.Context, key string, metadata objectstore.Metadata) error
	// RemoveMetadata removes the specified key for the persistence path.
	RemoveMetadata(ctx context.Context, key string) error
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

// Service provides the API for working with the objectstore.
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

// GetMetadata returns the persistence metadata for the specified key.
func (s *Service) GetMetadata(ctx context.Context, key string) (objectstore.Metadata, error) {
	metadata, err := s.st.GetMetadata(ctx, key)
	if err != nil {
		return objectstore.Metadata{}, fmt.Errorf("retrieving metadata %s: %w", key, err)
	}
	return metadata, nil
}

// GetAllMetadata returns the list of persistence metadata.
func (s *Service) GetAllMetadata(ctx context.Context) (map[string]objectstore.Metadata, error) {
	p, err := s.st.GetAllMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting all metadata: %w", err)
	}
	return p, nil
}

// PutMetadata adds a new specified key for the persistence metadata.
func (s *Service) PutMetadata(ctx context.Context, key string, metadata objectstore.Metadata) error {
	err := s.st.PutMetadata(ctx, key, metadata)
	if err != nil {
		return fmt.Errorf("adding path %s: %w", key, err)
	}
	return nil
}

// RemoveMetadata removes the specified key for the persistence metadata.
func (s *Service) RemoveMetadata(ctx context.Context, key string) error {
	err := s.st.RemoveMetadata(ctx, key)
	if err != nil {
		return fmt.Errorf("removing path %s: %w", key, err)
	}
	return nil
}

// Watch returns a watcher that emits the key changes that either have been
// added or removed.
func (s *Service) Watch() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceWatcher(
		"objectstore",
		changestream.All,
		s.st.InitialWatchStatement(),
	)
}
