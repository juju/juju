// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
)

type orchestrator struct {
	*LogForwarder // For now its just a single forwarder.
}

// OrchestratorArgs holds the info needed to open a log forwarding
// orchestration worker.
type OrchestratorArgs struct {
	// ControllerUUID is the UUID of the controller for which we will forward logs.
	ControllerUUID string

	// LogForwardConfig is the API used to access log forward config.
	LogForwardConfig LogForwardConfig

	// Caller is the API caller that will be used.
	Caller base.APICaller

	// Sinks are the named functions that open the underlying log sinks
	// to which log records will be forwarded.
	Sinks []LogSinkSpec

	// OpenLogStream is the function that will be used to for the
	// log stream.
	OpenLogStream LogStreamFn

	// OpenLogForwarder opens each log forwarder that will be used.
	OpenLogForwarder func(OpenLogForwarderArgs) (*LogForwarder, error)

	Logger Logger
}

func newOrchestratorForController(args OrchestratorArgs) (*orchestrator, error) {
	// For now we work with only 1 forwarder. Later we can have a proper
	// orchestrator that spawns a sub-worker for each log sink.
	if len(args.Sinks) == 0 {
		return nil, nil
	}
	if len(args.Sinks) > 1 {
		return nil, errors.Errorf("multiple log forwarding targets not supported (yet)")
	}
	lf, err := args.OpenLogForwarder(OpenLogForwarderArgs{
		ControllerUUID:   args.ControllerUUID,
		LogForwardConfig: args.LogForwardConfig,
		Caller:           args.Caller,
		Name:             args.Sinks[0].Name,
		OpenSink:         args.Sinks[0].OpenFn,
		OpenLogStream:    args.OpenLogStream,
		Logger:           args.Logger,
	})
	return &orchestrator{lf}, errors.Annotate(err, "opening log forwarder")
}
