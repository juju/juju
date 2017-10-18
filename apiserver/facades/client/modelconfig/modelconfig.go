// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelconfig

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/permission"
)

// ModelConfigAPI is the endpoint which implements the model config facade.
type ModelConfigAPI struct {
	backend Backend
	auth    facade.Authorizer
	check   *common.BlockChecker
}

// ModelConfigAPIV1 provides a way to wrap the different calls between
// version 1 and version 2 of the model config API
type ModelConfigAPIV1 struct {
	*ModelConfigAPI
}

// NewFacade is used for API registration.
func NewFacade(ctx facade.Context) (*ModelConfigAPI, error) {
	st := ctx.State()
	auth := ctx.Auth()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelConfigAPI(NewStateBackend(model), auth)
}

// NewFacadeV1 returns a V1 facade
func NewFacadeV1(ctx facade.Context) (*ModelConfigAPIV1, error) {
	base, err := NewFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ModelConfigAPIV1{base}, nil
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
	canWrite, err := c.auth.HasPermission(permission.WriteAccess, c.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !canWrite {
		return common.ErrPerm
	}
	return nil
}

func (c *ModelConfigAPI) isControllerAdmin() error {
	hasAccess, err := c.auth.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !hasAccess {
		return common.ErrPerm
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
		return common.ErrPerm
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
	checkAgentVersion := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		if v, found := updateAttrs["agent-version"]; found {
			oldVersion, _ := oldConfig.AgentVersion()
			if v != oldVersion.String() {
				return errors.New("agent-version cannot be changed")
			}
		}
		return nil
	}
	// Only controller admins can set trace level debugging on a model.
	checkLogTrace := func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error {
		spec, ok := updateAttrs["logging-config"]
		if !ok {
			return nil
		}
		logCfg, err := loggo.ParseConfigString(spec.(string))
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
			if errors.Cause(err) != common.ErrPerm {
				return errors.Trace(err)
			}
			return errors.New("only controller admins can set a model's logging level to TRACE")
		}
		return nil
	}

	// Replace any deprecated attributes with their new values.
	attrs := config.ProcessDeprecatedAttributes(args.Config)
	return c.backend.UpdateModelConfig(attrs, nil, checkAgentVersion, checkLogTrace)
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

// ModelSequences returns the internal sequences for the apiserver model endpoint.
func (m *ModelConfigAPI) ModelSequences() (params.ModelSequenceResult, error) {
	if err := m.canReadModel(); err != nil {
		return params.ModelSequenceResult{}, errors.Trace(err)
	}
	sequences, err := m.backend.AllSequences()
	if err != nil {
		return params.ModelSequenceResult{}, errors.Trace(err)
	}
	return params.ModelSequenceResult{sequences}, nil
}

// Mask the new methods from the V3 API. The API reflection code in
// rpc/rpcreflect/type.go:newMethod skips 2-argument methods, so this
// removes the method as far as the RPC machinery is concerned.

// ModelSequences isn't on the V1 API.
func (m *ModelConfigAPIV1) ModelSequences(_, _ struct{}) {}
