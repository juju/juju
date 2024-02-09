// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"github.com/juju/clock"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/internal/worker/metrics/spool"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
)

var (
	// NewCollect allows patching the function that creates the metric collection
	// entity.
	NewCollect = newCollect

	// NewRecorder allows patching the function that creates the metric recorder.
	NewRecorder = &newRecorder

	// ReadCharm reads the charm directory and returns the charm url and
	// metrics declared by the charm.
	ReadCharm = &readCharm

	// NewSocketListener creates a new socket listener with the provided
	// socket path and connection handler.
	NewSocketListener = &newSocketListener
)

// Ensure hookContext is a runner.Context.
var _ context.Context = (*hookContext)(nil)

type handlerSetterStopper interface {
	SetHandler(spool.ConnectionHandler)
	Stop() error
}

func NewSocketListenerFnc(listener handlerSetterStopper) func(string, spool.ConnectionHandler) (stopper, error) {
	return func(_ string, handler spool.ConnectionHandler) (stopper, error) {
		listener.SetHandler(handler)
		return listener, nil
	}
}

// NewHookContext returns a new hook context used to collect metrics.
// It is exported here for calling from tests, but not patching.
func NewHookContext(unitName string, recorder spool.MetricRecorder) context.Context {
	return newHookContext(hookConfig{
		unitName: unitName,
		recorder: recorder,
		clock:    clock.WallClock,
		logger:   loggo.GetLogger("test"),
	})
}
