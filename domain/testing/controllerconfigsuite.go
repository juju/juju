// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/controllerconfig/bootstrap"
)

type ControllerTxnProvider interface {
	ControllerTxnRunner() coredatabase.TxnRunner
}

func SeedControllerConfig(
	c *gc.C,
	config controller.Config,
	provider ControllerTxnProvider,
) controller.Config {
	err := bootstrap.InsertInitialControllerConfig(config)(context.Background(), provider.ControllerTxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	return config
}
