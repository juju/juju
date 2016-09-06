// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/description"
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

func (c *ModelConfigAPI) checkCanWrite() error {
	canWrite, err := c.auth.HasPermission(description.WriteAccess, c.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite {
		return common.ErrPerm
	}
	return nil
}

func (c *ModelConfigAPI) isAdmin() error {
	hasAccess, err := c.auth.HasPermission(description.SuperuserAccess, c.backend.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !hasAccess {
		return common.ErrPerm
	}
	return nil
}

// ModelGet implements the server-side part of the
// get-model-config CLI command.
func (c *ModelConfigAPI) ModelGet() (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	if err := c.checkCanWrite(); err != nil {
		return result, errors.Trace(err)
	}

	values, err := c.backend.ModelConfigValues()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = make(map[string]params.ConfigValue)
	for attr, val := range values {
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
	if err := c.checkCanWrite(); err != nil {
		return err
	}

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
	if err := c.checkCanWrite(); err != nil {
		return err
	}
	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return c.backend.UpdateModelConfig(nil, args.Keys, nil)
}

// ModelDefaults returns the default config values used when creating a new model.
func (c *ModelConfigAPI) ModelDefaults() (params.ModelDefaultsResult, error) {
	result := params.ModelDefaultsResult{}
	if err := c.isAdmin(); err != nil {
		return result, errors.Trace(err)
	}

	values, err := c.backend.ModelConfigDefaultValues()
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Config = make(map[string]params.ModelDefaults)
	for attr, val := range values {
		settings := params.ModelDefaults{
			Controller: val.Controller,
			Default:    val.Default,
		}
		for _, v := range val.Regions {
			settings.Regions = append(
				settings.Regions, params.RegionDefaults{
					RegionName: v.Name,
					Value:      v.Value})
		}
		result.Config[attr] = settings
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
	if err := c.isAdmin(); err != nil {
		return errors.Trace(err)
	}

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
	if err := c.isAdmin(); err != nil {
		return results, err
	}

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
