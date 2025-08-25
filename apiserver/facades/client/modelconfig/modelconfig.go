// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ModelConfigAPI provides the base implementation of the methods.
type ModelConfigAPI struct {
	backend   Backend
	auth      facade.Authorizer
	check     *common.BlockChecker
	cloudType string
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
	model := backend.Model()
	cloud, err := model.Cloud()
	if err != nil {
		return nil, err
	}

	client := &ModelConfigAPI{
		backend:   backend,
		auth:      authorizer,
		check:     common.NewBlockChecker(backend),
		cloudType: cloud.Type,
	}
	return &ModelConfigAPIV3{client}, nil
}

func (c *ModelConfigAPI) checkCanWrite() error {
	return c.auth.HasPermission(permission.WriteAccess, c.backend.ModelTag())
}

func (c *ModelConfigAPI) isControllerAdmin() error {
	return c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
}

func (c *ModelConfigAPI) canReadModel() error {
	err := c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	err = c.auth.HasPermission(permission.AdminAccess, c.backend.ModelTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	return c.auth.HasPermission(permission.ReadAccess, c.backend.ModelTag())
}

func (c *ModelConfigAPI) isModelAdmin() (bool, error) {
	err := c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, errors.Trace(err)
	} else if err == nil {
		return true, nil
	}
	err = c.auth.HasPermission(permission.AdminAccess, c.backend.ModelTag())
	return err == nil, err
}

// ModelGet implements the server-side part of the
// model-config CLI command.
func (c *ModelConfigAPI) ModelGet() (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	if err := c.canReadModel(); err != nil {
		return result, errors.Trace(err)
	}

	isAdmin, err := c.isModelAdmin()
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}

	defaultSchema, err := config.Schema(nil)
	if err != nil {
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

		// TODO (stickupkid): Remove this when we remove series.
		// This essentially, always ensures that we report back a default-series
		// if we have a default-base.
		if attr == config.DefaultBaseKey && val.Value != "" {
			base, err := corebase.ParseBaseFromString(val.Value.(string))
			if err != nil {
				return result, errors.Trace(err)
			}

			s, err := corebase.GetSeriesFromBase(base)
			if err != nil {
				return result, errors.Trace(err)
			}

			result.Config[config.DefaultSeriesKey] = params.ConfigValue{
				Source: val.Source,
				Value:  s,
			}
		}

		// Only admins get to see attributes marked as secret.
		if attr, ok := defaultSchema[attr]; ok && attr.Secret && !isAdmin {
			continue
		}

		result.Config[attr] = params.ConfigValue{
			Value:  val.Value,
			Source: val.Source,
		}
	}

	// TODO (stickupkid): For backwards compatibility we need to ensure that
	// we always report back a default-series.
	if _, ok := result.Config[config.DefaultSeriesKey]; !ok {
		result.Config[config.DefaultSeriesKey] = params.ConfigValue{
			Value:  "",
			Source: "default",
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
	isAdmin, err := c.isModelAdmin()
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}
	defaultSchema, err := config.Schema(nil)
	if err != nil {
		return errors.Trace(err)
	}
	// Only admins get to set attributes marked as secret.
	for attr := range args.Config {
		if attr, ok := defaultSchema[attr]; ok && attr.Secret && !isAdmin {
			return apiservererrors.ErrPerm
		}
	}

	// Series to base translations.
	checkUpdateDefaultBase := c.checkUpdateDefaultBase()

	// Make sure we don't allow changing agent-version.
	checkAgentVersion := c.checkAgentVersion()

	// Make sure we don't allow changing of the charmhub-url.
	checkCharmhubURL := c.checkCharmhubURL()

	// Only controller admins can set trace level debugging on a model.
	checkLogTrace := c.checkLogTrace()

	// Make sure DefaultSpace exists.
	checkDefaultSpace := c.checkDefaultSpace()

	// Make sure the secret backend exists.
	checkSecretBackend := c.checkSecretBackend()

	// Make sure the passed config does not set authorized-keys.
	checkAuthorizedKeys := c.checkAuthorizedKeys()

	checkLXDProfile := c.checkLXDProfile()

	// Replace any deprecated attributes with their new values.
	attrs := config.ProcessDeprecatedAttributes(args.Config)

	return c.backend.UpdateModelConfig(attrs,
		nil,
		checkAgentVersion,
		checkLogTrace,
		checkDefaultSpace,
		checkCharmhubURL,
		checkSecretBackend,
		checkUpdateDefaultBase,
		checkAuthorizedKeys,
		checkLXDProfile,
	)
}

