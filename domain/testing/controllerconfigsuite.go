// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/controllerconfig/bootstrap"
)

type ControllerTxnProvider interface {
	ControllerTxnRunner() coredatabase.TxnRunner
}

func SeedControllerConfig(
	c *gc.C,
	config controller.Config,
	controllerModelUUID coremodel.UUID,
	provider ControllerTxnProvider,
) controller.Config {
	err := bootstrap.InsertInitialControllerConfig(config, controllerModelUUID)(context.Background(), provider.ControllerTxnRunner(), noopTxnRunner{})
	c.Assert(err, jc.ErrorIsNil)
	return config
}

type noopTxnRunner struct{}

func (noopTxnRunner) Txn(context.Context, func(context.Context, *sqlair.TX) error) error {
	return coreerrors.NotImplemented
}

func (noopTxnRunner) StdTxn(context.Context, func(context.Context, *sql.Tx) error) error {
	return coreerrors.NotImplemented
}
