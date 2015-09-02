// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package docker exposes an API to convert Jujuisms to dockerisms.
package docker

import (
	"bytes"
)

// Client represents a client to docker's API.
type Client interface {
	// Run runs a new docker container with the given info.
	Run(args RunArgs) (string, error)

	// Inspect gets info about the given container ID (or name).
	Inspect(id string) (*Info, error)

	// Stop stops the identified container.
	Stop(id string) error

	// Remove removes the identified container.
	Remove(id string) error
}

// CLIClient is a Client that wraps CLI execution of the docker command.
type CLIClient struct {
	// RunDocker executes the provided docker sub-command and args.
	RunDocker func(string, ...string) ([]byte, error)
}

// NewCLIClient returns a new CLIClient.
func NewCLIClient() *CLIClient {
	cli := &CLIClient{
		RunDocker: runDocker,
	}
	return cli
}

// Run runs a new docker container with the given info.
func (cli *CLIClient) Run(args RunArgs) (string, error) {
	cmdArgs := args.CommandlineArgs()
	out, err := cli.RunDocker("run", cmdArgs...)
	if err != nil {
		return "", err
	}
	id := string(bytes.TrimSpace(out))
	return id, nil
}

// Inspect gets info about the given container ID (or name).
func (cli *CLIClient) Inspect(id string) (*Info, error) {
	out, err := cli.RunDocker("inspect", id)
	if err != nil {
		return nil, err
	}

	info, err := ParseInfoJSON(id, out)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// Stop stops the identified container.
func (cli *CLIClient) Stop(id string) error {
	if _, err := cli.RunDocker("stop", id); err != nil {
		return err
	}
	return nil
}

// Remove removes the identified container.
func (cli *CLIClient) Remove(id string) error {
	if _, err := cli.RunDocker("rm", id); err != nil {
		return err
	}
	return nil
}
