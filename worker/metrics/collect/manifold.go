// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package collect provides a worker that executes the collect-metrics hook
// periodically, as long as the workload has been started (between start and
// stop hooks). collect-metrics executes in its own execution context, which is
// restricted to avoid contention with uniter "lifecycle" hooks.
package collect

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	uniterapi "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniteravailability"
)

const defaultPeriod = 5 * time.Minute

var (
	logger = loggo.GetLogger("juju.worker.metrics.collect")
)

// ManifoldConfig identifies the resource names upon which the collect manifold
// depends.
type ManifoldConfig struct {
	Period *time.Duration

	AgentName              string
	APICallerName          string
	MetricSpoolName        string
	UniterAvailabilityName string
}

// Manifold returns a collect-metrics manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.MetricSpoolName,
			config.UniterAvailabilityName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			collector, err := newCollect(config, getResource)
			if err != nil {
				return nil, err
			}
			return worker.NewPeriodicWorker(collector.Do, collector.period), nil
		},
	}
}

// UnitCharmLookup can look up the charm URL for a unit tag.
type UnitCharmLookup interface {
	CharmURL(names.UnitTag) (*corecharm.URL, error)
}

var newCollect = func(config ManifoldConfig, getResource dependency.GetResourceFunc) (*collect, error) {
	period := defaultPeriod
	if config.Period != nil {
		period = *config.Period
	}

	var agent agent.Agent
	if err := getResource(config.AgentName, &agent); err != nil {
		return nil, err
	}
	tag := agent.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected a unit tag, got %v", tag)
	}

	var apiCaller base.APICaller
	if err := getResource(config.APICallerName, &apiCaller); err != nil {
		return nil, err
	}
	uniterFacade := uniterapi.NewState(apiCaller, unitTag)

	var metricFactory spool.MetricFactory
	err := getResource(config.MetricSpoolName, &metricFactory)
	if err != nil {
		return nil, err
	}

	var uniterAvailability uniteravailability.UniterAvailabilityGetter
	err = getResource(config.UniterAvailabilityName, &uniterAvailability)
	if err != nil {
		return nil, err
	}

	collector := &collect{
		period:             period,
		agent:              agent,
		unitCharmLookup:    &unitCharmLookup{uniterFacade},
		metricFactory:      metricFactory,
		uniterAvailability: uniterAvailability,
	}
	return collector, nil
}

type unitCharmLookup struct {
	st *uniterapi.State
}

// CharmURL implements UnitCharmLookup.
func (r *unitCharmLookup) CharmURL(unitTag names.UnitTag) (*corecharm.URL, error) {
	unit, err := r.st.Unit(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return unit.CharmURL()
}

type collect struct {
	period             time.Duration
	agent              agent.Agent
	unitCharmLookup    UnitCharmLookup
	metricFactory      spool.MetricFactory
	uniterAvailability uniteravailability.UniterAvailabilityGetter
}

var newRecorder = func(unitTag names.UnitTag, paths context.Paths, unitCharm UnitCharmLookup, metricFactory spool.MetricFactory) (spool.MetricRecorder, error) {
	ch, err := corecharm.ReadCharm(paths.GetCharmDir())
	if err != nil {
		return nil, errors.Trace(err)
	}

	chURL, err := unitCharm.CharmURL(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	charmMetrics := map[string]corecharm.Metric{}
	if ch.Metrics() != nil {
		charmMetrics = ch.Metrics().Metrics
	}
	return metricFactory.Recorder(charmMetrics, chURL.String(), unitTag.String())
}

// Do satisfies the worker.PeriodWorkerCall function type.
func (w *collect) Do(stop <-chan struct{}) error {
	if !w.uniterAvailability.Available() {
		logger.Tracef("uniter not available")
		return nil
	}

	config := w.agent.CurrentConfig()
	tag := config.Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return errors.Errorf("expected a unit tag, got %v", tag)
	}
	paths := uniter.NewPaths(config.DataDir(), unitTag)

	recorder, err := newRecorder(unitTag, paths, w.unitCharmLookup, w.metricFactory)
	if err != nil {
		return errors.Annotate(err, "failed to instantiate metric recorder")
	}

	ctx := &hookContext{unitName: unitTag.String(), recorder: recorder}
	err = ctx.addJujuUnitsMetric()
	if err != nil {
		return errors.Annotatef(err, "error adding 'juju-units' metric")
	}

	r := runner.NewRunner(ctx, paths)
	err = r.RunHook(string(hooks.CollectMetrics))
	if err != nil {
		return errors.Annotatef(err, "error running 'collect-metrics' hook")
	}
	return nil
}

type hookContext struct {
	// TODO(cmars): deal with unimplemented methods in a better way than
	// panicking. Need a proper restricted hook context.
	runner.Context

	unitName string
	recorder spool.MetricRecorder
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
func (ctx *hookContext) SetProcess(process *os.Process) {
}
