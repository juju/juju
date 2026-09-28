// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshproxy serves the SSH tunnel upgrade endpoint on the API
// server (GET /model/:modeluuid/ssh-tunnel/:tunnelID): machine agents push
// reverse SSH tunnels to the controller. The wire-level upgrade helpers
// shared with the agent-side dialer live in core/sshproxy, since this
// package's internal path is only importable from within apiserver.
package sshproxy
