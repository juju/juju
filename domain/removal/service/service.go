// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for entity removal.
type State interface {
	RelationState

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

	clock  clock.Clock
	logger logger.Logger
}

// GetAllJobs returns all removal jobs.
func (s *Service) GetAllJobs(ctx context.Context) (_ []removal.Job, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()
	jobs, err := s.st.GetAllJobs(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return jobs, nil
}

// ExecuteJob runs the appropriate removal logic for the input job.
// If the job is determined to have run successfully, we ensure that
// no removal job with the same UUID exists in the database.
func (s *Service) ExecuteJob(ctx context.Context, job removal.Job) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	switch job.RemovalType {
	case removal.RelationJob:
		err = s.processRelationRemovalJob(ctx, job)
	default:
		err = errors.Errorf("removal job type %q not supported", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotSupported)
	}

	if err != nil {
		if errors.Is(err, removalerrors.RemovalJobIncomplete) {
			return nil
		}
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
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:     st,
			clock:  clock,
			logger: logger,
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
