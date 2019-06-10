// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

//go:generate go run ../../../generate/schemagen/schemagen.go allFacadesSchema commands describeapi_schemagen.go

// newDescribeAPICommand returns a full description of the api-servers
// AllFacades information as a JSON schema.
func newDescribeAPICommon() cmd.Command {
	return &describeAPICommand{}
}

type describeAPICommand struct {
	modelcmd.CommandBase
	out cmd.Output
}

const describeAPIHelpDoc = `
describe-api returns a full description of the api-servers AllFacades
information as a JSON schema. 

Examples:

	juju describe-api
`

// Info implements Command.
func (c *describeAPICommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "describe-api",
		Purpose: "Displays the JSON schema of the api-servers AllFacades.",
		Doc:     describeAPIHelpDoc,
	})
}

// SetFlags implements Command.
func (c *describeAPICommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
}

// Init implements Command.
func (c *describeAPICommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// Run implements Command.
func (c *describeAPICommand) Run(ctx *cmd.Context) error {
	_, err := ctx.Stdout.Write([]byte(allFacadesSchema))
	return err
}
