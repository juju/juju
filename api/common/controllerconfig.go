// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/rpc/params"
)

// ControllerConfigAPI provides common client-side API functions
// to call into apiserver.common.ControllerConfig.
type ControllerConfigAPI struct {
	facade base.FacadeCaller
}

// NewControllerConfig creates a ControllerConfig on the specified facade,
// and uses this name when calling through the caller.
func NewControllerConfig(facade base.FacadeCaller) *ControllerConfigAPI {
	return &ControllerConfigAPI{facade}
}

// ControllerConfig returns the current controller configuration.
func (e *ControllerConfigAPI) ControllerConfig() (controller.Config, error) {
	var result params.ControllerConfigResult
	err := e.facade.FacadeCall(context.TODO(), "ControllerConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	return controller.Config(result.Config), nil
}
