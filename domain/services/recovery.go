// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"context"

	"github.com/juju/juju/core/database"
	recoveryservice "github.com/juju/juju/domain/recovery/service"
	recoverystate "github.com/juju/juju/domain/recovery/state"
	"github.com/juju/juju/internal/errors"
)

// NewRecoveryService wires recovery persistence without starting workers or
// watchers. The caller's exclusive database session applies the model schema
// when opening a namespace and closes every runner after recovery finishes.
// The controller ID is the provisioned replacement node's identity, used
// when rebuilding object store placements during the load.
func NewRecoveryService(controller database.TxnRunnerFactory, openModel func(context.Context, string) (database.TxnRunner, error), controllerID string) (*recoveryservice.Service, error) {
	if controller == nil || openModel == nil {
		return nil, errors.New("recovery requires controller and model database factories")
	}
	controllerFactory := func(ctx context.Context) (database.TxnRunner, error) {
		runner, err := controller(ctx)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if runner == nil {
			return nil, errors.New("controller database factory returned nil runner")
		}
		return runner, nil
	}
	return recoveryservice.NewService(recoverystate.NewState(controllerFactory), func(ctx context.Context, model string) (recoveryservice.State, error) {
		runner, err := openModel(ctx, model)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if runner == nil {
			return nil, errors.New("model database factory returned nil runner")
		}
		return recoverystate.NewState(func(context.Context) (database.TxnRunner, error) {
			return runner, nil
		}), nil
	}, controllerID)
}
