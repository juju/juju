// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/uuid"
)

// State describes the methods that a state implementation must provide to manage
// operation for a model.
type State interface {
	// CancelTask attempts to cancel an enqueued task, identified by its
	// ID.
	CancelTask(ctx context.Context, taskID string) (operation.Task, error)
	// GetIDAndStatusIfReceiversTask returns the task ID and status of
	// the given task UUID if the receiver UUID matches. Returns TaskNotFound
	// if the task does not belong to the provided receiver.
	GetIDAndStatusIfReceiversTask(
		ctx context.Context,
		receiverUUID, taskUUID string,
	) (string, corestatus.Status, error)
	// GetTask returns the task identified by its ID.
	// It returns the task as well as the path to its output in the object store,
	// if any. It's up to the caller to retrieve the actual output from the object
	// store.
	GetTask(ctx context.Context, taskID string) (operation.Task, *string, error)
	// NamespaceForTaskAbortingWatcher returns the name space (table) to be
	// for the TaskAbortingWatcher.
	// GetUUIDsAndIDsForAbortingTaskOfReceiver returns a map of task UUID to
	// task ID for any task with the given receiver UUID and has a status of
	// Aborting
	GetUUIDsAndIDsForAbortingTaskOfReceiver(
		ctx context.Context,
		receiverUUID uuid.UUID,
	) (map[string]string, error)
	NamespaceForTaskAbortingWatcher() string
	// TaskStatus returns the status of the given task.
	TaskStatus(ctx context.Context, taskUUID string) (corestatus.Status, error)
}

// Service provides the API for managing operation
type Service struct {
	st                State
	clock             clock.Clock
	logger            logger.Logger
	objectStoreGetter objectstore.ModelObjectStoreGetter
}

// NewService returns a new Service for managing operation
func NewService(
	st State,
	clock clock.Clock,
	logger logger.Logger,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
) *Service {
	return &Service{
		st:                st,
		clock:             clock,
		logger:            logger,
		objectStoreGetter: objectStoreGetter,
	}
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new watcher that receives changes from the
	// input base watcher's db/queue.
	NewNamespaceMapperWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// WatchableService defines a service for interacting with the underlying state
// and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new Service for interacting with the
// underlying state and the ability to create watchers.
func NewWatchableService(
	st State,
	clock clock.Clock,
	logger logger.Logger,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
	wf WatcherFactory,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:                st,
			clock:             clock,
			objectStoreGetter: objectStoreGetter,
			logger:            logger,
		},
		watcherFactory: wf,
	}
}
