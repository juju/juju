// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshsession provides a worker that runs on the machine agent and
// enables reverse SSH tunnels back to the controller.
//
// Juju units have connectivity to controllers, but the reverse is not true.
// To establish an SSH connection to a machine, the controller records a
// one-shot SSH connection request (keyed by tunnel ID) in the model database.
// The machine agent's sshsession worker watches for these requests and, for
// requests targeting its own machine, reverse-dials the controller's SSH
// server and pipes the resulting tunnel to the local sshd.
//
// The high-level flow for handling a request is:
//  1. Watch for SSH connection request tunnel IDs in the model.
//  2. For each new tunnel ID, read the request and skip it unless it targets
//     this machine.
//  3. Inject the request's ephemeral public key into authorized_keys.
//  4. Reverse-dial one of the request's controller addresses on the controller
//     SSH port, pinning the controller host public key.
//  5. Pipe the resulting tunnel to the local sshd.
//  6. On completion, remove the ephemeral public key.
package sshsession
