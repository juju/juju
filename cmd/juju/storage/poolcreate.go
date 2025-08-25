// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/utils/v3/keyvalues"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
)

// PoolCreateAPI defines the API methods that pool create command uses.
type PoolCreateAPI interface {
	Close() error
	CreatePool(pname, ptype string, pconfig map[string]interface{}) error
}

const poolCreateCommandDoc = `
Further reading:

- https://documentation.ubuntu.com/juju/3.6/reference/storage/#storage-pool
- https://documentation.ubuntu.com/juju/3.6/reference/storage/#storage-provider

`

const poolCreateCommandExamples = `
    juju create-storage-pool ebsrotary ebs volume-type=standard
    juju create-storage-pool gcepd storage-provisioner=kubernetes.io/gce-pd [storage-mode=RWX|RWO|ROX] parameters.type=pd-standard

`

// NewPoolCreateCommand returns a command that creates or defines a storage pool
func NewPoolCreateCommand() cmd.Command {
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
	modelType, err := c.ModelType()
	if err != nil {
		return errors.Trace(err)
	}
	if modelType == model.CAAS && len(args) > 0 {
		if len(args) == 1 {
			args = []string{args[0], string(k8sconstants.StorageProviderType)}
		}
		if strings.Contains(args[1], "=") {
			newArgs := []string{args[0], string(k8sconstants.StorageProviderType)}
			args = append(newArgs, args[1:]...)
		}
	}
	if len(args) < 2 {
		return errors.New("pool creation requires names, provider type and optional attributes for configuration")
	}

	c.poolName = args[0]
	c.provider = args[1]

	// poolName and provider can contain any character, except for '='.
	// However, the last arguments are always expected to be key=value pairs.
	// Since it's possible for users to mistype, we want to check here for cases
	// such as:
	//    $ juju create-storage-pool poolName key=value
	//    $ juju create-storage-pool key=value poolName
	// as either a provider or a pool name are missing.

	if strings.Contains(c.poolName, "=") || strings.Contains(c.provider, "=") {
		return errors.New("pool creation requires names and provider type before optional attributes for configuration")
	}

	options, err := keyvalues.Parse(args[2:], false)
	if err != nil {
		return err
	}

	c.attrs = make(map[string]interface{})
	if len(options) == 0 {
		return nil
	}
	for key, value := range options {
		c.attrs[key] = value
	}
	return nil
}

// Info implements Command.Info.
func (c *poolCreateCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "create-storage-pool",
		Args:     "<name> <storage provider> [<key>=<value> [<key>=<value>...]]",
		Purpose:  "Create or define a storage pool.",
		Doc:      poolCreateCommandDoc,
		Examples: poolCreateCommandExamples,
		SeeAlso: []string{
			"remove-storage-pool",
			"update-storage-pool",
			"storage-pools",
		},
	})
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
