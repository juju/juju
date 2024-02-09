// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	stdcontext "context"
	"fmt"
	"math/rand"
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/metrics/spool"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type hookContext struct {
	jujuc.RestrictedContext

	id     string
	config hookConfig
}

type hookConfig struct {
	unitName string
	recorder spool.MetricRecorder
	clock    Clock
	logger   Logger
}

func newHookContext(config hookConfig) *hookContext {
	now := config.clock.Now().Unix()
	id := fmt.Sprintf("%s-%s-%d", config.unitName, "collect-metrics", rand.New(rand.NewSource(now)).Int63())
	return &hookContext{id: id, config: config}
}

// HookVars implements runner.Context.
func (ctx *hookContext) HookVars(
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
	return append(vars, context.OSDependentEnvVars(paths, envVars)...), nil
}

// GetLogger returns the logger for the specified module.
func (ctx *hookContext) GetLogger(module string) loggo.Logger {
	return ctx.config.logger.Root().Child(module)
}

// UnitName implements runner.Context.
func (ctx *hookContext) UnitName() string {
	return ctx.config.unitName
}

// ModelType implements runner.Context
func (ctx *hookContext) ModelType() model.ModelType {
	// Can return IAAS constant because collect-metrics is only used in Uniter.
	// TODO(caas): Required for CAAS support.
	return model.IAAS
}

// Flush implements runner.Context.
func (ctx *hookContext) Flush(_ stdcontext.Context, process string, ctxErr error) (err error) {
	return ctx.config.recorder.Close()
}

// AddMetric implements runner.Context.
func (ctx *hookContext) AddMetric(key string, value string, created time.Time) error {
	return ctx.config.recorder.AddMetric(key, value, created, nil)
}

// AddMetricLabels implements runner.Context.
func (ctx *hookContext) AddMetricLabels(key string, value string, created time.Time, labels map[string]string) error {
	return ctx.config.recorder.AddMetric(key, value, created, labels)
}

// addJujuUnitsMetric adds the juju-units built in metric if it
// is defined for this context.
func (ctx *hookContext) addJujuUnitsMetric() error {
	if ctx.config.recorder.IsDeclaredMetric("juju-units") {
		// TODO(fwereade): 2016-03-17 lp:1558657
		now := ctx.config.clock.Now().UTC()
		err := ctx.config.recorder.AddMetric("juju-units", "1", now, nil)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetProcess implements runner.Context.
func (ctx *hookContext) SetProcess(process context.HookProcess) {}

// ActionData implements runner.Context.
func (ctx *hookContext) ActionData() (*context.ActionData, error) {
	return nil, jujuc.ErrRestrictedContext
}

// HasExecutionSetUnitStatus implements runner.Context.
func (ctx *hookContext) HasExecutionSetUnitStatus() bool { return false }

// ResetExecutionSetUnitStatus implements runner.Context.
func (ctx *hookContext) ResetExecutionSetUnitStatus() {}

// Id implements runner.Context.
func (ctx *hookContext) Id() string { return ctx.id }

// Prepare implements runner.Context.
func (ctx *hookContext) Prepare(stdcontext.Context) error {
	return jujuc.ErrRestrictedContext
}
