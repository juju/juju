// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
)

// State describes retrieval and persistence methods for the objectstore.
type State interface {
	// GetPath returns the persistence path for the specified key.
	GetPath(ctx context.Context, path string) (string, error)
	// ListPaths returns the list of persistence paths.
	ListPaths(ctx context.Context) (map[string]string, error)
	// PutPath adds a new specified key for the persistence path.
	PutPath(ctx context.Context, key, path string) error
	// RemovePath removes the specified key for the persistence path.
	RemovePath(ctx context.Context, key string) error
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

// GetPath returns the persistence path for the specified key.
func (s *Service) GetPath(ctx context.Context, key string) (string, error) {
	p, err := s.st.GetPath(ctx, key)
	if err != nil {
		return "", errors.Annotatef(err, "retrieving path %s", key)
	}
	return p, nil
}

// ListPaths returns the list of persistence paths.
func (s *Service) ListPaths(ctx context.Context) (map[string]string, error) {
	p, err := s.st.ListPaths(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "listing paths")
	}
	return p, nil
}

// PutPath adds a new specified key for the persistence path.
func (s *Service) PutPath(ctx context.Context, key, path string) error {
	err := s.st.PutPath(ctx, key, path)
	return errors.Annotatef(err, "adding path %s", key)
}

// RemovePath removes the specified key for the persistence path.
func (s *Service) RemovePath(ctx context.Context, key string) error {
	err := s.st.RemovePath(ctx, key)
	return errors.Annotatef(err, "removing path %s", key)
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
