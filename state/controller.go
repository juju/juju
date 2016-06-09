// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
)

// controllerSettingsGlobalKey is the key for the controller and its settings.
const controllerSettingsGlobalKey = "controllerSettings"

func controllerOnlyAttribute(attr string) bool {
	for _, a := range config.ControllerOnlyConfigAttributes {
		if attr == a {
			return true
		}
	}
	return false
}

// controllerConfig returns the controller config attributes from cfg.
func controllerConfig(cfg map[string]interface{}) map[string]interface{} {
	controllerCfg := make(map[string]interface{})
	for _, attr := range config.ControllerOnlyConfigAttributes {
		if val, ok := cfg[attr]; ok {
			controllerCfg[attr] = val
		}
	}
	return controllerCfg
}

// modelConfig returns the model config attributes that result when we
// take what is required for the model and remove any attributes that
// are specifically controller related or are already present in the
// shared cloud config.
func modelConfig(sharedCloudCfg, cfg map[string]interface{}) map[string]interface{} {
	modelCfg := make(map[string]interface{})
	for attr, cfgValue := range cfg {
		if controllerOnlyAttribute(attr) {
			continue
		}
		if sharedValue, ok := sharedCloudCfg[attr]; !ok || sharedValue != cfgValue {
			modelCfg[attr] = cfgValue
		}
	}
	return modelCfg
}

// ControllerConfig returns the config values for the controller.
func (st *State) ControllerConfig() (map[string]interface{}, error) {
	settings, err := readSettings(st, controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}
