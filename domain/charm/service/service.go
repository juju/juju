// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"regexp"

	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/charm"
	charmerrors "github.com/juju/juju/domain/charm/errors"
	internalcharm "github.com/juju/juju/internal/charm"
)

var (
	// charmNameRegExp is a regular expression representing charm name.
	// This is the same one from the names package.
	charmNameSnippet = "[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*"
	charmNameRegExp  = regexp.MustCompile("^" + charmNameSnippet + "$")
)

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	NewUUIDsWatcher(
		namespace string, changeMask changestream.ChangeType,
	) (watcher.StringsWatcher, error)
}

// State describes retrieval and persistence methods for charms.
type State interface {
	// GetCharmUUID returns the charm UUID by the natural key.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmUUID(ctx context.Context, name string) (corecharm.UUID, error)

	// IsControllerCharm returns whether the charm is a controller charm.
	// If the charm does not exist, a NotFound error is returned.
	IsControllerCharm(ctx context.Context, uuid corecharm.UUID) (bool, error)

	// IsSubordinateCharm returns whether the charm is a subordinate charm.
	// If the charm does not exist, a NotFound error is returned.
	IsSubordinateCharm(ctx context.Context, charmUUID corecharm.UUID) (bool, error)

	// SupportsContainers returns whether the charm supports containers.
	// If the charm does not exist, a NotFound error is returned.
	SupportsContainers(ctx context.Context, charmUUID corecharm.UUID) (bool, error)

	// GetCharmMetadata returns the metadata for the charm using the charm UUID.
	// If the charm does not exist, a NotFound error is returned.
	GetCharmMetadata(ctx context.Context, charmUUID corecharm.UUID) (charm.Metadata, error)
}

// Service provides the API for working with charms.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// GetCharmUUID returns a charm UUID by name. It returns an error if the charm
// can not be found by the name.
// This can also be used as a cheap way to see if a charm exists without
// needing to load the charm metadata.
func (s *Service) GetCharmUUID(ctx context.Context, name string) (corecharm.UUID, error) {
	if !charmNameRegExp.MatchString(name) {
		return "", charmerrors.NameNotValid
	}

	return s.st.GetCharmUUID(ctx, name)
}

// IsControllerCharm returns whether the charm is a controller charm.
// This will return true if the charm is a controller charm, and false
// otherwise.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) IsControllerCharm(ctx context.Context, uuid corecharm.UUID) (bool, error) {
	if err := uuid.Validate(); err != nil {
		return false, fmt.Errorf("charm uuid: %w", err)
	}
	return s.st.IsControllerCharm(ctx, uuid)
}

// SupportsContainers returns whether the charm supports containers. This
// currently means that the charm is a kubernetes charm.
// This will return true if the charm is a controller charm, and false
// otherwise.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) SupportsContainers(ctx context.Context, uuid corecharm.UUID) (bool, error) {
	if err := uuid.Validate(); err != nil {
		return false, fmt.Errorf("charm uuid: %w", err)
	}
	return s.st.SupportsContainers(ctx, uuid)
}

// IsSubordinateCharm returns whether the charm is a subordinate charm.
// This will return true if the charm is a subordinate charm, and false
// otherwise.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) IsSubordinateCharm(ctx context.Context, uuid corecharm.UUID) (bool, error) {
	if err := uuid.Validate(); err != nil {
		return false, fmt.Errorf("charm uuid: %w", err)
	}
	return s.st.IsSubordinateCharm(ctx, uuid)
}

// GetCharmMetadata returns the metadata for the charm using the charm UUID.
// If the charm does not exist, a NotFound error is returned.
func (s *Service) GetCharmMetadata(ctx context.Context, uuid corecharm.UUID) (internalcharm.Meta, error) {
	if err := uuid.Validate(); err != nil {
		return internalcharm.Meta{}, fmt.Errorf("charm uuid: %w", err)
	}

	metadata, err := s.st.GetCharmMetadata(ctx, uuid)
	if err != nil {
		return internalcharm.Meta{}, errors.Trace(err)
	}

	return convertMetadata(metadata)
}

// WatchableService provides the API for working with charms and the
// ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(st State, watcherFactory WatcherFactory, logger logger.Logger) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchCharms returns a watcher that observes changes to charms.
func (s *WatchableService) WatchCharms() (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewUUIDsWatcher(
		"charm",
		changestream.All,
	)
}
