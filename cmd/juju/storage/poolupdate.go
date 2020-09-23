// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/v2/keyvalues"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	Type = "type"
)

// PoolUpdateAPI defines the API methods that the storage commands use.
type PoolUpdateAPI interface {
	Close() error
	UpdatePool(name, provider string, attr map[string]interface{}) error
	BestAPIVersion() int
}

const poolUpdateCommandDoc = `
Update configuration attributes for a single existing storage pool.

Example:
    # Update the storage-pool named iops with new configuration details

      juju update-storage-pool operator-storage volume-type=provisioned-iops iops=40

    # Update which provider the pool is for
      juju update-storage-pool lxd-storage type=lxd-zfs

See also:
    create-storage-pool
    remove-storage-pool
    storage-pools
`

// NewPoolUpdateCommand returns a command that replaces the named storage pools' attributes.
func NewPoolUpdateCommand() cmd.Command {
	cmd := &poolUpdateCommand{}
	cmd.newAPIFunc = func() (PoolUpdateAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// poolUpdateCommand updates a storage pool configuration attributes.
type poolUpdateCommand struct {
	PoolCommandBase
	newAPIFunc  func() (PoolUpdateAPI, error)
	poolName    string
	configAttrs map[string]interface{}
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
	c.configAttrs = make(map[string]interface{})
	for key, value := range config {
		c.configAttrs[key] = value
	}
	return nil
}

// Info implements Command.Info.
func (c *poolUpdateCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "update-storage-pool",
		Purpose: "Update storage pool attributes.",
		Doc:     poolUpdateCommandDoc,
		Args:    "<name> [<key>=<value> [<key>=<value>...]]",
	})
}

// Run implements Command.Run.
func (c *poolUpdateCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()
	if api.BestAPIVersion() < 5 {
		return errors.New("updating storage pools is not supported by this version of Juju")
	}
	err = api.UpdatePool(c.poolName, c.provider, c.configAttrs)
	if err != nil {
		return err
	}
	return nil
}
