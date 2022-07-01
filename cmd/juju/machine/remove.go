// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v3/api"
	"github.com/juju/juju/v3/api/client/machinemanager"
	jujucmd "github.com/juju/juju/v3/cmd"
	"github.com/juju/juju/v3/cmd/juju/block"
	"github.com/juju/juju/v3/cmd/modelcmd"
	"github.com/juju/juju/v3/rpc/params"
)

// NewRemoveCommand returns a command used to remove a specified machine.
func NewRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

// removeCommand causes an existing machine to be destroyed.
type removeCommand struct {
	baseMachinesCommand
	apiRoot      api.Connection
	machineAPI   RemoveMachineAPI
	MachineIds   []string
	Force        bool
	KeepInstance bool
	NoWait       bool
	fs           *gnuflag.FlagSet
}

const destroyMachineDoc = `
Machines are specified by their numbers, which may be retrieved from the
output of ` + "`juju status`." + `

It is possible to remove machine from Juju model without affecting
the corresponding cloud instance by using --keep-instance option.

Machines responsible for the model cannot be removed.

Machines running units or containers can be removed using the '--force'
option; this will also remove those units and containers without giving
them an opportunity to shut down cleanly.

Machine removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.

Examples:

    juju remove-machine 5
    juju remove-machine 6 --force
    juju remove-machine 6 --force --no-wait
    juju remove-machine 7 --keep-instance

See also:
    add-machine
`

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-machine",
		Args:    "<machine number> ...",
		Purpose: "Removes one or more machines from a model.",
		Doc:     destroyMachineDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Completely remove a machine and all its dependencies")
	f.BoolVar(&c.KeepInstance, "keep-instance", false, "Do not stop the running cloud instance")
	f.BoolVar(&c.NoWait, "no-wait", false, "Rush through machine removal without waiting for each individual step to complete")
	c.fs = f
}

func (c *removeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no machines specified")
	}
	for _, id := range args {
		if !names.IsValidMachine(id) {
			return errors.Errorf("invalid machine id %q", id)
		}
	}
	c.MachineIds = args
	return nil
}

type RemoveMachineAPI interface {
	DestroyMachinesWithParams(force, keep bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error)
	Close() error
}

func (c *removeCommand) getAPIRoot() (api.Connection, error) {
	if c.apiRoot != nil {
		return c.apiRoot, nil
	}
	return c.NewAPIRoot()
}

func (c *removeCommand) getRemoveMachineAPI() (RemoveMachineAPI, error) {
	if c.machineAPI != nil {
		return c.machineAPI, nil
	}
	root, err := c.getAPIRoot()
	if err != nil {
		return nil, err
	}
	return machinemanager.NewClient(root), nil
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	noWaitSet := false
	forceSet := false
	c.fs.Visit(func(flag *gnuflag.Flag) {
		if flag.Name == "no-wait" {
			noWaitSet = true
		} else if flag.Name == "force" {
			forceSet = true
		}
	})
	if !forceSet && noWaitSet {
		return errors.NotValidf("--no-wait without --force")
	}
	var maxWait *time.Duration
	if c.Force {
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}

	client, err := c.getRemoveMachineAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	results, err := client.DestroyMachinesWithParams(c.Force, c.KeepInstance, maxWait, c.MachineIds...)
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return err
	}

	anyFailed := false
	for _, result := range results {
		err = logRemovedMachine(ctx, result, c.KeepInstance)
		if err != nil {
			anyFailed = true
			ctx.Infof("%s", err)
		}
	}

	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

func logRemovedMachine(ctx *cmd.Context, result params.DestroyMachineResult, keepInstance bool) error {
	if result.Error != nil {
		return errors.Errorf("removing machine failed: %s", result.Error)
	}
	for _, destroyContainerResult := range result.Info.DestroyedContainers {
		err := logRemovedMachine(ctx, destroyContainerResult, keepInstance)
		if err != nil {
			ctx.Infof("%s", err)
		}
		ctx.Infof("\n")
	}
	id := result.Info.MachineId
	if keepInstance {
		ctx.Infof("removing machine %s (but retaining cloud instance)", id)
	} else {
		ctx.Infof("removing machine %s", id)
	}
	for _, entity := range result.Info.DestroyedUnits {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			logger.Warningf("%s", err)
			continue
		}
		ctx.Infof("- will remove %s", names.ReadableString(unitTag))
	}
	for _, entity := range result.Info.DestroyedStorage {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			logger.Warningf("%s", err)
			continue
		}
		ctx.Infof("- will remove %s", names.ReadableString(storageTag))
	}
	for _, entity := range result.Info.DetachedStorage {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			logger.Warningf("%s", err)
			continue
		}
		ctx.Infof("- will detach %s", names.ReadableString(storageTag))
	}
	return nil
}
