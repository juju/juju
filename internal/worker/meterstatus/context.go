// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	stdcontext "context"
	"fmt"
	"math/rand"
	"path"

	"github.com/juju/loggo"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type limitedContext struct {
	jujuc.RestrictedContext

	env map[string]string

	id     string
	config hookConfig
}

type hookConfig struct {
	unitName string
	clock    Clock
	logger   Logger
}

// newLimitedContext creates a new context that implements just the bare minimum
// of the hooks.Context interface.
func newLimitedContext(config hookConfig) *limitedContext {
	now := config.clock.Now().Unix()
	id := fmt.Sprintf("%s-%s-%d", config.unitName, "meter-status", rand.New(rand.NewSource(now)).Int63())
	return &limitedContext{id: id, config: config}
}

// HookVars implements runner.Context.
func (ctx *limitedContext) HookVars(
	_ stdcontext.Context,
	paths context.Paths,
	remote bool,
	envVars context.Environmenter,
) ([]string, error) {
	vars := []string{
		"CHARM_DIR=" + paths.GetCharmDir(), // legacy
		"JUJU_CHARM_DIR=" + paths.GetCharmDir(),
		"JUJU_CONTEXT_ID=" + ctx.id,
		"JUJU_AGENT_SOCKET_ADDRESS=" + paths.GetJujucClientSocket(remote).Address,
		"JUJU_AGENT_SOCKET_NETWORK=" + paths.GetJujucClientSocket(remote).Network,
		"JUJU_UNIT_NAME=" + ctx.config.unitName,
	}
	if remote {
		vars = append(vars,
			"JUJU_AGENT_CA_CERT="+path.Join(paths.GetBaseDir(), caas.CACertFile),
		)
	}
	for key, val := range ctx.env {
		vars = append(vars, fmt.Sprintf("%s=%s", key, val))
	}
	return append(vars, context.OSDependentEnvVars(paths, envVars)...), nil
}

// GetLogger returns the logger for the specified module.
func (ctx *limitedContext) GetLogger(module string) loggo.Logger {
	return ctx.config.logger.Root().Child(module)
}

// SetEnvVars sets additional environment variables to be exported by the context.
func (ctx *limitedContext) SetEnvVars(vars map[string]string) {
	if ctx.env == nil {
		ctx.env = vars
		return
	}
	for key, val := range vars {
		ctx.env[key] = val
	}
}

// UnitName implements runner.Context.
func (ctx *limitedContext) UnitName() string {
	return ctx.config.unitName
}

// ModelType implements runner.Context
func (ctx *limitedContext) ModelType() model.ModelType {
	// Can return IAAS constant because meter status is only used in Uniter.
	// TODO(caas): Required for CAAS support.
	return model.IAAS
}

// SetProcess implements runner.Context.
func (ctx *limitedContext) SetProcess(process context.HookProcess) {}

// ActionData implements runner.Context.
func (ctx *limitedContext) ActionData() (*context.ActionData, error) {
	return nil, jujuc.ErrRestrictedContext
}

// Flush implements runner.Context.
func (ctx *limitedContext) Flush(_ stdcontext.Context, _ string, err error) error {
	return err
}

// HasExecutionSetUnitStatus implements runner.Context.
func (ctx *limitedContext) HasExecutionSetUnitStatus() bool { return false }

// ResetExecutionSetUnitStatus implements runner.Context.
func (ctx *limitedContext) ResetExecutionSetUnitStatus() {}

// Id implements runner.Context.
func (ctx *limitedContext) Id() string { return ctx.id }

// Prepare implements runner.Context.
func (ctx *limitedContext) Prepare(stdcontext.Context) error {
	return jujuc.ErrRestrictedContext
}
