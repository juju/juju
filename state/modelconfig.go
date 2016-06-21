// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
)

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
	if cfg.AdminSecret() != "" {
		return errors.Errorf("admin-secret should never be written to the state")
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return errors.Errorf("agent-version must always be set in state")
	}
	return nil
}

// checkLocalCloudConfigDefaults returns an error if the shared local cloud config is definitely invalid.
func checkLocalCloudConfigDefaults(attrs map[string]interface{}) error {
	if _, ok := attrs[config.AdminSecretKey]; ok {
		return errors.Errorf("local cloud config cannot contain admin-secret")
	}
	if _, ok := attrs[config.AgentVersionKey]; ok {
		return errors.Errorf("local cloud config cannot contain agent-version")
	}
	for _, attrName := range controller.ControllerOnlyConfigAttributes {
		if _, ok := attrs[attrName]; ok {
			return errors.Errorf("local cloud config cannot contain controller attribute %q", attrName)
		}
	}
	return nil
}

func (st *State) buildAndValidateModelConfig(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) (validCfg *config.Config, err error) {
	for attr := range updateAttrs {
		if controller.ControllerOnlyAttribute(attr) {
			return nil, errors.Errorf("cannot set controller attribute %q on a model", attr)
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

	modelSettings.Update(validAttrs)
	changes, err := modelSettings.Write()

	// Update the map recording the source of each attribute
	// in the model config. Any updates are cumulative; even
	// attribute deletes record the sources as "model", so
	// it's ok to do this outside the txn to write the updates
	// themselves.
	return st.updateModelConfigSources(changes)
}

func (st *State) updateModelConfigSources(changes []ItemChange) error {
	var update bson.D
	var set = make(bson.M)
	for _, c := range changes {
		set[c.Key] = "model"
	}
	replace := inSubdocReplacer("sources")
	set = bson.M(copyMap(map[string]interface{}(set), replace))
	update = append(update,
		bson.DocElem{"$set", set},
		bson.DocElem{"$inc", bson.D{{"version", 1}}})

	ops := []txn.Op{{
		C:      modelSettingsSourcesC,
		Id:     modelGlobalKey,
		Assert: txn.DocExists,
		Update: update,
	}}
	err := st.runTransaction(ops)
	if err == txn.ErrAborted {
		return errors.NotFoundf("settings sources")
	}
	return errors.Trace(err)
}

// settingsSourcesDoc stores for each model attribute,
// the source of the attribute.
type settingsSourcesDoc struct {
	// Sources defines the named source for each settings attribute.
	Sources map[string]string `bson:"sources,omitempty"`

	// Version is a version number for the settings sources,
	// and is increased every time the sources change.
	Version int64 `bson:"version"`
}

func createSettingsSourceOp(values map[string]string) txn.Op {
	return txn.Op{
		C:      modelSettingsSourcesC,
		Id:     modelGlobalKey,
		Assert: txn.DocMissing,
		Insert: &settingsSourcesDoc{
			Sources: values,
		},
	}
}

// ModelConfigSources returns the named source for each config attribute.
func (st *State) ModelConfigSources() (map[string]string, error) {
	sources, closer := st.getCollection(modelSettingsSourcesC)
	defer closer()

	var out settingsSourcesDoc
	err := sources.FindId(modelGlobalKey).One(&out)
	if err == mgo.ErrNotFound {
		err = errors.NotFoundf("settings sources")
	}
	if err != nil {
		return nil, err
	}
	return out.Sources, nil
}

// These constants define named sources of model config attributes.
const (
	// jujuCloudSource is used to label model config attributes that
	// come from those associated with the host cloud.
	jujuCloudSource = "juju cloud"

	// jujuModelSource is used to label model config attributes that
	// come explicitly from the user.
	jujuModelSource = "model"
)

type modelConfigSourceFunc func() (map[string]interface{}, error)

type modelConfigSource struct {
	name       string
	sourceFunc modelConfigSourceFunc
}

func modelConfigSources(st *State) []modelConfigSource {
	return []modelConfigSource{
		{jujuCloudSource, st.localCloudConfig},
		// We wil also support local cloud region, tenant, user etc
	}
}

// localCloudConfig returns the inherited config values
// sourced from the local cloud config.
func (st *State) localCloudConfig() (map[string]interface{}, error) {
	info, err := st.ControllerInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	settings, err := readSettings(st, modelInheritedSettingsC, cloudGlobalKey(info.CloudName))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings.Map(), nil
}

// composeModelConfigAttributes returns a set of model config settings composed from known
// sources of default values overridden by model specific attributes.
// Also returned is a map containing the source location for each model attribute.
// The source location is the name of the config values from which an attribute came.
func (st *State) composeModelConfigAttributes(
	modelAttr map[string]interface{}, extraSources ...modelConfigSource,
) (map[string]interface{}, map[string]string, error) {
	resultAttrs := make(map[string]interface{})
	settingsSources := make(map[string]string)

	configSources := modelConfigSources(st)
	configSources = append(configSources, extraSources...)
	// Compose default settings from all known sources.
	for _, source := range configSources {
		newSettings, err := source.sourceFunc()
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, nil, errors.Annotatef(err, "reading %s settings", source.name)
		}
		for name, val := range newSettings {
			resultAttrs[name] = val
			settingsSources[name] = source.name
		}
	}

	// Merge in model specific settings.
	for attr, val := range modelAttr {
		resultAttrs[attr] = val
		settingsSources[attr] = jujuModelSource
	}

	return resultAttrs, settingsSources, nil
}
