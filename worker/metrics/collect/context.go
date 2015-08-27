// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/juju/errors"

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
	id := fmt.Sprintf("%s-%s-%d", unitName, "collect-metrics", rand.New(rand.NewSource(time.Now().Unix())).Int63())
	return &hookContext{unitName: unitName, id: id, recorder: recorder}
}

// HookVars implements runner.Context.
func (ctx *hookContext) HookVars(paths context.Paths) ([]string, error) {
	// TODO(cmars): Provide restricted hook context vars.
	return nil, nil
}

// UnitName implements runner.Context.
func (ctx *hookContext) UnitName() string {
	return ctx.unitName
}

// Flush implements runner.Context.
func (ctx *hookContext) Flush(process string, ctxErr error) (err error) {
	return ctx.recorder.Close()
}

// AddMetric implements runner.Context.
func (ctx *hookContext) AddMetric(key string, value string, created time.Time) error {
	return ctx.recorder.AddMetric(key, value, created)
}

// addJujuUnitsMetric adds the juju-units built in metric if it
// is defined for this context.
func (ctx *hookContext) addJujuUnitsMetric() error {
	if ctx.recorder.IsDeclaredMetric("juju-units") {
		err := ctx.recorder.AddMetric("juju-units", "1", time.Now().UTC())
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// SetProcess implements runner.Context.
func (ctx *hookContext) SetProcess(process *os.Process) {}

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
