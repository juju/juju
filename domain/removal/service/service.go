// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// Provider describes methods for interacting with the provider.
type Provider interface {
	// ReleaseContainerAddresses releases the previously allocated
	// addresses matching the interface details passed in.
	ReleaseContainerAddresses(ctx context.Context, interfaces []string) error
}

// State describes retrieval and persistence methods for entity removal.
type State interface {
	RelationState
	UnitState
	ApplicationState
	MachineState

	// GetAllJobs returns all removal jobs.
	GetAllJobs(ctx context.Context) ([]removal.Job, error)

	// DeleteJob deletes a removal record under the assumption
	// that it was executed successfully.
	DeleteJob(ctx context.Context, jUUID string) error
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewUUIDsWatcher returns a watcher that emits the UUIDs for changes to the
	// input table name that match the input mask.
	NewUUIDsWatcher(tableName string, changeMask changestream.ChangeType) (watcher.StringsWatcher, error)
}

// Service provides the API for working with entity removal.
type Service struct {
	st State

	leadershipRevoker leadership.Revoker
	provider          providertracker.ProviderGetter[Provider]

	clock  clock.Clock
	logger logger.Logger
}

// GetAllJobs returns all removal jobs.
func (s *Service) GetAllJobs(ctx context.Context) ([]removal.Job, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	jobs, err := s.st.GetAllJobs(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return jobs, nil
}

// ExecuteJob runs the appropriate removal logic for the input job.
// If the job is determined to have run successfully, we ensure that
// no removal job with the same UUID exists in the database.
func (s *Service) ExecuteJob(ctx context.Context, job removal.Job) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	var err error
	switch job.RemovalType {
	case removal.RelationJob:
		err = s.processRelationRemovalJob(ctx, job)

	case removal.UnitJob:
		err = s.processUnitRemovalJob(ctx, job)

	case removal.ApplicationJob:
		err = s.processApplicationRemovalJob(ctx, job)

	case removal.MachineJob:
		err = s.processMachineRemovalJob(ctx, job)

	default:
		err = errors.Errorf("removal job type %q not supported", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotSupported)
	}

	if errors.Is(err, removalerrors.RemovalJobIncomplete) {
		return nil
	}
	if err != nil {
		return errors.Capture(err)
	}

	if err := s.st.DeleteJob(ctx, job.UUID.String()); err != nil {
		return errors.Errorf("completing removal %q: %w", job.UUID.String(), err)
	}
	return nil
}

// WatchableService provides the API for working with entity removal,
// including the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService creates a new WatchableService
// for working with entity removal.
func NewWatchableService(
	st State,
	watcherFactory WatcherFactory,
	leadershipRevoker leadership.Revoker,
	provider providertracker.ProviderGetter[Provider],
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:                st,
			leadershipRevoker: leadershipRevoker,
			provider:          provider,
			clock:             clock,
			logger:            logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchRemovals watches for scheduled removal jobs.
// The returned watcher emits the UUIDs of any inserted or updated jobs.
func (s *WatchableService) WatchRemovals() (watcher.StringsWatcher, error) {
	w, err := s.watcherFactory.NewUUIDsWatcher(s.st.NamespaceForWatchRemovals(), changestream.Changed)
	if err != nil {
		return nil, errors.Errorf("creating watcher for removals: %w", err)
	}
	return w, nil
}
