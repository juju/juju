// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshproxy holds the wire-level pieces of the SSH tunnel HTTP
// upgrade protocol (GET /model/:modeluuid/ssh-tunnel/:tunnelID): machine
// agents push reverse SSH tunnels to the controller over it. The upgrade
// helpers here are shared by both sides of the handshake, the API
// server's tunnel endpoint and the machine agent's dialer.
package sshproxy
