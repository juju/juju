// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

// PoolCreateAPI defines the API methods that pool create command uses.
type PoolCreateAPI interface {
	Close() error
	CreatePool(pname, ptype string, pconfig map[string]interface{}) error
}

const poolCreateCommandDoc = `
Create or define a storage pool.

Pools are a mechanism for administrators to define sources of storage that
they will use to satisfy service storage requirements.

A single pool might be used for storage from units of many different services -
it is a resource from which different stores may be drawn.

A pool describes provider-specific parameters for creating storage,
such as performance (e.g. IOPS), media type (e.g. magnetic vs. SSD),
or durability.

For many providers, there will be a shared resource
where storage can be requested (e.g. EBS in amazon).
Creating pools there maps provider specific settings
into named resources that can be used during deployment.

Pools defined at the model level are easily reused across services.

options:
    -m, --model (= "")
        juju model to operate in
    -o, --output (= "")
        specify an output file
    <name>
        pool name
    <provider type>
        pool provider type
    <key>=<value> (<key>=<value> ...)
        pool configuration attributes as space-separated pairs, 
        for e.g. tags, size, path, etc...
`

func newPoolCreateCommand() cmd.Command {
	cmd := &poolCreateCommand{}
	cmd.newAPIFunc = func() (PoolCreateAPI, error) {
		return cmd.NewStorageAPI()
	}
	return modelcmd.Wrap(cmd)
}

// poolCreateCommand lists storage pools.
type poolCreateCommand struct {
	PoolCommandBase
	newAPIFunc func() (PoolCreateAPI, error)
	poolName   string
	// TODO(anastasiamac 2015-01-29) type will need to become optional
	// if type is unspecified, use the environment's default provider type
	provider string
	attrs    map[string]interface{}
}

// Init implements Command.Init.
func (c *poolCreateCommand) Init(args []string) (err error) {
	if len(args) < 3 {
		return errors.New("pool creation requires names, provider type and attrs for configuration")
	}

	c.poolName = args[0]
	c.provider = args[1]

	options, err := keyvalues.Parse(args[2:], false)
	if err != nil {
		return err
	}

	if len(options) == 0 {
		return errors.New("pool creation requires attrs for configuration")
	}
	c.attrs = make(map[string]interface{})
	for key, value := range options {
		c.attrs[key] = value
	}
	return nil
}

// Info implements Command.Info.
func (c *poolCreateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "create",
		Args:    "<name> <provider> [<key>=<value> [<key>=<value>...]]",
		Purpose: "create storage pool",
		Doc:     poolCreateCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *poolCreateCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
}

// Run implements Command.Run.
func (c *poolCreateCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()
	return api.CreatePool(c.poolName, c.provider, c.attrs)
}
