// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package romulus

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
)

// GetMeteringURLForModelCmd returns the controller-configured metering URL for
// a given model command.
var GetMeteringURLForModelCmd = getMeteringURLForModelCmdImpl

// GetMeteringURLForControllerCmd returns the controller-configured metering URL for
// a given controller command.
var GetMeteringURLForControllerCmd = getMeteringURLForControllerCmdImpl

func getMeteringURLForModelCmdImpl(c *modelcmd.ModelCommandBase) (string, error) {
	controllerAPIRoot, err := c.NewControllerAPIRoot()
	if err != nil {
		return "", errors.Trace(err)
	}
	controllerAPI := controller.NewClient(controllerAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return controllerCfg.MeteringURL(), nil
}

func getMeteringURLForControllerCmdImpl(c *modelcmd.ControllerCommandBase) (string, error) {
	controllerAPIRoot, err := c.NewAPIRoot()
	if err != nil {
		return "", errors.Trace(err)
	}
	controllerAPI := controller.NewClient(controllerAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return "", errors.Trace(err)
	}
	return controllerCfg.MeteringURL(), nil
}
