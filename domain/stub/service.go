// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
)

// StubService is a special service that collects temporary methods required for
// wiring together domains which not completely implemented or wired up.
//
// Given the temporary nature of this service, we have not implemented the full
// service/state layer indirection. Instead, the service directly uses a transaction
// runner.
//
// Deprecated: All methods here should be thrown away as soon as we're done with
// then.
type StubService struct {
	modelUUID       coremodel.UUID
	modelState      *domain.StateBase
	controllerState *domain.StateBase
}

// NewStubService returns a new StubService.
func NewStubService(
	modelUUID coremodel.UUID,
	controllerFactory database.TxnRunnerFactory,
	modelFactory database.TxnRunnerFactory,
) *StubService {
	return &StubService{
		modelUUID:       modelUUID,
		controllerState: domain.NewStateBase(controllerFactory),
		modelState:      domain.NewStateBase(modelFactory),
	}
}
