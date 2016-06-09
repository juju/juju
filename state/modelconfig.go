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
	model, err := st.Model()
	if err != nil {
		return nil, err
	}
	cloudSettings, err := readSettings(st, cloudSettingsC, cloudGlobalKey(model.Cloud()))
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelSettings, err := readSettings(st, settingsC, modelGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Callers still expect ModelConfig to contain all of the controller
	// settings attributes.
	attrs := controllerSettings.Map()

	// Merge in the cloud settings.
	for k, v := range cloudSettings.Map() {
		attrs[k] = v
	}

	// Finally, any model specific settings are added.
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

// checkCloudConfig returns an error if the shared config is definitely invalid.
func checkCloudConfig(attrs map[string]interface{}) error {
	if _, ok := attrs[config.AdminSecretKey]; ok {
		return errors.Errorf("cloud config cannot contain admin-secret")
	}
	if _, ok := attrs[config.AgentVersionKey]; ok {
		return errors.Errorf("cloud config cannot contain agent-version")
	}
	for _, attrName := range config.ControllerOnlyConfigAttributes {
		if _, ok := attrs[attrName]; ok {
			return errors.Errorf("cloud config cannot contain controller attribute %q", attrName)
		}
	}
	return nil
}

func (st *State) buildAndValidateModelConfig(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) (validCfg *config.Config, err error) {
	for attr := range updateAttrs {
		if controllerOnlyAttribute(attr) {
			return nil, errors.Errorf("cannot set controller attribute %q on a model", attr)
		}
	}
	//TODO(wallyworld) if/when cloud config becomes mutable, we must check for concurrent changes
	// when writing config to ensure the validation we do here remains true
	cloudConfig, err := st.CloudConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for attr := range updateAttrs {
		if _, ok := cloudConfig[attr]; ok {
			return nil, errors.Errorf("cannot set shared cloud attribute %q on a model", attr)
		}
	}
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
			modelSettings.Delete(k)
		}
	}

	// Remove any attributes that are the same as what's in cloud config.
	cloudAttrs, err := st.CloudConfig()
	if err != nil {
		return errors.Trace(err)
	}
	for attr, sharedValue := range cloudAttrs {
		if newValue, ok := validAttrs[attr]; ok && newValue == sharedValue {
			delete(validAttrs, attr)
			modelSettings.Delete(attr)
		}
	}

	modelSettings.Update(validAttrs)
	_, err = modelSettings.Write()
	return errors.Trace(err)
}
