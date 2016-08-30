// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"runtime"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/introspection"
)

// introspectionConfig defines the various components that the introspection
// worker reports on or needs to start up.
type introspectionConfig struct {
	Agent      agent.Agent
	Engine     *dependency.Engine
	WorkerFunc func(config introspection.Config) (worker.Worker, error)
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

	socketName := "jujud-" + cfg.Agent.CurrentConfig().Tag().String()
	w, err := cfg.WorkerFunc(introspection.Config{
		SocketName: socketName,
		Reporter:   cfg.Engine,
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
