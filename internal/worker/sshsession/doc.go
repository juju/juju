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
package sshsession
