// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// WatcherFactory instances return a watcher for a specified credential UUID,
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// State describes retrieval and persistence methods for storage.
type State interface {
	ProviderState

	// CreateCloud creates the input cloud entity and provides Admin
	// permissions for the owner.
	CreateCloud(ctx context.Context, owner user.Name, cloudUUID string, cloud cloud.Cloud) error

	// UpdateCloud updates the input cloud entity.
	UpdateCloud(context.Context, cloud.Cloud) error

	// DeleteCloud deletes the input cloud entity.
	DeleteCloud(context.Context, string) error

	// ListClouds returns the clouds matching the optional filter.
	ListClouds(context.Context) ([]cloud.Cloud, error)
}

// Service provides the API for working with clouds.
type Service struct {
	st State
}

// CreateCloud creates the input cloud entity and provides Admin
// permissions for the owner.
func (s *Service) CreateCloud(ctx context.Context, owner user.Name, cloud cloud.Cloud) error {
	credUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Errorf("creating uuid for cloud %q: %w", cloud.Name, err)
	}
	err = s.st.CreateCloud(ctx, owner, credUUID.String(), cloud)
	if err != nil {
		return errors.Errorf("creating cloud %q: %w", cloud.Name, err)
	}
	return nil
}

// UpdateCloud updates the specified cloud.
func (s *Service) UpdateCloud(ctx context.Context, cloud cloud.Cloud) error {
	err := s.st.UpdateCloud(ctx, cloud)
	if err != nil {
		return errors.Errorf("updating cloud %q: %w", cloud.Name, err)
	}
	return nil
}

// DeleteCloud removes the specified cloud.
func (s *Service) DeleteCloud(ctx context.Context, name string) error {
	err := s.st.DeleteCloud(ctx, name)
	if err != nil {
		return errors.Errorf("deleting cloud %q: %w", name, err)
	}
	return nil
}

// ListAll returns all the clouds.
func (s *Service) ListAll(ctx context.Context) ([]cloud.Cloud, error) {
	all, err := s.st.ListClouds(ctx)
	return all, errors.Capture(err)
}

// Cloud returns the named cloud.
func (s *Service) Cloud(ctx context.Context, name string) (*cloud.Cloud, error) {
	cloud, err := s.st.Cloud(ctx, name)
	return cloud, errors.Capture(err)
}

// GetModelCloud looks up the model's cloud and region.
// The following error types can be expected:
// - [modelerrors.NotFound]: when the model does not exist.
// - [clouderrors.NotFound]: when the cloud does not exist.
func (s *Service) GetModelCloud(ctx context.Context, uuid model.UUID) (*cloud.Cloud, string, error) {
	cld, region, err := s.st.GetModelCloud(ctx, uuid)
	if err != nil {
		return nil, "", errors.Errorf("getiing cloud and region for model %q: %w", uuid, err)
	}
	return cld, region, nil
}

// WatchableService defines a service for interacting with the underlying state
// and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new service reference wrapping the
// input state and watcher factory.
func NewWatchableService(st State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchCloud returns a watcher that observes changes to the specified cloud.
func (s *WatchableService) WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error) {
	return s.st.WatchCloud(ctx, s.watcherFactory.NewNotifyWatcher, name)
}
