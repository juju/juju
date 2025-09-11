// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/operation/internal"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetIDsForAbortingTaskOfReceiver returns a slice of task IDs for any
// task with the given receiver UUID and having a status of Aborting.
func (st *State) GetIDsForAbortingTaskOfReceiver(
	ctx context.Context,
	receiverUUID internaluuid.UUID,
) ([]string, error) {
	return nil, errors.NotImplemented
}

// GetPaginatedTaskLogsByUUID returns a paginated slice of log messages and
// the page number.
func (st *State) GetPaginatedTaskLogsByUUID(
	ctx context.Context,
	taskUUID string,
	page int,
) ([]internal.TaskLogMessage, int, error) {
	// TODO: return log messages in order from oldest to newest.
	return nil, 0, errors.NotImplemented
}

// GetTaskIDsByUUIDsFilteredByReceiverUUID returns task IDs of the tasks
// provided having the given receiverUUID.
func (st *State) GetTaskIDsByUUIDsFilteredByReceiverUUID(
	ctx context.Context,
	receiverUUID internaluuid.UUID,
	taskUUIDs []string,
) ([]string, error) {
	return nil, errors.NotImplemented
}

// GetTaskUUIDByID returns the task UUID for the given task ID.
func (st *State) GetTaskUUIDByID(ctx context.Context, taskID string) (string, error) {
	return "", errors.NotImplemented
}

// NamespaceForTaskAbortingWatcher returns the namespace for
// the TaskAbortingWatcher. This custom namespace only notifies
// if an operation task's status is set to ABORTING.
func (st *State) NamespaceForTaskAbortingWatcher() string {
	return "custom_operation_task_status_aborting"
}

// NamespaceForTaskLogWatcher returns the name space for watching task
// log messages.
func (st *State) NamespaceForTaskLogWatcher() string {
	return "operation_task_log"
}
