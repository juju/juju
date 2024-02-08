// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// PoolCommandBase is a helper base structure for pool commands.
type PoolCommandBase struct {
	StorageCommandBase
}

// PoolInfo defines the serialization behaviour of the storage pool information.
type PoolInfo struct {
	Provider string                 `yaml:"provider" json:"provider"`
	Attrs    map[string]interface{} `yaml:"attrs,omitempty" json:"attrs,omitempty"`
}

func formatPoolInfo(all []params.StoragePool) map[string]PoolInfo {
	output := make(map[string]PoolInfo)
	for _, one := range all {
		output[one.Name] = PoolInfo{
			Provider: one.Provider,
			Attrs:    one.Attrs,
		}
	}
	return output
}

const poolListCommandDoc = `
The user can filter on pool type, name.

If no filter is specified, all current pools are listed.
If at least 1 name and type is specified, only pools that match both a name
AND a type from criteria are listed.
If only names are specified, only mentioned pools will be listed.
If only types are specified, all pools of the specified types will be listed.

Both pool types and names must be valid.
Valid pool types are pool types that are registered for Juju model.
`

// NewPoolListCommand returns a command that lists storage pools on a model
func NewPoolListCommand() cmd.Command {
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
	return jujucmd.Info(&cmd.Info{
		Name:    "storage-pools",
		Purpose: "List storage pools.",
		Doc:     poolListCommandDoc,
		Aliases: []string{"list-storage-pools"},
	})
}

// SetFlags implements Command.SetFlags.
func (c *poolListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.Var(cmd.NewAppendStringsValue(&c.Providers), "provider", "Only show pools of these provider types")
	f.Var(cmd.NewAppendStringsValue(&c.Names), "name", "Only show pools with these names")

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
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
		ctx.Infof("No storage pools to display.")
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
