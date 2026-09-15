// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshproxy serves the SSH upgrade endpoints on the API server.
// It has two roles:
//
//   - Tunnel endpoint (GET /model/:modeluuid/ssh-tunnel/:tunnelID):
//     machine agents push reverse SSH tunnels to the controller.
//
//   - Relay endpoint (GET /ssh-relay/:virtualHostname): user SSH
//     sessions, relayed blind by an intermediary such as JIMM,
//     terminate in the embedded SSH server built via
//     TerminatingServerFactory.
package sshproxy
