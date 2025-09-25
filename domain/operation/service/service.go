// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock"

	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/objectstore"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/domain/operation/internal"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// State describes the methods that a state implementation must provide to manage
// operation for a model.
type State interface {
	// CancelTask attempts to cancel an enqueued task, identified by its
	// ID.
	CancelTask(ctx context.Context, taskID string) (operation.Task, error)

	// FilterTaskUUIDsForMachine returns a list of task IDs that corresponds to the
	// filtered list of task UUIDs from the provided list that target the given
	// machine uuid, including the ones in pending status.
	FilterTaskUUIDsForMachine(ctx context.Context, tUUIDs []string, machineUUID string) ([]string, error)

	// FiltertTaskUUIDsForUnit returns a list of task IDs that corresponds to the
	// filtered list of task UUIDs from the provided list that target the given
	// unit uuid and are not in pending status.
	FilterTaskUUIDsForUnit(ctx context.Context, tUUIDs []string, unitUUID string) ([]string, error)

	// GetUnitUUIDByName returns the unit UUID for the given unit name.
	GetUnitUUIDByName(ctx context.Context, n coreunit.Name) (string, error)

	// GetReceiverFromTaskID returns a receiver string for the task identified.
	// The string should satisfy the ActionReceiverTag type.
	GetReceiverFromTaskID(ctx context.Context, taskID string) (string, error)

	// GetMachineUUIDByName returns the machine UUID for the given machine name.
	GetMachineUUIDByName(ctx context.Context, n coremachine.Name) (string, error)

	// GetIDsForAbortingTaskOfReceiver returns a slice of task IDs for any
	// task with the given receiver UUID and having a status of Aborting.
	GetIDsForAbortingTaskOfReceiver(
		ctx context.Context,
		receiverUUID internaluuid.UUID,
	) ([]string, error)

	// GetTask returns the task identified by its ID.
	// It returns the task as well as the path to its output in the object store,
	// if any. It's up to the caller to retrieve the actual output from the object
	// store.
	GetTask(ctx context.Context, taskID string) (operation.Task, *string, error)

	// GetMachineTaskIDsWithStatus retrieves all task IDs associated with a specific
	//machine and filtered by a given status.
	GetMachineTaskIDsWithStatus(ctx context.Context, machineName string, statusFilter string) ([]string,
		error)

	// GetTaskIDsByUUIDsFilteredByReceiverUUID returns task IDs of the tasks
	// provided having the given receiverUUID.
	GetTaskIDsByUUIDsFilteredByReceiverUUID(
		ctx context.Context,
		receiverUUID internaluuid.UUID,
		taskUUIDs []string,
	) ([]string, error)

	// GetTaskUUIDByID returns the task UUID for the given task ID.
	GetTaskUUIDByID(ctx context.Context, taskID string) (string, error)

	// GetLatestTaskLogsByUUID returns a slice of log messages newer than
	// the cursor provided. A new cursor is returned with the latest value.
	GetLatestTaskLogsByUUID(
		ctx context.Context,
		taskUUID string,
		cursor time.Time,
	) ([]internal.TaskLogMessage, time.Time, error)

	// FinishTask updates the task status to an inactive status value
	// and saves a reference to its results in the object store. If the
	// task's operation has no active tasks, mark the completed time for
	// the operation.
	FinishTask(context.Context, internal.CompletedTask) error

	// InitialWatchStatementUnitTask returns the namespace (table) and an
	// initial query function which returns the list of non-pending task ids for
	// the given unit.
	InitialWatchStatementUnitTask() (string, string)

	// InitialWatchStatementMachineTask returns the namespace and an initial
	// query function which returns the list of task ids for the given machine.
	InitialWatchStatementMachineTask() (string, string)

	// LogTaskMessage stores the message for the given task ID.
	LogTaskMessage(ctx context.Context, taskID, message string) error

	// StartTask sets the task start time and updates the status to running.
	// The following errors may be returned:
	// - [operationerrors.TaskNotFound] if the task does not exist.
	// - [operationerrors.TaskNotPending] if the task is not pending.
	StartTask(ctx context.Context, taskID string) error

	// GetTaskStatusByID returns the status of the given task.
	GetTaskStatusByID(ctx context.Context, taskID string) (string, error)

	// NamespaceForTaskAbortingWatcher returns the name space to be used
	// for the TaskAbortingWatcher.
	NamespaceForTaskAbortingWatcher() string

	// NamespaceForTaskLogWatcher returns the name space for watching task
	// log messages.
	NamespaceForTaskLogWatcher() string

	// PruneOperations deletes operations that are older than maxAge and larger than maxSizeMB (in megabytes).
	// It returns the paths from objectStore that should be freed
	PruneOperations(ctx context.Context, maxAge time.Duration, maxSizeMB int) ([]string, error)

	// AddExecOperation creates an exec operation with tasks for various machines
	// and units, using the provided parameters.
	AddExecOperation(ctx context.Context, operationUUID internaluuid.UUID, target internal.ReceiversWithResolvedLeaders, args operation.ExecArgs) (operation.RunResult, error)

	// AddExecOperationOnAllMachines creates an exec operation with tasks based
	// on the provided parameters on all machines.
	AddExecOperationOnAllMachines(ctx context.Context, operationUUID internaluuid.UUID, args operation.ExecArgs) (operation.RunResult, error)

	// AddActionOperation creates an action operation with tasks for various
	// units using the provided parameters.
	AddActionOperation(ctx context.Context, operationUUID internaluuid.UUID, targetUnits []coreunit.Name, args operation.TaskArgs) (operation.RunResult, error)
}

// LeadershipService describes the methods for managing (application)
// leadership.
type LeadershipService interface {
	// ApplicationLeader returns the leader unit name for the input application.
	ApplicationLeader(appName string) (string, error)
}

// Service provides the API for managing operation
type Service struct {
	st                State
	clock             clock.Clock
	logger            logger.Logger
	objectStoreGetter objectstore.ModelObjectStoreGetter
	leadershipService LeadershipService
}

// NewService returns a new Service for managing operation
func NewService(
	st State,
	clock clock.Clock,
	logger logger.Logger,
	objectStoreGetter objectstore.ModelObjectStoreGetter,
	leadershipService LeadershipService,
) *Service {
	return &Service{
		st:                st,
		clock:             clock,
		logger:            logger,
		objectStoreGetter: objectStoreGetter,
		leadershipService: leadershipService,
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
	leadershipService LeadershipService,
	wf WatcherFactory,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:                st,
			clock:             clock,
			objectStoreGetter: objectStoreGetter,
			leadershipService: leadershipService,
			logger:            logger,
		},
		watcherFactory: wf,
	}
}
