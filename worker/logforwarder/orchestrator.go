// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
)

type orchestrator struct {
	*LogForwarder // For now its just a single forwarder.
}

// OrchestratorArgs holds the info needed to open a log forwarding
// orchestration worker.
type OrchestratorArgs struct {
	// Config is the model config that will be used.
	Config *config.Config

	// Caller is the API caller that will be used.
	Caller base.APICaller

	// SinkOpeners are the functions that open the underlying log sinks
	// to which log records will be forwarded.
	SinkOpeners []LogSinkFn

	// OpenLogStream is the function that will be used to for the
	// log stream.
	OpenLogStream LogStreamFn

	// OpenLogForwarder opens each log forwarder that will be used.
	OpenLogForwarder func(OpenLogForwarderArgs) (*LogForwarder, error)
}

func newOrchestratorForController(args OrchestratorArgs) (*orchestrator, error) {
	if args.Config.Name() != environs.ControllerModelName {
		return nil, errors.New("model-level log forwarding not supported")
	}
	controllerUUID := args.Config.UUID() // This won't work for per-model forwarding.

	// For now we work with only 1 forwarder. Later we can have a proper
	// orchestrator that spawns a sub-worker for each log sink.
	if len(args.SinkOpeners) == 0 {
		return nil, nil
	}
	if len(args.SinkOpeners) > 1 {
		return nil, errors.Errorf("multiple log forwarding targets not supported (yet)")
	}
	lf, err := args.OpenLogForwarder(OpenLogForwarderArgs{
		AllModels:      true,
		ControllerUUID: controllerUUID,
		Config:         args.Config,
		Caller:         args.Caller,
		OpenSink:       args.SinkOpeners[0],
		OpenLogStream:  args.OpenLogStream,
	})
	return &orchestrator{lf}, errors.Annotate(err, "opening log forwarder")
}
