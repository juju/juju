// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controlleragentconfig provides a worker that listens on the "/reload"
// endpoint of the config change socket and restarts any workers that have
// requested to watch the config.
// It also supplies the *agent* ID to external requesters via the "/agent-id"
// endpoint.
// It is a worker that listens for SIGHUP signals
// and reloads the agent configuration when one is received.
package controlleragentconfig
