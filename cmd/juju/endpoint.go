// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
)

// EndpointCommand returns the api endpoint
type EndpointCommand struct {
	cmd.EnvCommandBase
}

func (c *EndpointCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "endpoint",
		Args:    "",
		Purpose: "Print the api server address",
	}
}

func (c *EndpointCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}


// Print out the addresses of the api server endpoints.
func (c *EndpointCommand) Run(ctx *cmd.Context) error {
	environ, err := environs.NewFromName(c.EnvName)
	if err != nil {
		return err
	}

	_, api_info, err := environ.StateInfo()
	if err != nil {
		return err
	}

	for _, url := range(api_info.Addrs) {
		url = fmt.Sprintf("%s\n", url)
		_, err := ctx.Stdout.Write([]byte(url))
		if err != nil {
			return err
		}

	}
	return nil
}
