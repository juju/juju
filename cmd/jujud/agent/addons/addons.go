// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addons

import (
	"runtime"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/worker/introspection"
)

var logger = loggo.GetLogger("juju.cmd.jujud.agent.addons")

// DefaultIntrospectionSocketName returns the socket name to use for the
// abstract domain socket that the introspection worker serves requests
// over.
func DefaultIntrospectionSocketName(entityTag names.Tag) string {
	return "jujud-" + entityTag.String()
}

// IntrospectionConfig defines the various components that the introspection
// worker reports on or needs to start up.
type IntrospectionConfig struct {
	Agent              agent.Agent
	Engine             *dependency.Engine
	StatePoolReporter  introspection.Reporter
	PubSubReporter     introspection.Reporter
	MachineLock        machinelock.Lock
	PrometheusGatherer prometheus.Gatherer
	PresenceRecorder   presence.Recorder
	Clock              clock.Clock
	LocalHub           *pubsub.SimpleHub
	CentralHub         *pubsub.StructuredHub
	LeaseFSM           *raftlease.FSM

	NewSocketName func(names.Tag) string
	WorkerFunc    func(config introspection.Config) (worker.Worker, error)
}

// StartIntrospection creates the introspection worker. It cannot and should
// not be in the engine itself as it reports on the engine, and other aspects
// of the runtime. If we put it in the engine, then it is mostly likely shut
// down in the times we need it most, which is when the agent is having
// problems shutting down. Here we effectively start the worker and tie its
// life to that of the engine that is returned.
func StartIntrospection(cfg IntrospectionConfig) error {
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
		Clock:              cfg.Clock,
		LocalHub:           cfg.LocalHub,
		CentralHub:         cfg.CentralHub,
		Leases:             cfg.LeaseFSM,
	})
	if err != nil {
		return errors.Trace(err)
	}
	go func() {
		_ = cfg.Engine.Wait()
		logger.Debugf("engine stopped, stopping introspection")
		w.Kill()
		_ = w.Wait()
		logger.Debugf("introspection stopped")
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
