// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"io"
)

// RemoveMachineFunc that every provisioner should have
type RemoveMachineFunc func(RemoveMachineArgs) error

// RemoveMachineArgs used for arguments for the Remover methods
type RemoveMachineArgs struct {
	// user and host of the ssh or winrm conn
	Host string
	User string

	// Stdin is required to respond to sudo prompts,
	// and must be a terminal (except in tests)
	Stdin io.Reader

	// Stdout is required to present sudo prompts to the user.
	Stdout io.Writer

	// Stderr is required to present machine provisioning progress to the user.
	Stderr io.Writer

	// CommandExec executes commands based on inputs passed in
	CommandExec CommandExec

	// Winrm client to execute commands based on inputs.
	// TODO (stickupkid): we should attempt to merge CommandExec with
	// WinrmClientAPI, as they're _almost_ similar.
	WinrmClientAPI
}

// CommandExec runs a command on a given host with various options.
type CommandExec interface {
	// Command returns a command for executing.
	Command(host string, command []string) CommandRunner
}

// CommandRunner runs a given command
type CommandRunner interface {
	// Set the various stdin, out, errs
	SetStdin(io.Reader)
	SetStdout(io.Writer)
	SetStderr(io.Writer)

	// Run runs the command, and returns the result as an error.
	Run() error
}
