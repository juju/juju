// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// PoolCommandBase is a helper base structure for pool commands.
type PoolCommandBase struct {
	StorageCommandBase
}

// PoolInfo defines the serialization behaviour of the storage pool information.
type PoolInfo struct {
	Provider string         `yaml:"provider" json:"provider"`
	Attrs    map[string]any `yaml:"attrs,omitempty" json:"attrs,omitempty"`
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
Use ` + "`--name`" + ` to select a pool; a positional name does not filter the
list. To give several names or providers, repeat the corresponding flag.

The default table shows each pool's name, provider and attributes. In
JSON and YAML, the pool names are the keys. If no pools match, the
command succeeds without writing a table or a JSON or YAML document.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage-pool/
`

const poolListCommandExample = `
List the pools in the selected model:

    juju storage-pools

Show two pools by name:

    juju storage-pools --name rootfs --name tmpfs

On an AWS model, show the pools for the EBS and loop providers as YAML:

    juju storage-pools --provider ebs --provider loop --format yaml
`

// NewPoolListCommand returns a command that lists storage pools on a model
func NewPoolListCommand() cmd.Command {
	cmd := &poolListCommand{}
	cmd.newAPIFunc = func(ctx context.Context) (PoolListAPI, error) {
		return cmd.NewStorageAPI(ctx)
	}
	return modelcmd.Wrap(cmd)
}

// poolListCommand lists storage pools.
type poolListCommand struct {
	PoolCommandBase
	newAPIFunc func(ctx context.Context) (PoolListAPI, error)
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
		Name:     "storage-pools",
		Purpose:  "List storage pools.",
		Doc:      poolListCommandDoc,
		Aliases:  []string{"list-storage-pools"},
		Examples: poolListCommandExample,
		SeeAlso: []string{
			"create-storage-pool",
			"remove-storage-pool",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *poolListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.Var(cmd.NewAppendStringsValue(&c.Providers), "provider", "Filter by provider; repeat for each provider")
	f.Var(cmd.NewAppendStringsValue(&c.Names), "name", "Filter by pool name; repeat for each name")

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatPoolListTabular,
	})
}

// Run implements Command.Run.
func (c *poolListCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc(ctx)
	if err != nil {
		return err
	}
	defer api.Close()
	result, err := api.ListPools(ctx, c.Providers, c.Names)
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
	ListPools(ctx context.Context, providers, names []string) ([]params.StoragePool, error)
}
