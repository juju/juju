// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type limitedContext struct {
	jujuc.RestrictedContext

	env map[string]string

	unitName string
	id       string
}

// NewLimitedContext creates a new context that implements just the bare minimum
// of the jujuc.Context interface.
func NewLimitedContext(unitName string) *limitedContext {
	id := fmt.Sprintf("%s-%s-%d", unitName, "meter-status", rand.New(rand.NewSource(time.Now().Unix())).Int63())
	return &limitedContext{unitName: unitName, id: id}
}

// HookVars implements runner.Context.
func (ctx *limitedContext) HookVars(paths context.Paths) ([]string, error) {
	vars := []string{
		"JUJU_CHARM_DIR=" + paths.GetCharmDir(),
		"JUJU_CONTEXT_ID=" + ctx.id,
		"JUJU_AGENT_SOCKET=" + paths.GetJujucSocket(),
		"JUJU_UNIT_NAME=" + ctx.unitName,
	}
	for key, val := range ctx.env {
		vars = append(vars, fmt.Sprintf("%s=%s", key, val))
	}
	return append(vars, context.OSDependentEnvVars(paths)...), nil
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
	return ctx.unitName
}

// SetProcess implements runner.Context.
func (ctx *limitedContext) SetProcess(process context.HookProcess) {}

// ActionData implements runner.Context.
func (ctx *limitedContext) ActionData() (*context.ActionData, error) {
	return nil, jujuc.ErrRestrictedContext
}

// Flush implementes runner.Context.
func (ctx *limitedContext) Flush(_ string, err error) error {
	return err
}

// HasExecutionSetUnitStatus implements runner.Context.
func (ctx *limitedContext) HasExecutionSetUnitStatus() bool { return false }

// ResetExecutionSetUnitStatus implements runner.Context.
func (ctx *limitedContext) ResetExecutionSetUnitStatus() {}

// Id implements runner.Context.
func (ctx *limitedContext) Id() string { return ctx.id }

// Prepare implements runner.Context.
func (ctx *limitedContext) Prepare() error {
	return jujuc.ErrRestrictedContext
}

// Component implements runner.Context.
func (ctx *limitedContext) Component(name string) (jujuc.ContextComponent, error) {
	return nil, errors.NotFoundf("context component %q", name)
}
