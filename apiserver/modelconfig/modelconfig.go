// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("ModelConfig", 1, newFacade)
}

func newFacade(st *state.State, _ facade.Resources, auth facade.Authorizer) (*ModelConfigAPI, error) {
	return NewModelConfigAPI(NewStateBackend(st), auth)
}

// ModelConfigAPI is the endpoint which implements the model config facade.
type ModelConfigAPI struct {
	backend Backend
	auth    facade.Authorizer
	check   *common.BlockChecker
}

// NewModelConfigAPI creates a new instance of the ModelConfig Facade.
func NewModelConfigAPI(backend Backend, authorizer facade.Authorizer) (*ModelConfigAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	client := &ModelConfigAPI{
		backend: backend,
		auth:    authorizer,
		check:   common.NewBlockChecker(backend),
	}
	return client, nil
}

// ModelGet implements the server-side part of the
// get-model-config CLI command.
func (c *ModelConfigAPI) ModelGet() (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	values, err := c.backend.ModelConfigValues()
	if err != nil {
		return result, err
	}

	// TODO(wallyworld) - this can be removed once credentials are properly
	// managed outside of model config.
	// Strip out any model config attributes that are credential attributes.
	provider, err := environs.Provider(values[config.TypeKey].Value.(string))
	if err != nil {
		return result, err
	}
	credSchemas := provider.CredentialSchemas()
	var allCredentialAttributes []string
	for _, schema := range credSchemas {
		for _, attr := range schema {
			allCredentialAttributes = append(allCredentialAttributes, attr.Name)
		}
	}
	isCredentialAttribute := func(attr string) bool {
		for _, a := range allCredentialAttributes {
			if a == attr {
				return true
			}
		}
		return false
	}

	result.Config = make(map[string]params.ConfigValue)
	for attr, val := range values {
		if isCredentialAttribute(attr) {
			continue
		}
		// Authorized keys are able to be listed using
		// juju ssh-keys and including them here just
		// clutters everything.
		if attr == config.AuthorizedKeysKey {
			continue
		}
		result.Config[attr] = params.ConfigValue{
			Value:  val.Value,
			Source: val.Source,
		}
	}
	return result, nil
}

// ModelSet implements the server-side part of the
// set-model-config CLI command.
func (c *ModelConfigAPI) ModelSet(args params.ModelSet) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// Make sure we don't allow changing agent-version.
	checkAgentVersion := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		if v, found := updateAttrs["agent-version"]; found {
			oldVersion, _ := oldConfig.AgentVersion()
			if v != oldVersion.String() {
				return errors.New("agent-version cannot be changed")
			}
		}
		return nil
	}
	// Replace any deprecated attributes with their new values.
	attrs := config.ProcessDeprecatedAttributes(args.Config)
	return c.backend.UpdateModelConfig(attrs, nil, checkAgentVersion)
}

// ModelUnset implements the server-side part of the
// set-model-config CLI command.
func (c *ModelConfigAPI) ModelUnset(args params.ModelUnset) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return c.backend.UpdateModelConfig(nil, args.Keys, nil)
}

// ModelDefaults returns the default config values used when creating a new model.
func (c *ModelConfigAPI) ModelDefaults() (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	values, err := c.backend.ModelConfigDefaultValues()
	if err != nil {
		return result, err
	}
	result.Config = make(map[string]params.ConfigValue)
	for attr, val := range values {
		result.Config[attr] = params.ConfigValue{
			Value:  val.Value,
			Source: val.Source,
		}
	}
	return result, nil
}

// SetModelDefaults writes new values for the specified default model settings.
func (c *ModelConfigAPI) SetModelDefaults(args params.SetModelDefaults) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Config))}
	if err := c.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, arg := range args.Config {
		// TODO(wallyworld) - use arg.Cloud and arg.CloudRegion as appropriate
		results.Results[i].Error = common.ServerError(
			c.setModelDefaults(arg),
		)
	}
	return results, nil
}

func (c *ModelConfigAPI) setModelDefaults(args params.ModelDefaultValues) error {
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// Make sure we don't allow changing agent-version.
	if _, found := args.Config["agent-version"]; found {
		return errors.New("agent-version cannot have a default value")
	}
	return c.backend.UpdateModelConfigDefaultValues(args.Config, nil)
}

// UnsetModelDefaults removes the specified default model settings.
func (c *ModelConfigAPI) UnsetModelDefaults(args params.UnsetModelDefaults) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Keys))}
	if err := c.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, arg := range args.Keys {
		// TODO(wallyworld) - use arg.Cloud and arg.CloudRegion as appropriate
		results.Results[i].Error = common.ServerError(
			c.backend.UpdateModelConfigDefaultValues(nil, arg.Keys),
		)
	}
	return results, nil
}