func (c *ModelConfigAPI) checkLXDProfile() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		if c.cloudType != "lxd" {
			return nil
		}

		_, updateProject := updateAttrs["project"]
		if !updateProject {
			return nil
		}

		model := c.backend.Model()
		if length, err := model.MachinesLen(); err != nil {
			return err
		} else if length > 0 {
			return fmt.Errorf("cannot change project because model %q is non-empty", model.Name())
		}

		return nil
	}
}

// checkAuthorizedKeys checks that the passed config attributes does not
// contain authorized-keys.
func (c *ModelConfigAPI) checkAuthorizedKeys() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		if _, found := updateAttrs[config.AuthorizedKeysKey]; found {
			return errors.New("authorized-keys cannot be set")
		}
		return nil
	}
}

func (c *ModelConfigAPI) checkUpdateDefaultBase() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		cfgSeries, defaultSeriesOK := updateAttrs[config.DefaultSeriesKey]
		_, defaultBaseOK := updateAttrs[config.DefaultBaseKey]

		if defaultSeriesOK && defaultBaseOK {
			if cfgSeries != "" {
				// If the default-series is set (and non-empty) and the default-base is set, error
				// out, as you can't change both.
				return errors.New("cannot set both default-series and default-base")
			}
		} else if defaultSeriesOK {
			// If the default-series is set, but empty, then we need to patch the
			// base.
			if cfgSeries == "" {
				updateAttrs[config.DefaultBaseKey] = ""
			} else {
				// Ensure that the new default-series updates the default-base.
				base, err := corebase.GetBaseFromSeries(cfgSeries.(string))
				if err != nil {
					return errors.Trace(err)
				}
				updateAttrs[config.DefaultBaseKey] = base.String()
			}
		}

		// Always remove the default-series.
		delete(updateAttrs, config.DefaultSeriesKey)
		return nil
	}
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

		err = c.isControllerAdmin()
		if !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return errors.Trace(err)
		} else if err != nil {
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

func (c *ModelConfigAPI) checkSecretBackend() state.ValidateConfigFunc {
	return func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		v, ok := updateAttrs[config.SecretBackendKey]
		if !ok {
			return nil
		}
		backendName, ok := v.(string)
		if !ok {
			return errors.NewNotValid(nil, fmt.Sprintf("%q config value is not a string", config.SecretBackendKey))
		}
		if backendName == "" {
			return errors.NotValidf("empty %q config value", config.SecretBackendKey)
		}
		if backendName == config.DefaultSecretBackend {
			return nil
		}
		backend, err := c.backend.GetSecretBackend(backendName)
		if err != nil {
			return errors.Trace(err)
		}
		p, err := commonsecrets.GetProvider(backend.BackendType)
		if err != nil {
			return errors.Annotatef(err, "cannot get backend for provider type %q", backend.BackendType)
		}
		err = commonsecrets.PingBackend(p, backend.Config)
		return errors.Annotatef(err, "cannot ping backend %q", backend.Name)
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

	// If we're attempting to remove the default-series, then we need to
	// swap that out for the default-base.
	for i, key := range args.Keys {
		if key == config.DefaultSeriesKey {
			args.Keys[i] = config.DefaultBaseKey
			break
		}
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
	return params.GetConstraintsResults{Constraints: cons}, nil
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
