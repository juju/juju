// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"github.com/juju/juju/v2/apiserver/common"
	"github.com/juju/juju/v2/apiserver/facade"
	"github.com/juju/juju/v2/rpc/params"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/apiserver/facade Authorizer,Context,Resources

// Facade allows model config manager clients to watch controller config changes and fetch controller config.
type Facade struct {
	auth                facade.Authorizer
	controllerConfigAPI *common.ControllerConfigAPI
}

func (f *Facade) ControllerConfig() (params.ControllerConfigResult, error) {
	return f.controllerConfigAPI.ControllerConfig()
}
