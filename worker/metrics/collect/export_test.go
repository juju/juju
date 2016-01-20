// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import "github.com/juju/juju/worker/uniter/runner"

var (
	// NewCollect allows patching the function that creates the metric collection
	// entity.
	NewCollect = newCollect

	// NewRecorder allows patching the function that creates the metric recorder.
	NewRecorder = &newRecorder

	// NewHookContext returns a new hook context used to collect metrics.
	// It is exported here for calling from tests, but not patching.
	NewHookContext = newHookContext

	// ReadCharm reads the charm directory and returns the charm url and
	// metrics declared by the charm.
	ReadCharm = &readCharm
	// SocketPath returns the socket path and can be patched for testing purposes.
	SocketPath = &socketPath
)

// Ensure hookContext is a runner.Context.
var _ runner.Context = (*hookContext)(nil)
