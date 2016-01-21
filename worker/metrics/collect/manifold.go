// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package collect provides a worker that executes the collect-metrics hook
// periodically, as long as the workload has been started (between start and
// stop hooks). collect-metrics executes in its own execution context, which is
// restricted to avoid contention with uniter "lifecycle" hooks.
package collect

import (
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charm.v6-unstable/hooks"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
)

const defaultPeriod = 5 * time.Minute

var (
	logger = loggo.GetLogger("juju.worker.metrics.collect")

	// errMetricsNotDefined is returned when the charm the uniter is running does
	// not declared any metrics.
	errMetricsNotDefined = errors.New("no metrics defined")

	// readCharm function reads the charm directory and extracts declared metrics and the charm url.
	readCharm = func(unitTag names.UnitTag, paths context.Paths) (*corecharm.URL, map[string]corecharm.Metric, error) {
		ch, err := corecharm.ReadCharm(paths.GetCharmDir())
		if err != nil {
			return nil, nil, errors.Annotatef(err, "failed to read charm from: %v", paths.GetCharmDir())
		}
		chURL, err := charm.ReadCharmURL(path.Join(paths.GetCharmDir(), charm.CharmURLPath))
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		charmMetrics := map[string]corecharm.Metric{}
		if ch.Metrics() != nil {
			charmMetrics = ch.Metrics().Metrics
		}
		return chURL, charmMetrics, nil
	}

	socketPath = func(p uniter.Paths) string {
		return path.Join(p.State.BaseDir, "metrics-collect.socket")
	}

	// newRecorder returns a struct that implements the spool.MetricRecorder
	// interface.
	newRecorder = func(unitTag names.UnitTag, paths context.Paths, metricFactory spool.MetricFactory) (spool.MetricRecorder, error) {
		chURL, charmMetrics, err := readCharm(unitTag, paths)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(charmMetrics) == 0 {
			return nil, errMetricsNotDefined
		}
		return metricFactory.Recorder(charmMetrics, chURL.String(), unitTag.String())
	}
)

// ManifoldConfig identifies the resource names upon which the collect manifold
// depends.
type ManifoldConfig struct {
	Period *time.Duration

	AgentName       string
	MetricSpoolName string
	CharmDirName    string
}

// Manifold returns a collect-metrics manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.MetricSpoolName,
			config.CharmDirName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			collector, err := newCollect(config, getResource)
			if err != nil {
				return nil, err
			}
			return newPeriodicWorker(collector.Do, collector.period, worker.NewTimer, collector.stop), nil
		},
	}
}

// newPeriodicWorker returns a periodic worker, that will call a stop function
// when it is killed.
func newPeriodicWorker(do worker.PeriodicWorkerCall, period time.Duration, newTimer func(time.Duration) worker.PeriodicTimer, stop func()) worker.Worker {
	return &periodicWorker{
		Worker: worker.NewPeriodicWorker(do, period, newTimer),
		stop:   stop,
	}
}

type periodicWorker struct {
	worker.Worker
	stop func()
}

// Kill implements the worker.Worker interface.
func (w *periodicWorker) Kill() {
	w.stop()
	w.Worker.Kill()
}

func newCollect(config ManifoldConfig, getResource dependency.GetResourceFunc) (*collect, error) {
	period := defaultPeriod
	if config.Period != nil {
		period = *config.Period
	}

	var agent agent.Agent
	if err := getResource(config.AgentName, &agent); err != nil {
		return nil, err
	}

	var metricFactory spool.MetricFactory
	err := getResource(config.MetricSpoolName, &metricFactory)
	if err != nil {
		return nil, err
	}

	var charmdir fortress.Guest
	err = getResource(config.CharmDirName, &charmdir)
	if err != nil {
		return nil, err
	}

	agentConfig := agent.CurrentConfig()
	tag := agentConfig.Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected a unit tag, got %v", tag)
	}
	paths := uniter.NewWorkerPaths(agentConfig.DataDir(), unitTag, "metrics-collect")
	var listener *socketListener
	charmURL, validMetrics, err := readCharm(unitTag, paths)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(validMetrics) > 0 && charmURL.Schema == "local" {
		listener, err = newSocketListener(
			socketPath(paths),
			socketListenerConfig{
				unitTag:      unitTag,
				charmURL:     charmURL,
				validMetrics: validMetrics,
				paths:        paths,
			})
		if err != nil {
			return nil, err
		}
	}
	collector := &collect{
		period:        period,
		agent:         agent,
		metricFactory: metricFactory,
		charmdir:      charmdir,
		listener:      listener,
	}

	return collector, nil
}

type collect struct {
	period        time.Duration
	agent         agent.Agent
	metricFactory spool.MetricFactory
	charmdir      fortress.Guest
	listener      *socketListener
}

func (w *collect) stop() {
	if w.listener != nil {
		w.listener.stop()
	}
}

// Do satisfies the worker.PeriodWorkerCall function type.
func (w *collect) Do(stop <-chan struct{}) error {
	config := w.agent.CurrentConfig()
	tag := config.Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return errors.Errorf("expected a unit tag, got %v", tag)
	}
	paths := uniter.NewWorkerPaths(config.DataDir(), unitTag, "metrics-collect")

	recorder, err := newRecorder(unitTag, paths, w.metricFactory)
	if errors.Cause(err) == errMetricsNotDefined {
		logger.Tracef("%v", err)
		return nil
	} else if err != nil {
		return errors.Annotate(err, "failed to instantiate metric recorder")
	}

	err = w.charmdir.Visit(func() error {
		return do(unitTag.String(), paths, recorder)
	}, stop)
	if err == fortress.ErrAborted {
		logger.Tracef("cannot execute collect-metrics: %v", err)
		return nil
	}
	return err
}

func do(unitTag string, paths uniter.Paths, recorder spool.MetricRecorder) error {
	logger.Tracef("recording metrics")

	ctx := newHookContext(unitTag, recorder)
	err := ctx.addJujuUnitsMetric()
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
