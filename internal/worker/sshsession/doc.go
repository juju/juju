// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ale8k): Write this better.
// Package sshsession provides a worker that runs an SSH session.
//
// In order to keep Juju's networking model, we need the machine agents
// to connect to the controller (as the controller never connects to machine agents and
// instead it is the otherway around). As such, this worker enables reverse SSH connections.
//
// The purpose of this worker is to watch for incoming connections over <insert real name here>
// facade method and upon receiving an update, do the following:
//
//  1. Update the authorised_keys for this machine (over the lifetime of the SSH connection).
//  2. Perform an SSH connection to the controller's SSH server using the address provided.
//     It will authenticate using a JWT given in the update and provide it in the password handler.
//
// This allows the SSH server within the controller a means to connect back to this unit's machine.
package sshsession
