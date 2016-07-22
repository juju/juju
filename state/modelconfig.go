// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
)

type attrValues map[string]interface{}

var disallowedModelConfigAttrs = [...]string{
	"admin-secret",
	"ca-private-key",
}

// ModelConfig returns the complete config for the model represented
// by this state.
func (st *State) ModelConfig() (*config.Config, error) {
	modelSettings, err := readSettings(st, settingsC, modelGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return config.New(config.NoDefaults, modelSettings.Map())
}

// checkModelConfig returns an error if the config is definitely invalid.
func checkModelConfig(cfg *config.Config) error {
	allAttrs := cfg.AllAttrs()
	for _, attr := range disallowedModelConfigAttrs {
		if _, ok := allAttrs[attr]; ok {
			return errors.Errorf(attr + " should never be written to the state")
		}
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return errors.Errorf("agent-version must always be set in state")
	}
	for attr := range allAttrs {
		if controller.ControllerOnlyAttribute(attr) {
			return errors.Errorf("cannot set controller attribute %q on a model", attr)
		}
	}
	return nil
}

// ModelConfigValues returns the config values for the model represented
// by this state.
func (st *State) ModelConfigValues() (config.ConfigValues, error) {
	modelSettings, err := readSettings(st, settingsC, modelGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Read all of the current inherited config values so
	// we can dynamically reflect the origin of the model config.
	configSources := modelConfigSources(st)
	sourceNames := make([]string, 0, len(configSources))
	sourceAttrs := make([]attrValues, 0, len(configSources))
	for _, src := range configSources {
		sourceNames = append(sourceNames, src.name)
		cfg, err := src.sourceFunc()
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, errors.Annotatef(err, "reading %s settings", src.name)
		}
		sourceAttrs = append(sourceAttrs, cfg)
	}

	// Figure out the source of each config attribute based
	// on the current model values and the inherited values.
	result := make(config.ConfigValues)
	for attr, val := range modelSettings.Map() {
		// Find the source of config for which the model
		// value matches. If there's a match, the last match
		// in the search order will be the source of config.
		// If there's no match, the source is the model.
		source := config.JujuModelConfigSource
		n := len(sourceAttrs)
		for i := range sourceAttrs {
			if sourceAttrs[n-i-1][attr] == val {
				source = sourceNames[n-i-1]
				break
			}
		}
		result[attr] = config.ConfigValue{
			Value:  val,
			Source: source,
		}
	}
	return result, nil
}

// checkControllerInheritedConfig returns an error if the shared local cloud config is definitely invalid.
func checkControllerInheritedConfig(attrs attrValues) error {
	disallowedCloudConfigAttrs := append(disallowedModelConfigAttrs[:], config.AgentVersionKey)
	for _, attr := range disallowedCloudConfigAttrs {
		if _, ok := attrs[attr]; ok {
			return errors.Errorf("local cloud config cannot contain " + attr)
		}
	}
	for attrName := range attrs {
		if controller.ControllerOnlyAttribute(attrName) {
			return errors.Errorf("local cloud config cannot contain controller attribute %q", attrName)
		}
	}
	return nil
}

func (st *State) buildAndValidateModelConfig(updateAttrs attrValues, removeAttrs []string, oldConfig *config.Config) (validCfg *config.Config, err error) {
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

	modelSettings.Update(validAttrs)
	_, ops := modelSettings.settingsUpdateOps()
	return modelSettings.write(ops)
}

type modelConfigSourceFunc func() (attrValues, error)

type modelConfigSource struct {
	name       string
	sourceFunc modelConfigSourceFunc
}

// modelConfigSources returns a slice of named model config
// sources, in hierarchical order. Starting from the first source,
// config is retrieved and each subsequent source adds to the
// overall config values, later values override earlier ones.
func modelConfigSources(st *State) []modelConfigSource {
	return []modelConfigSource{
		{config.JujuControllerSource, st.ControllerInheritedConfig},
		// We will also support local cloud region, tenant, user etc
	}
}

// ControllerInheritedConfig returns the inherited config values
// sourced from the local cloud config.
func (st *State) ControllerInheritedConfig() (attrValues, error) {
	settings, err := readSettings(st, globalSettingsC, controllerInheritedSettingsGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}

// composeModelConfigAttributes returns a set of model config settings composed from known
// sources of default values overridden by model specific attributes.
func composeModelConfigAttributes(
	modelAttr attrValues, configSources ...modelConfigSource,
) (attrValues, error) {
	resultAttrs := make(attrValues)

	// Compose default settings from all known sources.
	for _, source := range configSources {
		newSettings, err := source.sourceFunc()
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, errors.Annotatef(err, "reading %s settings", source.name)
		}
		for name, val := range newSettings {
			resultAttrs[name] = val
		}
	}

	// Merge in model specific settings.
	for attr, val := range modelAttr {
		resultAttrs[attr] = val
	}

	return resultAttrs, nil
}
