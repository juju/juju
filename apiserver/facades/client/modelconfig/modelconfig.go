// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ModelConfigAPI provides the base implementation of the methods.
type ModelConfigAPI struct {
	backend Backend
	auth    facade.Authorizer
	check   *common.BlockChecker
}

// ModelConfigAPIV3 is currently the latest.
type ModelConfigAPIV3 struct {
	*ModelConfigAPI
}

// NewModelConfigAPI creates a new instance of the ModelConfig Facade.
func NewModelConfigAPI(backend Backend, authorizer facade.Authorizer) (*ModelConfigAPIV3, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	client := &ModelConfigAPI{
		backend: backend,
		auth:    authorizer,
		check:   common.NewBlockChecker(backend),
	}
	return &ModelConfigAPIV3{client}, nil
}

func (c *ModelConfigAPI) checkCanWrite() error {
	canWrite, err := c.auth.HasPermission(permission.WriteAccess, c.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (c *ModelConfigAPI) isControllerAdmin() error {
	hasAccess, err := c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !hasAccess {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (c *ModelConfigAPI) canReadModel() error {
	isAdmin, err := c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}
	canRead, err := c.auth.HasPermission(permission.ReadAccess, c.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !isAdmin && !canRead {
		return apiservererrors.ErrPerm
	}
	return nil
}

// ModelGet implements the server-side part of the
// model-config CLI command.
func (c *ModelConfigAPI) ModelGet() (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	if err := c.canReadModel(); err != nil {
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
	checkAgentVersion := c.checkAgentVersion()

	// Make sure we don't allow changing of the charmhub-url.
	checkCharmhubURL := c.checkCharmhubURL()

	// Only controller admins can set trace level debugging on a model.
	checkLogTrace := c.checkLogTrace()

	// Make sure DefaultSpace exists.
	checkDefaultSpace := c.checkDefaultSpace()

	// To use the logging-output feature, the feature flag needs to be set.
	checkLoggingConfig := c.checkLoggingOutput()

	// Replace any deprecated attributes with their new values.
	attrs := config.ProcessDeprecatedAttributes(args.Config)
	return c.backend.UpdateModelConfig(attrs, nil,
		checkAgentVersion, checkLogTrace, checkDefaultSpace, checkCharmhubURL, checkLoggingConfig)
}

func (c *ModelConfigAPI) checkLogTrace() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		spec, ok := updateAttrs["logging-config"]
		if !ok {
			return nil
		}
		// This prevents a panic when trying to convert a spec which can be nil.
		logSpec, ok := spec.(string)
		if !ok {
			return nil
		}
		logCfg, err := loggo.ParseConfigString(logSpec)
		if err != nil {
			return errors.Trace(err)
		}
		// Does at least one package have TRACE level logging requested.
		haveTrace := false
		for _, level := range logCfg {
			haveTrace = level == loggo.TRACE
			if haveTrace {
				break
			}
		}
		// No TRACE level requested, so no need to check for admin.
		if !haveTrace {
			return nil
		}
		if err := c.isControllerAdmin(); err != nil {
			if errors.Cause(err) != apiservererrors.ErrPerm {
				return errors.Trace(err)
			}
			return errors.New("only controller admins can set a model's logging level to TRACE")
		}
		return nil
	}
}

func (c *ModelConfigAPI) checkAgentVersion() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		if v, found := updateAttrs["agent-version"]; found {
			oldVersion, _ := oldConfig.AgentVersion()
			if v != oldVersion.String() {
				return errors.New("agent-version cannot be changed")
			}
		}
		return nil
	}
}

func (c *ModelConfigAPI) checkDefaultSpace() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		v, ok := updateAttrs["default-space"]
		if !ok {
			return nil
		}
		spaceName, ok := v.(string)
		if !ok {
			return errors.NotValidf("\"default-space\" is not a string")
		}
		if spaceName == "" {
			// No need to verify if a space isn't defined.
			return nil
		}
		return c.backend.SpaceByName(spaceName)
	}
}

func (c *ModelConfigAPI) checkCharmhubURL() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		if v, found := updateAttrs["charmhub-url"]; found {
			oldURL, _ := oldConfig.CharmHubURL()
			if v != oldURL {
				return errors.New("charmhub-url cannot be changed")
			}
		}
		return nil
	}
}

func (c *ModelConfigAPI) checkLoggingOutput() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		v, ok := updateAttrs[config.LoggingOutputKey]
		if !ok || v == "" {
			return nil
		}
		cfg, err := c.backend.ControllerConfig()
		if err != nil {
			return errors.Trace(err)
		}
		if !cfg.Features().Contains(feature.LoggingOutput) {
			return errors.Errorf("cannot set %q without setting the %q feature flag", config.LoggingOutputKey, feature.LoggingOutput)
		}
		return nil
	}
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
	return c.backend.UpdateModelConfig(nil, args.Keys)
}

// GetModelConstraints returns the constraints for the model.
func (c *ModelConfigAPI) GetModelConstraints() (params.GetConstraintsResults, error) {
	if err := c.canReadModel(); err != nil {
		return params.GetConstraintsResults{}, err
	}

	cons, err := c.backend.ModelConstraints()
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	return params.GetConstraintsResults{cons}, nil
}

// SetModelConstraints sets the constraints for the model.
func (c *ModelConfigAPI) SetModelConstraints(args params.SetConstraints) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	return c.backend.SetModelConstraints(args.Constraints)
}

// SetSLALevel sets the sla level on the model.
func (c *ModelConfigAPI) SetSLALevel(args params.ModelSLA) error {
	if err := c.checkCanWrite(); err != nil {
		return err
	}
	return c.backend.SetSLA(args.Level, args.Owner, args.Credentials)

}

// SLALevel returns the current sla level for the model.
func (c *ModelConfigAPI) SLALevel() (params.StringResult, error) {
	result := params.StringResult{}
	level, err := c.backend.SLALevel()
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Result = level
	return result, nil
}

// Sequences returns the model's sequence names and next values.
func (c *ModelConfigAPI) Sequences() (params.ModelSequencesResult, error) {
	result := params.ModelSequencesResult{}
	if err := c.canReadModel(); err != nil {
		return result, errors.Trace(err)
	}

	values, err := c.backend.Sequences()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Sequences = values
	return result, nil
}
