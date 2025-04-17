// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshsession provides a worker for establishing SSH tunnels between
// a controller and a machine.
//
// In order to keep Juju's networking model, machine agents must connect to the controller
// since the controller never connects to machine agents.
// As such, this worker enables reverse SSH connections.
//
// The purpose of this worker is to watch for connection requests and do the following:
//
//  1. Update the authorised_keys for this machine (over the lifetime of the SSH connection). To add/remove
//     the keys we use the EphemeralKeysUpdater interface exposed by the authentication worker.
//  2. Perform an SSH connection to the controller's SSH server using the address provided.
//     It will authenticate using a JWT given in the update and provide it in the password handler.
//
// This allows the SSH server within the controller a means to connect back to this unit's machine.
package sshsession
