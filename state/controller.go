// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"

	jujucontroller "github.com/juju/juju/controller"
)

const (
	// controllerSettingsGlobalKey is the key for the controller and its settings.
	controllerSettingsGlobalKey = "controllerSettings"

	// controllerGlobalKey is the key for controller.
	controllerGlobalKey = "c"
)

// controllerKey will return the key for a given controller using the
// controller uuid and the controllerGlobalKey.
func controllerKey(controllerUUID string) string {
	return fmt.Sprintf("%s#%s", controllerGlobalKey, controllerUUID)
}

// ControllerConfig returns the config values for the controller.
func (st *State) ControllerConfig() (jujucontroller.Config, error) {
	settings, err := readSettings(st, controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}
