// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package rafttransport provides a manifold and worker that manages
// an apiserver-based raft.Transport. The Worker type, returned by
// NewWorker, implements raft.Transport.
//
// The rafttransport worker installs an HTTP handler into the provided
// apiserverhttp.Mux at a configured path. The worker watches the
// central hub for notification of API server addresses, and uses this
// for dialing connections to those API servers.

package rafttransport
