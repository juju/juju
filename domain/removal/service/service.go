// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for entity removal.
type State interface {
	RelationState

	// GetAllJobs returns all removal jobs.
	GetAllJobs(ctx context.Context) ([]removal.Job, error)
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
func (s *Service) GetAllJobs(ctx context.Context) ([]removal.Job, error) {
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
