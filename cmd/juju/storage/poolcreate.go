// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/keyvalues"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// PoolCreateAPI defines the API methods that pool create command uses.
type PoolCreateAPI interface {
	Close() error
	CreatePool(ctx context.Context, pname, ptype string, pconfig map[string]interface{}) error
}

const poolCreateCommandDoc = `
Pools are a mechanism for administrators to define sources of storage that
they will use to satisfy application storage requirements.

A single pool might be used for storage from units of many different applications -
it is a resource from which different stores may be drawn.

A pool describes provider-specific parameters for creating storage,
such as performance (e.g. IOPS), media type (e.g. magnetic vs. SSD),
or durability.

For many providers, there will be a shared resource
where storage can be requested (e.g. EBS in amazon).
Creating pools there maps provider specific settings
into named resources that can be used during deployment.

Pools defined at the model level are easily reused across applications.
Pool creation requires a pool name, the provider type and attributes for
configuration as space-separated pairs, e.g. tags, size, path, etc.

For Kubernetes models, the provider type defaults to "kubernetes"
unless otherwise specified.
`

const poolCreateCommandExamples = `
    juju create-storage-pool ebsrotary ebs volume-type=standard
    juju create-storage-pool gcepd storage-provisioner=kubernetes.io/gce-pd [storage-mode=RWX|RWO|ROX] parameters.type=pd-standard

`

// NewPoolCreateCommand returns a command that creates or defines a storage pool
func NewPoolCreateCommand() cmd.Command {
	cmd := &poolCreateCommand{}
	cmd.newAPIFunc = func(ctx context.Context) (PoolCreateAPI, error) {
		return cmd.NewStorageAPI(ctx)
	}
	return modelcmd.Wrap(cmd)
}

// poolCreateCommand lists storage pools.
type poolCreateCommand struct {
	PoolCommandBase
	newAPIFunc func(ctx context.Context) (PoolCreateAPI, error)
	poolName   string
	// TODO(anastasiamac 2015-01-29) type will need to become optional
	// if type is unspecified, use the environment's default provider type
	provider string
	attrs    map[string]interface{}
}

// Init implements Command.Init.
func (c *poolCreateCommand) Init(args []string) (err error) {
	modelType, err := c.ModelType(context.TODO())
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
		Args:     "<name> <provider> [<key>=<value> [<key>=<value>...]]",
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
	api, err := c.newAPIFunc(ctx)
	if err != nil {
		return err
	}
	defer api.Close()
	return api.CreatePool(ctx, c.poolName, c.provider, c.attrs)
}
