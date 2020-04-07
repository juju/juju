// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"fmt"
	"math/rand"
	"path"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type hookContext struct {
	jujuc.RestrictedContext

	unitName string
	id       string
	recorder spool.MetricRecorder
}

func newHookContext(unitName string, recorder spool.MetricRecorder) *hookContext {
	// TODO(fwereade): 2016-03-17 lp:1558657
	id := fmt.Sprintf("%s-%s-%d", unitName, "collect-metrics", rand.New(rand.NewSource(time.Now().Unix())).Int63())
	return &hookContext{unitName: unitName, id: id, recorder: recorder}
}

// HookVars implements runner.Context.
func (ctx *hookContext) HookVars(paths context.Paths, remote bool, getEnv context.GetEnvFunc) ([]string, error) {
	vars := []string{
		"CHARM_DIR=" + paths.GetCharmDir(), // legacy
		"JUJU_CHARM_DIR=" + paths.GetCharmDir(),
		"JUJU_CONTEXT_ID=" + ctx.id,
		"JUJU_AGENT_SOCKET_ADDRESS=" + paths.GetJujucClientSocket(remote).Address,
		"JUJU_AGENT_SOCKET_NETWORK=" + paths.GetJujucClientSocket(remote).Network,
		"JUJU_UNIT_NAME=" + ctx.unitName,
	}
	if remote {
		vars = append(vars,
			"JUJU_AGENT_CA_CERT="+path.Join(paths.GetBaseDir(), caas.CACertFile),
		)
	}
	return append(vars, context.OSDependentEnvVars(paths, getEnv)...), nil
}

// UnitName implements runner.Context.
func (ctx *hookContext) UnitName() string {
	return ctx.unitName
}

// ModelType implements runner.Context
func (ctx *hookContext) ModelType() model.ModelType {
	// Can return IAAS constant because collect-metrics is only used in Uniter.
	// TODO(caas): Required for CAAS support.
	return model.IAAS
}

// Flush implements runner.Context.
func (ctx *hookContext) Flush(process string, ctxErr error) (err error) {
	return ctx.recorder.Close()
}

// AddMetric implements runner.Context.
func (ctx *hookContext) AddMetric(key string, value string, created time.Time) error {
	return ctx.recorder.AddMetric(key, value, created, nil)
}

// AddMetricLabels implements runner.Context.
func (ctx *hookContext) AddMetricLabels(key string, value string, created time.Time, labels map[string]string) error {
	return ctx.recorder.AddMetric(key, value, created, labels)
}

// addJujuUnitsMetric adds the juju-units built in metric if it
// is defined for this context.
func (ctx *hookContext) addJujuUnitsMetric() error {
	if ctx.recorder.IsDeclaredMetric("juju-units") {
		// TODO(fwereade): 2016-03-17 lp:1558657
		err := ctx.recorder.AddMetric("juju-units", "1", time.Now().UTC(), nil)
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
func (ctx *hookContext) Prepare() error {
	return jujuc.ErrRestrictedContext
}

// Component implements runner.Context.
func (ctx *hookContext) Component(name string) (jujuc.ContextComponent, error) {
	return nil, errors.NotFoundf("context component %q", name)
}
