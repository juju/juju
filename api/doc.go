// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package api provides client-side API connections to Juju controllers.
//
// API connections enable clients (CLI commands, agents, external tools, etc.)
// to make authenticated RPC calls to Juju controllers and models. The Open
// function establishes connections using configuration from Info (controller
// addresses, certificates, authentication credentials, etc.) and DialOpts
// (timeouts, retry delays, etc.), returning a Connection interface that
// provides RPC facilities, authentication state, and server metadata.
// Authentication is performed by LoginProvider implementations supporting
// various methods (password, macaroon, session token, etc.).
//
// See github.com/juju/juju/api/connector for higher-level connection
// abstractions. See github.com/juju/juju/api/base for the APICaller interface
// used by facade clients. See subpackages (agent, client, controller, etc.) for
// facade-specific API clients.
package api
