// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/schema"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

type attrValues map[string]interface{}

var disallowedModelConfigAttrs = [...]string{
	"admin-secret",
	"ca-private-key",
}

// ModelConfig returns the complete config for the model
func (m *Model) ModelConfig(context.Context) (*config.Config, error) {
	return getModelConfig(m.st.db(), m.UUID())
}

// AgentVersion returns the agent version for the model config.
// If no agent version is found, it returns NotFound error.
func (m *Model) AgentVersion() (version.Number, error) {
	cfg, err := m.ModelConfig(context.Background())
	if err != nil {
		return version.Number{}, errors.Trace(err)
	}
	ver, ok := cfg.AgentVersion()
	if !ok {
		return version.Number{}, errors.NotFoundf("agent version")
	}
	return ver, nil
}

func getModelConfig(db Database, uuid string) (*config.Config, error) {
	modelSettings, err := readSettings(db, settingsC, modelGlobalKey)
	if err != nil {
		return nil, errors.Annotatef(err, "model %q", uuid)
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

// inheritedConfigAttributes returns the merged collection of inherited config
// values used as model defaults when adding models or unsetting values.
func (st *State) inheritedConfigAttributes(configSchemaGetter config.ConfigSchemaSourceGetter) (map[string]interface{}, error) {
	rspec, err := st.regionSpec()
	if err != nil {
		return nil, errors.Trace(err)
	}
	configSources := modelConfigSources(configSchemaGetter, st, rspec)
	values := make(attrValues)
	for _, src := range configSources {
		cfg, err := src.sourceFunc()
		if errors.Is(err, errors.NotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Annotatef(err, "reading %s settings", src.name)
		}
		for attrName, value := range cfg {
			values[attrName] = value
		}
	}
	return values, nil
}

// UpdateModelConfigDefaultValues updates the inherited settings used when creating a new model.
func (st *State) UpdateModelConfigDefaultValues(updateAttrs map[string]interface{}, removeAttrs []string, regionSpec *environscloudspec.CloudRegionSpec) error {
	var key string

	if regionSpec != nil {
		if regionSpec.Region == "" {
			key = cloudGlobalKey(regionSpec.Cloud)
		}
	} else {
		// For backwards compatibility default to the model's cloud.
		model, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}
		key = cloudGlobalKey(model.CloudName())
	}
	settings, err := readSettings(st.db(), globalSettingsC, key)
	if err != nil {
		if !errors.Is(err, errors.NotFound) {
			return errors.Annotatef(err, "model %q", st.ModelUUID())
		}
		// We haven't created settings for this region yet.
		_, err := createSettings(st.db(), globalSettingsC, key, updateAttrs)
		if err != nil {
			return errors.Annotatef(err, "model %q", st.ModelUUID())
		}
		return nil
	}

	// TODO(axw) 2013-12-6 #1167616
	// Ensure that the settings on disk have not changed
	// underneath us. The settings changes are actually
	// applied as a delta to what's on disk; if there has
	// been a concurrent update, the change may not be what
	// the user asked for.

	// Attempt to validate against the current old model and the new model, that
	// should be enough to verify the config against.
	// If there are additional fields in the config, then this should be fine
	// and should not throw a validation error.
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	oldConfig, err := model.ModelConfig(context.Background())
	if err != nil {
		return errors.Trace(err)
	}
	validCfg, err := st.buildAndValidateModelConfig(updateAttrs, removeAttrs, oldConfig)
	if err != nil {
		return errors.Trace(err)
	}
	validAttrs := validCfg.AllAttrs()
	for k := range updateAttrs {
		if v, ok := validAttrs[k]; ok {
			updateAttrs[k] = v
		}
	}

	updateAttrs = config.CoerceForStorage(updateAttrs)
	settings.Update(updateAttrs)
	for _, r := range removeAttrs {
		settings.Delete(r)
	}
	_, err = settings.Write()
	return err
}

// ModelConfigDefaultValues returns the default config values to be used
// when creating a new model, and the origin of those values.
func (st *State) ModelConfigDefaultValues(configSchemaGetter config.ConfigSchemaSourceGetter, cloudName string) (config.ModelDefaultAttributes, error) {
	result := make(config.ModelDefaultAttributes)
	// Juju defaults
	defaultAttrs, err := st.defaultInheritedConfig(configSchemaGetter, cloudName)()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range defaultAttrs {
		result[k] = config.AttributeDefaultValues{Default: v}
	}
	// Controller config
	ciCfg, err := st.controllerInheritedConfig(cloudName)()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)

	}
	for k, v := range ciCfg {
		if ds, ok := result[k]; ok {
			ds.Controller = v
			result[k] = ds
		} else {
			result[k] = config.AttributeDefaultValues{Controller: v}
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

func (st *State) buildAndValidateModelConfig(updateAttrs attrValues, removeAttrs []string, oldConfig *config.Config) (*config.Config, error) {
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
func (m *Model) UpdateModelConfig(configSchemaGetter config.ConfigSchemaSourceGetter, updateAttrs map[string]interface{}, removeAttrs []string, additionalValidation ...ValidateConfigFunc) error {
	if len(updateAttrs)+len(removeAttrs) == 0 {
		return nil
	}

	st := m.State()
	if len(removeAttrs) > 0 {
		var removed []string
		if updateAttrs == nil {
			updateAttrs = make(map[string]interface{})
		}
		// For each removed attribute, pick up any inherited value
		// and if there's one, use that.
		inherited, err := st.inheritedConfigAttributes(configSchemaGetter)
		if err != nil {
			return errors.Trace(err)
		}
		for _, attr := range removeAttrs {
			// We are updating an attribute, that takes
			// precedence over removing.
			if _, ok := updateAttrs[attr]; ok {
				continue
			}
			if val, ok := inherited[attr]; ok {
				updateAttrs[attr] = val
			} else {
				removed = append(removed, attr)
			}
		}
		removeAttrs = removed
	}
	// TODO(axw) 2013-12-6 #1167616
	// Ensure that the settings on disk have not changed
	// underneath us. The settings changes are actually
	// applied as a delta to what's on disk; if there has
	// been a concurrent update, the change may not be what
	// the user asked for.
	modelSettings, err := readSettings(st.db(), settingsC, modelGlobalKey)
	if err != nil {
		return errors.Annotatef(err, "model %q", m.UUID())
	}

	oldConfig, err := m.ModelConfig(context.Background())
	if err != nil {
		return errors.Trace(err)
	}
	for _, additionalValidationFunc := range additionalValidation {
		err = additionalValidationFunc(updateAttrs, removeAttrs, oldConfig)
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
	// Some values require marshalling before storage.
	validAttrs = config.CoerceForStorage(validAttrs)

	modelSettings.Update(validAttrs)
	_, ops := modelSettings.settingsUpdateOps()
	if len(ops) > 0 {
		return modelSettings.write(ops)
	}
	return nil
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
func modelConfigSources(configSchemaGetter config.ConfigSchemaSourceGetter, st *State, regionSpec *environscloudspec.CloudRegionSpec) []modelConfigSource {
	return []modelConfigSource{
		{config.JujuDefaultSource, st.defaultInheritedConfig(configSchemaGetter, regionSpec.Cloud)},
		{config.JujuControllerSource, st.controllerInheritedConfig(regionSpec.Cloud)},
	}
}

// defaultInheritedConfig returns config values which are defined
// as defaults in either Juju or the cloud's environ provider.
func (st *State) defaultInheritedConfig(configSchemaGetter config.ConfigSchemaSourceGetter, cloudName string) func() (attrValues, error) {
	return func() (attrValues, error) {
		var defaults = make(map[string]interface{})
		for k, v := range config.ConfigDefaults() {
			defaults[k] = v
		}
		providerDefaults, err := configSchemaGetter(context.TODO(), cloudName)
		if errors.Is(err, errors.NotImplemented) {
			return defaults, nil
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		fields := schema.FieldMap(providerDefaults.ConfigSchema(), providerDefaults.ConfigDefaults())
		if coercedAttrs, err := fields.Coerce(defaults, nil); err != nil {
			return nil, errors.Trace(err)
		} else {
			for k, v := range coercedAttrs.(map[string]interface{}) {
				defaults[k] = v
			}
		}
		return defaults, nil
	}
}

// controllerInheritedConfig returns the inherited config values
// sourced from the local cloud config.
func (st *State) controllerInheritedConfig(cloudName string) func() (attrValues, error) {
	return func() (attrValues, error) {
		settings, err := readSettings(st.db(), globalSettingsC, cloudGlobalKey(cloudName))
		if err != nil {
			return nil, errors.Annotatef(err, "controller %q", st.ControllerUUID())
		}
		return settings.Map(), nil
	}
}

// regionSpec returns a suitable environscloudspec.CloudRegionSpec for use in
// regionInheritedConfig.
func (st *State) regionSpec() (*environscloudspec.CloudRegionSpec, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	rspec := &environscloudspec.CloudRegionSpec{
		Cloud:  model.CloudName(),
		Region: model.CloudRegion(),
	}
	return rspec, nil
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
		if errors.Is(err, errors.NotFound) {
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

// ComposeNewModelConfig returns a complete map of config attributes suitable for
// creating a new model, by combining user specified values with system defaults.
func (st *State) ComposeNewModelConfig(configSchemaGetter config.ConfigSchemaSourceGetter, modelAttr map[string]interface{}, regionSpec *environscloudspec.CloudRegionSpec) (map[string]interface{}, error) {
	configSources := modelConfigSources(configSchemaGetter, st, regionSpec)
	return composeModelConfigAttributes(modelAttr, configSources...)
}
