// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
)

// ModelConfig returns the complete config for the model represented
// by this state.
func (st *State) ModelConfig() (*config.Config, error) {
	controllerSettings, err := readSettings(st, controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelSettings, err := readSettings(st, settingsC, modelGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// The settings we use will consist of the controller settings
	// and any model settings which will take precedence of the
	// controller values.
	attrs := controllerSettings.Map()
	for k, v := range modelSettings.Map() {
		attrs[k] = v
	}
	return config.New(config.NoDefaults, attrs)
}

// checkModelConfig returns an error if the config is definitely invalid.
func checkModelConfig(cfg *config.Config) error {
	if cfg.AdminSecret() != "" {
		return errors.Errorf("admin-secret should never be written to the state")
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return errors.Errorf("agent-version must always be set in state")
	}
	return nil
}

func (st *State) buildAndValidateModelConfig(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) (validCfg *config.Config, err error) {
	newConfig, err := oldConfig.Apply(updateAttrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(removeAttrs) != 0 {
		newConfig, err = newConfig.Remove(removeAttrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	if err := checkModelConfig(newConfig); err != nil {
		return nil, errors.Trace(err)
	}
	return st.validate(newConfig, oldConfig)
}

type ValidateConfigFunc func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error

// UpdateModelConfig adds, updates or removes attributes in the current
// configuration of the model with the provided updateAttrs and
// removeAttrs.
func (st *State) UpdateModelConfig(updateAttrs map[string]interface{}, removeAttrs []string, additionalValidation ValidateConfigFunc) error {
	if len(updateAttrs)+len(removeAttrs) == 0 {
		return nil
	}

	// TODO(axw) 2013-12-6 #1167616
	// Ensure that the settings on disk have not changed
	// underneath us. The settings changes are actually
	// applied as a delta to what's on disk; if there has
	// been a concurrent update, the change may not be what
	// the user asked for.

	controllerSettings, err := readSettings(st, controllersC, controllerSettingsGlobalKey)
	if err != nil {
		return errors.Trace(err)
	}

	modelSettings, err := readSettings(st, settingsC, modelGlobalKey)
	if err != nil {
		return errors.Trace(err)
	}

	// Get the existing model config from state.
	oldConfig, err := st.ModelConfig()
	if err != nil {
		return errors.Trace(err)
	}
	if additionalValidation != nil {
		err = additionalValidation(updateAttrs, removeAttrs, oldConfig)
		if err != nil {
			return errors.Trace(err)
		}
	}
	validCfg, err := st.buildAndValidateModelConfig(updateAttrs, removeAttrs, oldConfig)
	if err != nil {
		return errors.Trace(err)
	}

	validAttrs := validCfg.AllAttrs()
	for k := range oldConfig.AllAttrs() {
		if _, ok := validAttrs[k]; !ok {
			controllerSettings.Delete(k)
			modelSettings.Delete(k)
		}
	}
	modelSettings.Update(validAttrs)

	// Remove any model settings that are the same as those of
	// its host controller.
	for k, v := range modelSettings.Map() {
		// Some attributes we want to retain regardless.
		if retainModelAttribute(k) {
			continue
		}
		if cv, ok := controllerSettings.Get(k); ok && v == cv {
			modelSettings.Delete(k)
		}
	}

	_, err = modelSettings.Write()
	return errors.Trace(err)
}
