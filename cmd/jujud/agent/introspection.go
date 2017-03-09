// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"runtime"

	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	names "gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/dependency"
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
	PrometheusGatherer prometheus.Gatherer
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
		PrometheusGatherer: cfg.PrometheusGatherer,
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
	if err := r.Register(prometheus.NewProcessCollector(os.Getpid(), "")); err != nil {
		return nil, errors.Trace(err)
	}
	return r, nil
}

func (h *statePoolHolder) IntrospectionReport() string {
	if h.pool == nil {
		return "agent has no pool set"
	}
	return h.pool.IntrospectionReport()
}
