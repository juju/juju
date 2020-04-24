// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"runtime"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/introspection"
)

// DefaultIntrospectionSocketName returns the socket name to use for the
// abstract domain socket that the introspection worker serves requests
// over.
func DefaultIntrospectionSocketName(entityTag names.Tag) string {
	return "jujud-" + entityTag.String()
}

// introspectionConfig defines the various components that the introspection
// worker reports on or needs to start up.
type introspectionConfig struct {
	Agent              agent.Agent
	Engine             *dependency.Engine
	StatePoolReporter  introspection.IntrospectionReporter
	PubSubReporter     introspection.IntrospectionReporter
	MachineLock        machinelock.Lock
	PrometheusGatherer prometheus.Gatherer
	PresenceRecorder   presence.Recorder
	NewSocketName      func(names.Tag) string
	WorkerFunc         func(config introspection.Config) (worker.Worker, error)
}

// startIntrospection creates the introspection worker. It cannot and should
// not be in the engine itself as it reports on the engine, and other aspects
// of the runtime. If we put it in the engine, then it is mostly likely shut
// down in the times we need it most, which is when the agent is having
// problems shutting down. Here we effectively start the worker and tie its
// life to that of the engine that is returned.
func startIntrospection(cfg introspectionConfig) error {
	if runtime.GOOS != "linux" {
		logger.Debugf("introspection worker not supported on %q", runtime.GOOS)
		return nil
	}

	socketName := cfg.NewSocketName(cfg.Agent.CurrentConfig().Tag())
	w, err := cfg.WorkerFunc(introspection.Config{
		SocketName:         socketName,
		DepEngine:          cfg.Engine,
		StatePool:          cfg.StatePoolReporter,
		PubSub:             cfg.PubSubReporter,
		MachineLock:        cfg.MachineLock,
		PrometheusGatherer: cfg.PrometheusGatherer,
		Presence:           cfg.PresenceRecorder,
	})
	if err != nil {
		return errors.Trace(err)
	}
	go func() {
		cfg.Engine.Wait()
		logger.Debugf("engine stopped, stopping introspection")
		w.Kill()
		w.Wait()
		logger.Debugf("introspection stopped")
	}()

	return nil
}

// newPrometheusRegistry returns a new prometheus.Registry with
// the Go and process metric collectors registered. This registry
// is exposed by the introspection abstract domain socket on all
// Linux agents.
func newPrometheusRegistry() (*prometheus.Registry, error) {
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

// statePoolIntrospectionReporter wraps a (possibly nil) state.StatePool,
// calling its IntrospectionReport method or returning a message if it
// is nil.
type statePoolIntrospectionReporter struct {
	mu   sync.Mutex
	pool *state.StatePool
}

func (h *statePoolIntrospectionReporter) set(pool *state.StatePool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pool = pool
}

func (h *statePoolIntrospectionReporter) IntrospectionReport() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pool == nil {
		return "agent has no pool set"
	}
	return h.pool.IntrospectionReport()
}
