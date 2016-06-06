// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
)

// controllerGlobalKey is the key for the controller and its settings.
const controllerSettingsGlobalKey = "controllerSettings"

var controllerOnlyConfigAttributes = []string{
	config.ApiPort,
	config.StatePort,
	config.CACertKey,
	config.ControllerUUIDKey,
}

func controllerOnlyAttribute(attr string) bool {
	for _, a := range controllerOnlyConfigAttributes {
		if attr == a {
			return true
		}
	}
	return false
}

// retainModelConfigAttributes are those attributes we always want to
// store with the model, even if the controller settings hold the same values.
var retainModelConfigAttributes = []string{
	config.UUIDKey,
	config.AgentVersionKey,
}

func retainModelAttribute(attr string) bool {
	for _, a := range retainModelConfigAttributes {
		if attr == a {
			return true
		}
	}
	return false
}

func controllerAndModelConfig(currentControllerCfg, cfg map[string]interface{}) (controllerCfg, modelCfg map[string]interface{}) {
	controllerCfg = make(map[string]interface{})
	modelCfg = make(map[string]interface{})

	if len(currentControllerCfg) == 0 {
		// No controller config yet, so we are setting up a
		// new controller. The initial controller config is
		// that of the controller model.
		for attr, value := range cfg {
			controllerCfg[attr] = value
		}
	} else {
		// Copy across attributes only valid for the controller config.
		for _, attr := range controllerOnlyConfigAttributes {
			if v, ok := currentControllerCfg[attr]; ok {
				controllerCfg[attr] = v
			}
		}
	}

	// Add to the controller attributes any model attributes different to those
	// of the controller, and delete said attributes from the model.
	for attr := range cfg {
		if controllerOnlyAttribute(attr) {
			continue
		}
		modelValue := cfg[attr]
		if retainModelAttribute(attr) {
			modelCfg[attr] = modelValue
			continue
		}
		if cv, ok := controllerCfg[attr]; !ok || modelValue != cv {
			modelCfg[attr] = modelValue
		}
	}
	return controllerCfg, modelCfg
}

func (st *State) ControllerConfig() (map[string]interface{}, error) {
	settings, err := readSettings(st, controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}
