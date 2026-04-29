// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package apiserver provides the server-side API implementation for Juju controllers.
//
// The server-side API handles authenticated client requests (from CLI commands,
// agents, external tools, etc.) by routing them to versioned RPC facades
// (endpoints organized by client type providing domain-specific operations).
// The Server type manages the API server and routes incoming requests to
// registered facades based on version and client authentication. Facades are
// organized into subpackages by client type (agent, client, controller) and
// registered at specific versions to support API evolution.
//
// See github.com/juju/juju/apiserver/facade for facade registration and
// versioning. See github.com/juju/juju/api for the client-side connection
// primitives. See subpackages (facades/agent, facades/client,
// facades/controller) for specific API implementations.
package apiserver
