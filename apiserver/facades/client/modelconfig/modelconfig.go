// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"context"
	"fmt"
	coreuser "github.com/juju/juju/core/user"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
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

func (c *ModelConfigAPI) checkCanWrite(usr coreuser.User) error {
	return c.auth.HasPermission(usr, permission.WriteAccess, c.backend.ModelTag())
}

func (c *ModelConfigAPI) isControllerAdmin(usr coreuser.User) error {
	return c.auth.HasPermission(usr, permission.SuperuserAccess, c.backend.ControllerTag())
}

func (c *ModelConfigAPI) canReadModel(usr coreuser.User) error {
	err := c.auth.HasPermission(usr, permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	err = c.auth.HasPermission(usr, permission.AdminAccess, c.backend.ModelTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	return c.auth.HasPermission(usr, permission.ReadAccess, c.backend.ModelTag())
}

func (c *ModelConfigAPI) isModelAdmin(usr coreuser.User) (bool, error) {
	err := c.auth.HasPermission(usr, permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return false, errors.Trace(err)
	} else if err == nil {
		return true, nil
	}
	err = c.auth.HasPermission(usr, permission.AdminAccess, c.backend.ModelTag())
	return err == nil, err
}

// ModelGet implements the server-side part of the
// model-config CLI command.
func (c *ModelConfigAPI) ModelGet(ctx context.Context) (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	if err := c.canReadModel(usr); err != nil {
		return result, errors.Trace(err)
	}

	isAdmin, err := c.isModelAdmin(usr)
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

		// Only admins get to see attributes marked as secret.
		if attr, ok := defaultSchema[attr]; ok && attr.Secret && !isAdmin {
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
func (c *ModelConfigAPI) ModelSet(ctx context.Context, args params.ModelSet) error {
	if err := c.checkCanWrite(usr); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}
	isAdmin, err := c.isModelAdmin(usr)
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

	// Make sure we don't allow changing agent-version.
	checkAgentVersion := c.checkAgentVersion()

	// Make sure we don't allow changing of the charmhub-url.
	checkCharmhubURL := c.checkCharmhubURL()

	// Only controller admins can set trace level debugging on a model.
	checkLogTrace := c.checkLogTrace(usr)

	// Make sure DefaultSpace exists.
	checkDefaultSpace := c.checkDefaultSpace()

	// Make sure the secret backend exists.
	checkSecretBackend := c.checkSecretBackend()

	// Make sure the passed config does not set authorized-keys.
	checkAuthorizedKeys := c.checkAuthorizedKeys()

	// Replace any deprecated attributes with their new values.
	return c.backend.UpdateModelConfig(args.Config,
		nil,
		checkAgentVersion,
		checkLogTrace,
		checkDefaultSpace,
		checkCharmhubURL,
		checkSecretBackend,
		checkAuthorizedKeys,
	)
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

func (c *ModelConfigAPI) checkLogTrace(usr coreuser.User) state.ValidateConfigFunc {
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

		err = c.isControllerAdmin(usr)
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
func (c *ModelConfigAPI) ModelUnset(ctx context.Context, args params.ModelUnset) error {
	if err := c.checkCanWrite(usr); err != nil {
		return err
	}
	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}

	return c.backend.UpdateModelConfig(nil, args.Keys)
}

// GetModelConstraints returns the constraints for the model.
func (c *ModelConfigAPI) GetModelConstraints(ctx context.Context) (params.GetConstraintsResults, error) {
	if err := c.canReadModel(usr); err != nil {
		return params.GetConstraintsResults{}, err
	}

	cons, err := c.backend.ModelConstraints()
	if err != nil {
		return params.GetConstraintsResults{}, err
	}
	return params.GetConstraintsResults{Constraints: cons}, nil
}

// SetModelConstraints sets the constraints for the model.
func (c *ModelConfigAPI) SetModelConstraints(ctx context.Context, args params.SetConstraints) error {
	if err := c.checkCanWrite(usr); err != nil {
		return err
	}

	if err := c.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}
	return c.backend.SetModelConstraints(args.Constraints)
}

// SetSLALevel sets the sla level on the model.
func (c *ModelConfigAPI) SetSLALevel(ctx context.Context, args params.ModelSLA) error {
	if err := c.checkCanWrite(usr); err != nil {
		return err
	}
	return c.backend.SetSLA(args.Level, args.Owner, args.Credentials)

}

// SLALevel returns the current sla level for the model.
func (c *ModelConfigAPI) SLALevel(ctx context.Context) (params.StringResult, error) {
	result := params.StringResult{}
	level, err := c.backend.SLALevel()
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Result = level
	return result, nil
}

// Sequences returns the model's sequence names and next values.
func (c *ModelConfigAPI) Sequences(ctx context.Context) (params.ModelSequencesResult, error) {
	result := params.ModelSequencesResult{}
	if err := c.canReadModel(usr); err != nil {
		return result, errors.Trace(err)
	}

	values, err := c.backend.Sequences()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Sequences = values
	return result, nil
}
