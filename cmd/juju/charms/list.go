// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

const ListCommandDoc = `
List charm URLs for requested charms.
If no charms are requested, URLs for all charms
in the Juju environment will be listed.
`

// CharmsListAPI defines the API methods that the list command uses.
type CharmsListAPI interface {
	List(names []string) ([]string, error)
	Close() error
}

// ListCommand lists charms URLs.
type ListCommand struct {
	CharmsCommandBase
	Names []string
	out   cmd.Output
}

// Info implements Command.Info.
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Args:    "<charm names>",
		Purpose: "lists charm URLs",
		Doc:     ListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CharmsCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
}

// Init implements Command.Init.
func (c *ListCommand) Init(args []string) (err error) {
	c.Names = args
	return nil
}

var getCharmsListAPI = func(c *ListCommand) (CharmsListAPI, error) {
	return c.NewCharmsClient()
}

// Run implements Command.Run.
func (c *ListCommand) Run(ctx *cmd.Context) (err error) {
	client, err := getCharmsListAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()
	result, err := client.List(c.Names)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, result)
}
