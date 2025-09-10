// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
)

// StartExecOperation creates an exec operation with tasks for various machines and units,
// using the provided parameters.
func (s *Service) StartExecOperation(ctx context.Context, target operation.Receivers,
	args operation.ExecArgs) (operation.RunResult, error) {
	return operation.RunResult{}, errors.New("operations in Dqlite not supported")
}

// StartExecOperationOnAllMachines creates an exec operation with tasks based
// on the provided parameters on all machines.
func (s *Service) StartExecOperationOnAllMachines(ctx context.Context, args operation.ExecArgs) (operation.RunResult, error) {
	return operation.RunResult{}, errors.New("operations in Dqlite not supported")
}

// StartActionOperation creates an action operation with tasks for various
// units using the provided parameters.
func (s *Service) StartActionOperation(ctx context.Context,
	target []operation.ActionReceiver,
	args operation.TaskArgs) (operation.RunResult, error) {
	return operation.RunResult{}, errors.New("operations in Dqlite not supported")
}
