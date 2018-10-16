// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/schema"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type attrValues map[string]interface{}

var disallowedModelConfigAttrs = [...]string{
	"admin-secret",
	"ca-private-key",
}

// ModelConfig returns the complete config for the model
func (m *Model) ModelConfig() (*config.Config, error) {
	return getModelConfig(m.st.db(), m.UUID())
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
func (st *State) inheritedConfigAttributes() (map[string]interface{}, error) {
	rspec, err := st.regionSpec()
	if err != nil {
		return nil, errors.Trace(err)
	}
	configSources := modelConfigSources(st, rspec)
	values := make(attrValues)
	for _, src := range configSources {
		cfg, err := src.sourceFunc()
		if errors.IsNotFound(err) {
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

// modelConfigValues returns the values and source for the supplied model config
// when combined with controller and Juju defaults.
func (model *Model) modelConfigValues(modelCfg attrValues) (config.ConfigValues, error) {
	resultValues := make(attrValues)
	for k, v := range modelCfg {
		resultValues[k] = v
	}

	// Read all of the current inherited config values so
	// we can dynamically reflect the origin of the model config.
	rspec, err := model.st.regionSpec()
	if err != nil {
		return nil, errors.Trace(err)
	}
	configSources := modelConfigSources(model.st, rspec)
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

		// If no modelCfg was passed in, we'll accumulate data
		// for the inherited values instead.
		if len(modelCfg) == 0 {
			for k, v := range cfg {
				resultValues[k] = v
			}
		}
	}

	// Figure out the source of each config attribute based
	// on the current model values and the inherited values.
	result := make(config.ConfigValues)
	for attr, val := range resultValues {
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

// UpdateModelConfigDefaultValues updates the inherited settings used when creating a new model.
func (model *Model) UpdateModelConfigDefaultValues(attrs map[string]interface{}, removed []string, regionSpec *environs.RegionSpec) error {
	var key string

	if regionSpec != nil {
		key = regionSettingsGlobalKey(regionSpec.Cloud, regionSpec.Region)
	} else {
		key = controllerInheritedSettingsGlobalKey
	}
	settings, err := readSettings(model.st.db(), globalSettingsC, key)
	if err != nil {
		if !errors.IsNotFound(err) {
			return errors.Annotatef(err, "model %q", model.UUID())
		}
		// We haven't created settings for this region yet.
		_, err := createSettings(model.st.db(), globalSettingsC, key, attrs)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	// TODO(axw) 2013-12-6 #1167616
	// Ensure that the settings on disk have not changed
	// underneath us. The settings changes are actually
	// applied as a delta to what's on disk; if there has
	// been a concurrent update, the change may not be what
	// the user asked for.
	settings.Update(attrs)
	for _, r := range removed {
		settings.Delete(r)
	}
	_, err = settings.Write()
	return err
}

// ModelConfigValues returns the config values for the model represented
// by this state.
func (model *Model) ModelConfigValues() (config.ConfigValues, error) {
	cfg, err := model.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return model.modelConfigValues(cfg.AllAttrs())
}

// ModelConfigDefaultValues returns the default config values to be used
// when creating a new model, and the origin of those values.
func (model *Model) ModelConfigDefaultValues() (config.ModelDefaultAttributes, error) {
	cloudName := model.Cloud()
	cloud, err := model.State().Cloud(cloudName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(config.ModelDefaultAttributes)
	// Juju defaults
	defaultAttrs, err := model.State().defaultInheritedConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for k, v := range defaultAttrs {
		result[k] = config.AttributeDefaultValues{Default: v}
	}
	// Controller config
	ciCfg, err := model.State().controllerInheritedConfig()
	if err != nil && !errors.IsNotFound(err) {
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
	// Region config
	for _, region := range cloud.Regions {
		rspec := &environs.RegionSpec{Cloud: cloudName, Region: region.Name}
		riCfg, err := model.State().regionInheritedConfig(rspec)()
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return nil, errors.Trace(err)
		}
		for k, v := range riCfg {
			regCfg := config.RegionDefaultValue{Name: region.Name, Value: v}
			if ds, ok := result[k]; ok {
				ds.Regions = append(result[k].Regions, regCfg)
				result[k] = ds
			} else {
				result[k] = config.AttributeDefaultValues{Regions: []config.RegionDefaultValue{regCfg}}
			}
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
func (m *Model) UpdateModelConfig(updateAttrs map[string]interface{}, removeAttrs []string, additionalValidation ...ValidateConfigFunc) error {
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
		inherited, err := st.inheritedConfigAttributes()
		if err != nil {
			return errors.Trace(err)
		}
		for _, attr := range removeAttrs {
			// We we are updating an attribute, that takes
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

	oldConfig, err := m.ModelConfig()
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
func modelConfigSources(st *State, regionSpec *environs.RegionSpec) []modelConfigSource {
	return []modelConfigSource{
		{config.JujuDefaultSource, st.defaultInheritedConfig},
		{config.JujuControllerSource, st.controllerInheritedConfig},
		{config.JujuRegionSource, st.regionInheritedConfig(regionSpec)},
	}
}

const (
	// controllerInheritedSettingsGlobalKey is the key for default settings shared across models.
	controllerInheritedSettingsGlobalKey = "controller"
)

// defaultInheritedConfig returns config values which are defined
// as defaults in either Juju or the state's environ provider.
func (st *State) defaultInheritedConfig() (attrValues, error) {
	var defaults = make(map[string]interface{})
	for k, v := range config.ConfigDefaults() {
		defaults[k] = v
	}
	providerDefaults, err := st.environsProviderConfigSchemaSource()
	if errors.IsNotImplemented(err) {
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

// controllerInheritedConfig returns the inherited config values
// sourced from the local cloud config.
func (st *State) controllerInheritedConfig() (attrValues, error) {
	settings, err := readSettings(st.db(), globalSettingsC, controllerInheritedSettingsGlobalKey)
	if err != nil {
		return nil, errors.Annotatef(err, "controller %q", st.ControllerUUID())
	}
	return settings.Map(), nil
}

// regionInheritedConfig returns the configuration attributes for the region in
// the cloud where the model is targeted.
func (st *State) regionInheritedConfig(regionSpec *environs.RegionSpec) func() (attrValues, error) {
	if regionSpec == nil {
		return func() (attrValues, error) {
			return nil, errors.New(
				"no environs.RegionSpec provided")
		}
	}
	if regionSpec.Region == "" {
		// It is expected that not all clouds have regions. So return not found
		// if there is not a region here.
		return func() (attrValues, error) {
			return nil, errors.NotFoundf("region")
		}
	}
	return func() (attrValues, error) {
		settings, err := readSettings(st.db(),
			globalSettingsC,
			regionSettingsGlobalKey(regionSpec.Cloud, regionSpec.Region),
		)
		if err != nil {
			return nil, errors.Annotatef(err, "region %q on %q cloud", regionSpec.Region, regionSpec.Cloud)
		}
		return settings.Map(), nil
	}
}

// regionSpec returns a suitable environs.RegionSpec for use in
// regionInheritedConfig.
func (st *State) regionSpec() (*environs.RegionSpec, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	rspec := &environs.RegionSpec{
		Cloud:  model.Cloud(),
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

// ComposeNewModelConfig returns a complete map of config attributes suitable for
// creating a new model, by combining user specified values with system defaults.
func (st *State) ComposeNewModelConfig(modelAttr map[string]interface{}, regionSpec *environs.RegionSpec) (map[string]interface{}, error) {
	configSources := modelConfigSources(st, regionSpec)
	return composeModelConfigAttributes(modelAttr, configSources...)
}
