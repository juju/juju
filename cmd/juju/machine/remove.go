// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveCommand returns a command used to remove a specified machine.
func NewRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

// removeCommand causes an existing machine to be destroyed.
type removeCommand struct {
	modelcmd.ModelCommandBase
	api        RemoveMachineAPI
	MachineIds []string
	Force      bool
}

const destroyMachineDoc = `
Machines are specified by their numbers, which may be retrieved from the
output of ` + "`juju status`." + `
Machines responsible for the model cannot be removed.
Machines running units or containers can be removed using the '--force'
option; this will also remove those units and containers without giving
them an opportunity to shut down cleanly.

Examples:

Remove machine number 5 which has no running units or containers:

    juju remove-machine 5

Remove machine 6 and any running units or containers:

    juju remove-machine 6 --force

See also:
    add-machine
`

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-machine",
		Args:    "<machine number> ...",
		Purpose: "Removes one or more machines from a model.",
		Doc:     destroyMachineDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.Force, "force", false, "Completely remove a machine and all its dependencies")
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
	DestroyMachines(machines ...string) ([]params.DestroyMachineResult, error)
	ForceDestroyMachines(machines ...string) ([]params.DestroyMachineResult, error)
	Close() error
}

// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
type removeMachineAdapter struct {
	*api.Client
}

func (a removeMachineAdapter) DestroyMachines(machines ...string) ([]params.DestroyMachineResult, error) {
	return a.destroyMachines(a.Client.DestroyMachines, machines)
}

func (a removeMachineAdapter) ForceDestroyMachines(machines ...string) ([]params.DestroyMachineResult, error) {
	return a.destroyMachines(a.Client.ForceDestroyMachines, machines)
}

func (a removeMachineAdapter) destroyMachines(f func(...string) error, machines []string) ([]params.DestroyMachineResult, error) {
	if err := f(machines...); err != nil {
		return nil, err
	}
	results := make([]params.DestroyMachineResult, len(machines))
	for i := range results {
		results[i].Info = &params.DestroyMachineInfo{}
	}
	return results, nil
}

func (c *removeCommand) getRemoveMachineAPI() (RemoveMachineAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	if root.BestFacadeVersion("MachineManager") >= 3 {
		return machinemanager.NewClient(root), nil
	}
	return removeMachineAdapter{root.Client()}, nil
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	client, err := c.getRemoveMachineAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	destroy := client.DestroyMachines
	if c.Force {
		destroy = client.ForceDestroyMachines
	}

	results, err := destroy(c.MachineIds...)
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return err
	}

	anyFailed := false
	for i, id := range c.MachineIds {
		result := results[i]
		if result.Error != nil {
			anyFailed = true
			ctx.Infof("removing machine %s failed: %s", id, result.Error)
			continue
		}
		ctx.Infof("removing machine %s", id)
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
	}

	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}
