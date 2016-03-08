// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

const poolListCommandDoc = `
Lists storage pools.
The user can filter on pool type, name.

If no filter is specified, all current pools are listed.
If at least 1 name and type is specified, only pools that match both a name
AND a type from criteria are listed.
If only names are specified, only mentioned pools will be listed.
If only types are specified, all pools of the specified types will be listed.

Both pool types and names must be valid.
Valid pool types are pool types that are registered for Juju model.

options:
-m, --model (= "")
   juju model to operate in
-o, --output (= "")
   specify an output file
--format (= yaml)
   specify output format (json|tabular|yaml)
--provider
   pool provider type
--name
   pool name

`

func newPoolListCommand() cmd.Command {
	cmd := &poolListCommand{}
	cmd.newAPIFunc = func() (PoolListAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// poolListCommand lists storage pools.
type poolListCommand struct {
	PoolCommandBase
	newAPIFunc func() (PoolListAPI, error)
	Providers  []string
	Names      []string
	out        cmd.Output
}

// Init implements Command.Init.
func (c *poolListCommand) Init(args []string) (err error) {
	return nil
}

// Info implements Command.Info.
func (c *poolListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list storage pools",
		Doc:     poolListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *poolListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.Var(cmd.NewAppendStringsValue(&c.Providers), "provider", "only show pools of these provider types")
	f.Var(cmd.NewAppendStringsValue(&c.Names), "name", "only show pools with these names")

	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatPoolListTabular,
	})
}

// Run implements Command.Run.
func (c *poolListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()
	result, err := api.ListPools(c.Providers, c.Names)
	if err != nil {
		return err
	}
	if len(result) == 0 {
		return nil
	}
	output := formatPoolInfo(result)
	return c.out.Write(ctx, output)
}

// PoolListAPI defines the API methods that the storage commands use.
type PoolListAPI interface {
	Close() error
	ListPools(providers, names []string) ([]params.StoragePool, error)
}
