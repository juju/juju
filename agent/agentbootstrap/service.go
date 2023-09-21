// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbootstrap

import (
	stdcontext "context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
)

// ControllerConfigService is used to obtain the controller config.
type ControllerConfigService interface {
	ControllerConfig(stdcontext.Context) (controller.Config, error)
}

// newControllerConfigService returns a new ControllerConfigService.
// Note: this side steps the need to use the full service factory. We don't
// have all the dependencies required to build one at the time we require the
// controller config. It is not recommended to use this pattern elsewhere.
func newControllerConfigService(runner database.TxnRunner) ControllerConfigService {
	return controllerconfigservice.NewService(
		controllerconfigstate.NewState(constTxnRunnerFactory(runner)),
		nil,
	)
}

// constTxnRunnerFactory always returns a TxnRunnerFactory that never fails.
func constTxnRunnerFactory[T database.TxnRunner](r T) database.TxnRunnerFactory {
	return func() (database.TxnRunner, error) {
		return r, nil
	}
}
