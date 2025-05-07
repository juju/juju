// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/testhelpers"
)

type stubCommand struct {
	modelcmd.ModelCommandBase
	stub    *testhelpers.Stub
	info    *cmd.Info
	envName string
}

func (c *stubCommand) Info() *cmd.Info {
	c.stub.AddCall("Info")
	c.stub.NextErr() // pop one off

	if c.info == nil {
		return &cmd.Info{
			Name: "some-command",
		}
	}
	return jujucmd.Info(c.info)
}

func (c *stubCommand) Run(ctx *cmd.Context) error {
	c.stub.AddCall("Run", ctx)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *stubCommand) SetModelName(name string, allowDefault bool) error {
	c.stub.AddCall("SetModelName", name)
	c.envName = name
	return c.stub.NextErr()
}

func (c *stubCommand) ModelName() (string, error) {
	c.stub.AddCall("ModelName")
	c.stub.NextErr() // pop one off

	return c.envName, nil
}

type stubRegistry struct {
	stub *testhelpers.Stub

	names []string
}

func (r *stubRegistry) Register(subcmd cmd.Command) {
	r.stub.AddCall("Register", subcmd)
	r.stub.NextErr() // pop one off

	r.names = append(r.names, subcmd.Info().Name)
	for _, name := range subcmd.Info().Aliases {
		r.names = append(r.names, name)
	}
}

func (r *stubRegistry) RegisterSuperAlias(name, super, forName string, check cmd.DeprecationCheck) {
	r.stub.AddCall("RegisterSuperAlias", name, super, forName)
	r.stub.NextErr() // pop one off

	r.names = append(r.names, name)
}

func (r *stubRegistry) RegisterDeprecated(subcmd cmd.Command, check cmd.DeprecationCheck) {
	r.stub.AddCall("RegisterDeprecated", subcmd, check)
	r.stub.NextErr() // pop one off

	r.names = append(r.names, subcmd.Info().Name)
	for _, name := range subcmd.Info().Aliases {
		r.names = append(r.names, name)
	}
}
