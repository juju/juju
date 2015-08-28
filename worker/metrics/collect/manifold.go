// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package collect provides a worker that executes the collect-metrics hook
// periodically, as long as the workload has been started (between start and
// stop hooks). collect-metrics executes in its own execution context, which is
// restricted to avoid contention with uniter "lifecycle" hooks.
package collect

import (
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
	"github.com/juju/juju/worker/charmdir"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
)

const defaultPeriod = 5 * time.Minute

var (
	logger = loggo.GetLogger("juju.worker.metrics.collect")
)

// ManifoldConfig identifies the resource names upon which the collect manifold
// depends.
type ManifoldConfig struct {
	Period *time.Duration

	AgentName       string
	APICallerName   string
	MetricSpoolName string
	CharmDirName    string
}

// Manifold returns a collect-metrics manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.MetricSpoolName,
			config.CharmDirName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			collector, err := newCollect(config, getResource)
			if err != nil {
				return nil, err
			}
			return worker.NewPeriodicWorker(collector.Do, collector.period, worker.NewTimer), nil
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

	var charmdir charmdir.Consumer
	err = getResource(config.CharmDirName, &charmdir)
	if err != nil {
		return nil, err
	}

	collector := &collect{
		period:          period,
		agent:           agent,
		unitCharmLookup: &unitCharmLookup{uniterFacade},
		metricFactory:   metricFactory,
		charmdir:        charmdir,
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
	period          time.Duration
	agent           agent.Agent
	unitCharmLookup UnitCharmLookup
	metricFactory   spool.MetricFactory
	charmdir        charmdir.Consumer
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
	ok, err := w.charmdir.Run(w.do)
	if !ok {
		logger.Tracef("charmdir not available")
	}
	return err
}

func (w *collect) do() error {
	logger.Tracef("recording metrics")

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

	ctx := newHookContext(unitTag.String(), recorder)
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
