// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logstream"
	"github.com/juju/juju/apiserver/params"
)

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	// These are the dependency resource names.
	APICallerName string

	// Sinks are the named functions that opens the underlying log sinks
	// to which log records will be forwarded.
	Sinks []LogSinkSpec

	// OpenLogStream is the function that will be used to for the
	// log stream.
	OpenLogStream LogStreamFn

	// OpenLogForwarder opens each log forwarder that will be used.
	OpenLogForwarder func(OpenLogForwarderArgs) (*LogForwarder, error)

	Logger Logger
}

// Manifold returns a dependency manifold that runs a log forwarding
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	openLogStream := config.OpenLogStream
	if openLogStream == nil {
		openLogStream = func(caller base.APICaller, cfg params.LogStreamConfig, controllerUUID string) (LogStream, error) {
			return logstream.Open(caller, cfg, controllerUUID)
		}
	}

	openForwarder := config.OpenLogForwarder
	if openForwarder == nil {
		openForwarder = NewLogForwarder
	}

	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			agentFacade, err := apiagent.NewState(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			controllerCfg, err := agentFacade.ControllerConfig()
			if err != nil {
				return nil, errors.Annotate(err, "cannot read controller config")
			}

			orchestrator, err := newOrchestratorForController(OrchestratorArgs{
				ControllerUUID:   controllerCfg.ControllerUUID(),
				LogForwardConfig: agentFacade,
				Caller:           apiCaller,
				Sinks:            config.Sinks,
				OpenLogStream:    openLogStream,
				OpenLogForwarder: openForwarder,
				Logger:           config.Logger,
			})
			return orchestrator, errors.Annotate(err, "creating log forwarding orchestrator")
		},
	}
}
