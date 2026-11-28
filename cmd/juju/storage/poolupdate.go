// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/keyvalues"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	Type = "type"
)

// PoolUpdateAPI defines the API methods that the storage commands use.
type PoolUpdateAPI interface {
	Close() error
	UpdatePool(ctx context.Context, name, provider string, attr map[string]any) error
}

const poolUpdateCommandDoc = `
Give the pool name and at least one ` + "`key=value`" + ` argument. Include every
attribute you want to keep. To check the current attributes before updating,
use ` + "`juju storage-pools --name <name>`" + `.

Use ` + "`type=<provider>`" + ` to select a provider. If that is the only argument
after the pool name, the command sends an empty set of attributes, not the
pool's current attributes.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage-pool/
`

const poolUpdateCommandExamples = `
Replace the attributes of the EBS pool named ` + "`workload-storage`" + `:

      juju update-storage-pool workload-storage volume-type=provisioned-iops iops=40

Change the storage provider of the pool named ` + "`workload-storage`" + ` to EBS on an AWS model and
set its attributes:

      juju update-storage-pool workload-storage type=ebs volume-type=gp3
`

// NewPoolUpdateCommand returns a command that replaces the named storage pools' attributes.
func NewPoolUpdateCommand() cmd.Command {
	cmd := &poolUpdateCommand{}
	cmd.newAPIFunc = func(ctx context.Context) (PoolUpdateAPI, error) {
		return cmd.NewStorageAPI(ctx)
	}
	return modelcmd.Wrap(cmd)
}

// poolUpdateCommand updates a storage pool configuration attributes.
type poolUpdateCommand struct {
	PoolCommandBase
	newAPIFunc  func(ctx context.Context) (PoolUpdateAPI, error)
	poolName    string
	configAttrs map[string]any
	provider    string
}

// Init implements Command.Init.
func (c *poolUpdateCommand) Init(args []string) (err error) {
	if len(args) < 2 {
		return errors.New("pool update requires name and configuration attributes")
	}

	c.poolName = args[0]

	config, err := keyvalues.Parse(args[1:], false)
	if err != nil {
		return err
	}

	if providerType, ok := config[Type]; ok {
		delete(config, Type)
		c.provider = providerType
	}
	c.configAttrs = make(map[string]any)
	for key, value := range config {
		c.configAttrs[key] = value
	}
	return nil
}

// Info implements Command.Info.
func (c *poolUpdateCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "update-storage-pool",
		Purpose:  "Replace a storage pool's attributes or change its provider.",
		Doc:      poolUpdateCommandDoc,
		Args:     "<name> <key>=<value> [<key>=<value> ...]",
		Examples: poolUpdateCommandExamples,
		SeeAlso: []string{
			"create-storage-pool",
			"remove-storage-pool",
			"storage-pools",
		},
	})
}

// Run implements Command.Run.
func (c *poolUpdateCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc(ctx)
	if err != nil {
		return err
	}
	defer api.Close()
	err = api.UpdatePool(ctx, c.poolName, c.provider, c.configAttrs)
	if err != nil {
		return err
	}
	return nil
}
