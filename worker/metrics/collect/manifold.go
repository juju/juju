// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"path"
	"sync"
	"time"

	corecharm "github.com/juju/charm/v12"
	"github.com/juju/charm/v12/hooks"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/wrench"
)

const (
	defaultSocketName = "metrics-collect.socket"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the logger passed into the manifold.
type logger interface{}

var (
	_ logger = struct{}{}

	defaultPeriod = 5 * time.Minute

	// errMetricsNotDefined is returned when the charm the uniter is running does
	// not declare any metrics.
	errMetricsNotDefined = errors.New("no metrics defined")

	// readCharm function reads the charm directory and extracts declared metrics and the charm url.
	readCharm = func(unitTag names.UnitTag, paths context.Paths) (string, map[string]corecharm.Metric, error) {
		ch, err := corecharm.ReadCharm(paths.GetCharmDir())
		if err != nil {
			return "", nil, errors.Annotatef(err, "failed to read charm from: %v", paths.GetCharmDir())
		}
		chURL, err := charm.ReadCharmURL(path.Join(paths.GetCharmDir(), charm.CharmURLPath))
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		charmMetrics := map[string]corecharm.Metric{}
		if ch.Metrics() != nil {
			charmMetrics = ch.Metrics().Metrics
		}
		return chURL, charmMetrics, nil
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
		return metricFactory.Recorder(charmMetrics, chURL, unitTag.String())
	}

	newSocketListener = func(path string, handler spool.ConnectionHandler) (stopper, error) {
		return spool.NewSocketListener(path, handler)
	}
)

type stopper interface {
	Stop() error
}

// Clock represents time methods used by this package.
type Clock interface {
	Now() time.Time
}

// Logger represents the logging methods used in this package.
type Logger interface {
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})

	Root() loggo.Logger
}

// ManifoldConfig identifies the resource names upon which the collect manifold
// depends.
type ManifoldConfig struct {
	Period *time.Duration

	AgentName       string
	MetricSpoolName string
	CharmDirName    string

	Clock  Clock
	Logger Logger
}

// Manifold returns a collect-metrics manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.MetricSpoolName,
			config.CharmDirName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			collector, err := newCollect(config, context)
			if err != nil {
				return nil, err
			}
			return spool.NewPeriodicWorker(collector.Do, collector.period, jworker.NewTimer, collector.stop), nil
		},
	}
}

func socketName(baseDir, unitTag string) string {
	return path.Join(baseDir, defaultSocketName)
}

func newCollect(config ManifoldConfig, context dependency.Context) (*collect, error) {
	if wrench.IsActive("metricscollector", "short-interval") {
		defaultPeriod = 10 * time.Second
	}

	period := defaultPeriod
	if config.Period != nil {
		period = *config.Period
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var metricFactory spool.MetricFactory
	err := context.Get(config.MetricSpoolName, &metricFactory)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var charmdir fortress.Guest
	err = context.Get(config.CharmDirName, &charmdir)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = charmdir.Visit(func() error {
		return nil
	}, context.Abort())
	if err != nil {
		return nil, errors.Trace(err)
	}

	agentConfig := agent.CurrentConfig()
	tag := agentConfig.Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected a unit tag, got %v", tag)
	}
	paths := uniter.NewWorkerPaths(agentConfig.DataDir(), unitTag, "metrics-collect", nil)
	runner := &hookRunner{
		unitTag: unitTag.String(),
		paths:   paths,
		clock:   config.Clock,
		logger:  config.Logger,
	}
	var listener stopper
	charmURL, validMetrics, err := readCharm(unitTag, paths)
	if err != nil {
		return nil, errors.Trace(err)
	}

	curl, err := corecharm.ParseURL(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(validMetrics) > 0 && curl.Schema == "local" {
		h := newHandler(handlerConfig{
			charmdir:       charmdir,
			agent:          agent,
			unitTag:        unitTag,
			metricsFactory: metricFactory,
			runner:         runner,
		})
		listener, err = newSocketListener(socketName(paths.State.BaseDir, unitTag.String()), h)
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
		runner:        runner,
		logger:        config.Logger,
	}

	return collector, nil
}

type collect struct {
	period        time.Duration
	agent         agent.Agent
	metricFactory spool.MetricFactory
	charmdir      fortress.Guest
	listener      stopper
	runner        *hookRunner
	logger        Logger
}

func (w *collect) stop() {
	if w.listener != nil {
		_ = w.listener.Stop()
	}
}

// Do satisfies the worker.PeriodWorkerCall function type.
func (w *collect) Do(stop <-chan struct{}) (err error) {
	defer func() {
		// See bug https://pad/lv/1733469
		// If this function which is run by a PeriodicWorker
		// exits with an error, we need to call stop() to
		// ensure the listener socket is closed.
		if err != nil {
			w.stop()
		}
	}()

	config := w.agent.CurrentConfig()
	tag := config.Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return errors.Errorf("expected a unit tag, got %v", tag)
	}
	paths := uniter.NewWorkerPaths(config.DataDir(), unitTag, "metrics-collect", nil)

	recorder, err := newRecorder(unitTag, paths, w.metricFactory)
	if errors.Cause(err) == errMetricsNotDefined {
		w.logger.Tracef("%v", err)
		return nil
	} else if err != nil {
		return errors.Annotate(err, "failed to instantiate metric recorder")
	}

	err = w.charmdir.Visit(func() error {
		return w.runner.do(recorder)
	}, stop)
	if err == fortress.ErrAborted {
		w.logger.Tracef("cannot execute collect-metrics: %v", err)
		return nil
	}
	if spool.IsMetricsDataError(err) {
		w.logger.Debugf("cannot record metrics: %v", err)
		return nil
	}
	return err
}

type hookRunner struct {
	m sync.Mutex

	unitTag string
	paths   uniter.Paths

	clock  Clock
	logger Logger
}

func (h *hookRunner) do(recorder spool.MetricRecorder) error {
	h.m.Lock()
	defer h.m.Unlock()
	h.logger.Debugf("recording metrics")

	ctx := newHookContext(hookConfig{
		unitName: h.unitTag,
		recorder: recorder,
		clock:    h.clock,
		logger:   h.logger,
	})
	err := ctx.addJujuUnitsMetric()
	if err != nil {
		return errors.Annotatef(err, "error adding 'juju-units' metric")
	}

	r := runner.NewRunner(ctx, h.paths, nil)
	handlerType, err := r.RunHook(string(hooks.CollectMetrics))
	switch {
	case charmrunner.IsMissingHookError(errors.Cause(err)):
		fallthrough
	case err == nil && handlerType == runner.InvalidHookHandler:
		h.logger.Debugf("skipped %q hook (missing)", hooks.CollectMetrics)
	case err != nil:
		return errors.Annotatef(err, "error running %q hook", hooks.CollectMetrics)
	default:
		h.logger.Debugf("ran %q hook (via %s)", hooks.CollectMetrics, handlerType)
	}

	return nil
}
