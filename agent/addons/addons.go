// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addons

import (
	"context"
	"path"
	"runtime"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/internal/worker/introspection"
)

// MetricSink describes a way to unregister a model metrics collector. This
// ensures that we correctly tidy up after the removal of a model.
type MetricSink interface {
	dependency.Metrics
	Unregister() bool
}

// IntrospectionSocketName is the name of the socket file inside
// the agent's directory used for introspection calls.
const IntrospectionSocketName = "introspection.socket"

// IntrospectionConfig defines the various components that the introspection
// worker reports on or needs to start up.
type IntrospectionConfig struct {
	AgentDir           string
	Engine             *dependency.Engine
	MachineLock        machinelock.Lock
	PrometheusGatherer prometheus.Gatherer
	Clock              clock.Clock
	Logger             logger.Logger

	WorkerFunc func(config introspection.Config) (worker.Worker, error)
}

// StartIntrospection creates the introspection worker. It cannot and should
// not be in the engine itself as it reports on the engine, and other aspects
// of the runtime. If we put it in the engine, then it is mostly likely shut
// down in the times we need it most, which is when the agent is having
// problems shutting down. Here we effectively start the worker and tie its
// life to that of the engine that is returned.
func StartIntrospection(cfg IntrospectionConfig) error {
	if runtime.GOOS != "linux" {
		cfg.Logger.Debugf(context.TODO(), "introspection worker not supported on %q", runtime.GOOS)
		return nil
	}

	socketName := path.Join(cfg.AgentDir, IntrospectionSocketName)
	w, err := cfg.WorkerFunc(introspection.Config{
		SocketName:         socketName,
		DepEngine:          cfg.Engine,
		MachineLock:        cfg.MachineLock,
		PrometheusGatherer: cfg.PrometheusGatherer,
		// TODO(leases) - add lease introspection
	})
	if err != nil {
		return errors.Trace(err)
	}
	go func() {
		_ = cfg.Engine.Wait()
		cfg.Logger.Debugf(context.TODO(), "engine stopped, stopping introspection")
		w.Kill()
		_ = w.Wait()
		cfg.Logger.Debugf(context.TODO(), "introspection stopped")
	}()

	return nil
}

// NewPrometheusRegistry returns a new prometheus.Registry with
// the Go and process metric collectors registered. This registry
// is exposed by the introspection abstract domain socket on all
// Linux agents.
func NewPrometheusRegistry() (*prometheus.Registry, error) {
	r := prometheus.NewRegistry()
	if err := r.Register(prometheus.NewGoCollector()); err != nil {
		return nil, errors.Trace(err)
	}
	if err := r.Register(prometheus.NewProcessCollector(
		prometheus.ProcessCollectorOpts{})); err != nil {
		return nil, errors.Trace(err)
	}
	return r, nil
}

// RegisterEngineMetrics registers the metrics sink on a prometheus registerer,
// ensuring that we cleanup when the worker has stopped.
func RegisterEngineMetrics(registry prometheus.Registerer, metrics prometheus.Collector, worker worker.Worker, sink MetricSink) error {
	if err := registry.Register(metrics); err != nil {
		return errors.Annotatef(err, "failed to register engine metrics")
	}

	go func() {
		_ = worker.Wait()
		_ = sink.Unregister()
		_ = registry.Unregister(metrics)
	}()
	return nil
}
