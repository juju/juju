// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	jujucontroller "github.com/juju/juju/controller"
)

const (
	// controllerSettingsGlobalKey is the key for the controller and its settings.
	controllerSettingsGlobalKey = "controllerSettings"

	// defaultModelSettingsGlobalKey is the key for default settings shared across models.
	defaultModelSettingsGlobalKey = "defaultModelSettings"
)

func controllerOnlyAttribute(attr string) bool {
	for _, a := range jujucontroller.ControllerOnlyConfigAttributes {
		// TODO(wallyworld) - we don't want to add controller uuid to models long term
		if a == jujucontroller.ControllerUUIDKey {
			return false
		}
		if attr == a {
			return true
		}
	}
	return false
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
func (st *State) ControllerConfig() (jujucontroller.Config, error) {
	settings, err := readSettings(st, controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}

// CloudConfig returns the config values shared across models.
func (st *State) CloudConfig() (map[string]interface{}, error) {
	settings, err := readSettings(st, controllersC, defaultModelSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}
