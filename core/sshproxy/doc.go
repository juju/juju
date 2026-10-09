// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshproxy holds the wire-level pieces of the SSH tunnel and relay
// HTTP upgrade protocols: machine agents push reverse SSH tunnels to the
// controller over the tunnel endpoint
// (GET /model/:modeluuid/ssh-tunnel/:tunnelID), and JIMM relays user SSH
// sessions over the relay endpoint (GET /ssh-relay/:virtualHostname). The
// upgrade helpers here are shared by both sides of each handshake, the API
// server's endpoints and the machine agent's dialer.
package sshproxy
