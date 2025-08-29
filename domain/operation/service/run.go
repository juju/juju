// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
)

// Run creates an operation with tasks for various machines and units, using the provided parameters.
func (s *Service) Run(ctx context.Context, args []operation.RunArgs) (operation.RunResult,
	error) {
	return operation.RunResult{}, errors.New("actions in Dqlite not supported")
}

// RunOnAllMachines creates an operation with tasks based on the provided parameters on all machines.
func (s *Service) RunOnAllMachines(ctx context.Context, args operation.TaskArgs) (operation.RunResult, error) {
	return operation.RunResult{}, errors.New("actions in Dqlite not supported")
}
