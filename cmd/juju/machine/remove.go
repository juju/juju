// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/instance"
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
	Placement    *instance.Placement
}

const destroyMachineDoc = `
Machines are specified by their numbers, which may be retrieved from the
output of ` + "`juju status`." + `

It is possible to remove machine from Juju model without affecting
the corresponding cloud instnace by using --keep-instance option.

Machines responsible for the model cannot be removed.

Machines running units or containers can be removed using the '--force'
option; this will also remove those units and containers without giving
them an opportunity to shut down cleanly.

Machine removal is a multi-step process. Under normal circumstances, Juju will not
proceed to a next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.

Removing manual provisioned machines can be cleaned up as long as they're still
accessible via SSH. Clean up of the machine removes the Juju services along with
cleaning up of lib and log directories.

Examples:

    juju remove-machine 5
    juju remove-machine 6 --force
    juju remove-machine 6 --force --no-wait
    juju remove-machine 7 --keep-instance
    juju remove-machine ssh:user@10.10.0.3
    juju remove-machine winrm:user@10.10.0.3

See also:
    add-machine
`

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-machine",
		Args:    "[<machine number> ... | ssh:[user@]host | winrm:[user@]host]",
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
	// attempt to first parse the placement, if that fails, fall back to the
	// original validation against machine ids.
	// we always set the machineIds if we've gone through parsing a placment,
	// as we'll need it when running the command later on and we'll assume that
	// the machine id is valid if it's been parsed.
	if len(args) == 1 {
		if placement, err := instance.ParsePlacement(args[0]); err == nil {
			if placement.Scope == instance.MachineScope && !names.IsValidMachine(args[0]) {
				return errors.Errorf("invalid machine id %q", args[0])
			}
			c.Placement = placement
			c.MachineIds = args
			return nil
		}
	}
	// placement syntax failed, fall back to machine validation
	for _, id := range args {
		if !names.IsValidMachine(id) {
			return errors.Errorf("invalid machine id %q", id)
		}
	}
	c.MachineIds = args
	return nil
}

type RemoveMachineAPI interface {
	// TODO (anastasiamac 2019-4-24) From Juju 3.0 this call will be removed in favour of DestroyMachinesWithParams.
	DestroyMachines(machines ...string) ([]params.DestroyMachineResult, error)
	DestroyMachinesWithParams(force, keep bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error)
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

func (a removeMachineAdapter) DestroyMachinesWithParams(force, keep bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error) {
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

func (c *removeCommand) getAPIRoot() (api.Connection, error) {
	if c.apiRoot != nil {
		return c.apiRoot, nil
	}
	return c.NewAPIRoot()
}

func (c *removeCommand) getRemoveMachineAPI() (RemoveMachineAPI, error) {
	root, err := c.getAPIRoot()
	if err != nil {
		return nil, err
	}
	if root.BestFacadeVersion("MachineManager") < 4 && c.KeepInstance {
		return nil, errors.New("this version of Juju doesn't support --keep-instance")
	}
	if root.BestFacadeVersion("MachineManager") >= 3 && c.machineAPI == nil {
		return machinemanager.NewClient(root), nil
	}
	if c.machineAPI != nil {
		return c.machineAPI, nil
	}
	return removeMachineAdapter{root.Client()}, nil
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

	if c.Placement != nil {
		if err := c.tryManualRemoval(); err != errNonManualScope {
			return err
		}
	}

	client, err := c.getRemoveMachineAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	var results []params.DestroyMachineResult

	if c.KeepInstance || c.Force {
		results, err = client.DestroyMachinesWithParams(c.Force, c.KeepInstance, maxWait, c.MachineIds...)
	} else {
		results, err = client.DestroyMachines(c.MachineIds...)
	}
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
		if c.KeepInstance {
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
	}

	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

func (c *removeCommand) tryManualRemoval() error {
	switch c.Placement.Scope {
	case sshScope:
	case winrmScope:
	default:
		return errNonManualScope
	}
	return nil
}
