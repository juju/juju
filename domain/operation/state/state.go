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

// GetTaskUUIDByID returns the task UUID for the given task ID.
func (st *State) GetTaskUUIDByID(ctx context.Context, taskID string) (string, error) {
	return "", errors.NotImplemented
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

// NamespaceForTaskLogWatcher returns the name space for watching task
// log messages.
func (st *State) NamespaceForTaskLogWatcher() string {
	return "operation_task_log"
}
