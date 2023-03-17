// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm

import (
	"github.com/juju/juju/api/agent/controllercharm"
	"github.com/juju/juju/api/base"
)

func NewFacade(apiCaller base.APICaller) (Facade, error) {
	facade := controllercharm.NewClient(apiCaller)
	return facade, nil
}
