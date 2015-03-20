// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

const PoolListCommandDoc = `
Lists storage pools.
The user can filter on pool type, name.

* note use of positional arguments

options:
-e, --environment (= "")
   juju environment to operate in
-o, --output (= "")
   specify an output file
--format (= yaml)
   specify output format (json|tabular|yaml)
--provider
   pool provider type
--name
   pool name

`

// PoolListCommand lists storage pools.
type PoolListCommand struct {
	PoolCommandBase
	Providers []string
	Names     []string
	out       cmd.Output
}

// Init implements Command.Init.
func (c *PoolListCommand) Init(args []string) (err error) {
	return nil
}

// Info implements Command.Info.
func (c *PoolListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list storage pools",
		Doc:     PoolListCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *PoolListCommand) SetFlags(f *gnuflag.FlagSet) {
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
func (c *PoolListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := getPoolListAPI(c)
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

var (
	getPoolListAPI = (*PoolListCommand).getPoolListAPI
)

// PoolListAPI defines the API methods that the storage commands use.
type PoolListAPI interface {
	Close() error
	ListPools(providers, names []string) ([]params.StoragePool, error)
}

func (c *PoolListCommand) getPoolListAPI() (PoolListAPI, error) {
	return c.NewStorageAPI()
}
