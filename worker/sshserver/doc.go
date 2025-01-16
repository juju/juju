// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshserver provides an SSH server allowing users to connect units
// via a central host.
//
// This server does a few things, requests that come in must go through the jump server.
// The jump server will pipe the connection and pass it into an in-memory instance of another
// SSH server.
//
// This second SSH server (seen within directTCPIPHandlerClosure) will handle the
// the termination of the SSH connections, note, it is not listening on any ports
// because we are passing the piped connection to it, essentially allowing the following
// to work (despite only having one server listening):
// - `ssh -J controller:2223 ubuntu@app.controller.model`
package sshserver
