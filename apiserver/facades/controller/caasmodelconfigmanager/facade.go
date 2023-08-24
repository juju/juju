// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"context"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
)

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
type Facade struct {
	auth                facade.Authorizer
	controllerConfigAPI *common.ControllerConfigAPI
}

func (f *Facade) ControllerConfig(ctx context.Context) (params.ControllerConfigResult, error) {
	return f.controllerConfigAPI.ControllerConfig(ctx)
}
