// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// NewRemoveUnitCommand returns a command which removes an application's units.
func NewRemoveUnitCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&removeUnitCommand{})
}

// removeUnitCommand is responsible for destroying application units.
type removeUnitCommand struct {
	modelcmd.ModelCommandBase
	DestroyStorage bool
	NumUnits       int
	EntityNames    []string
	api            RemoveApplicationAPI

	unknownModel bool
	Force        bool
	NoWait       bool
	fs           *gnuflag.FlagSet
}

const removeUnitDoc = `
Remove application units from the model.

The usage of this command differs depending on whether it is being used on a
k8s or cloud model.

Removing all units of a application is not equivalent to removing the
application itself; for that, the ` + "`juju remove-application`" + ` command
is used.

For k8s models only a single application can be supplied and only the
--num-units argument supported.
Specific units cannot be targeted for removal as that is handled by k8s,
instead the total number of units to be removed is specified.

Examples:
    juju remove-unit wordpress --num-units 2

For cloud models specific units can be targeted for removal.
Units of a application are numbered in sequence upon creation. For example, the
fourth unit of wordpress will be designated "wordpress/3". These identifiers
can be supplied in a space delimited list to remove unwanted units from the
model.

Juju will also remove the machine if the removed unit was the only unit left
on that machine (including units in containers).

Sometimes, the removal of the unit may fail as Juju encounters errors
and failures that need to be dealt with before a unit can be removed.
For example, Juju will not remove a unit if there are hook failures.
However, at times, there is a need to remove a unit ignoring
all operational errors. In these rare cases, use --force option but note
that --force will remove a unit and, potentially, its machine without
given them the opportunity to shutdown cleanly.

Unit removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using --force, users can also specify --no-wait to progress through steps
without delay waiting for each step to complete.

Examples:

    juju remove-unit wordpress/2 wordpress/3 wordpress/4

    juju remove-unit wordpress/2 --destroy-storage

    juju remove-unit wordpress/2 --force

    juju remove-unit wordpress/2 --force --no-wait

See also:
    remove-application
    scale-application
`

func (c *removeUnitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-unit",
		Args:    "<unit> [...] | <application>",
		Purpose: "Remove application units from the model.",
		Doc:     removeUnitDoc,
	})
}

func (c *removeUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	// This unused var is declared so we can pass a valid ptr into BoolVar
	var noPromptHolder bool
	f.BoolVar(&noPromptHolder, "no-prompt", false, "Does nothing. Option present for forward compatibility with Juju 3")
	f.IntVar(&c.NumUnits, "num-units", 0, "Number of units to remove (k8s models only)")
	f.BoolVar(&c.DestroyStorage, "destroy-storage", false, "Destroy storage attached to the unit")
	f.BoolVar(&c.Force, "force", false, "Completely remove an unit and all its dependencies")
	f.BoolVar(&c.NoWait, "no-wait", false, "Rush through unit removal without waiting for each individual step to complete")
	c.fs = f
}

func (c *removeUnitCommand) Init(args []string) error {
	c.EntityNames = args
	if err := c.validateArgsByModelType(); err != nil {
		if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}

		c.unknownModel = true
	}
	return nil
}

func (c *removeUnitCommand) validateArgsByModelType() error {
	modelType, err := c.ModelType()
	if err != nil {
		return err
	}
	if modelType == model.CAAS {
		return c.validateCAASRemoval()
	}

	return c.validateIAASRemoval()
}

func (c *removeUnitCommand) validateCAASRemoval() error {
	if c.DestroyStorage {
		// TODO(caas): enable --destroy-storage for caas model.
		return errors.New("k8s models only support --num-units")
	}
	if len(c.EntityNames) == 0 {
		return errors.Errorf("no application specified")
	}
	if len(c.EntityNames) != 1 {
		return errors.Errorf("only single application supported")
	}
	if names.IsValidUnit(c.EntityNames[0]) {
		msg := `
k8s models do not support removing named units.
Instead specify an application with --num-units.
`[1:]
		return errors.New(msg)
	}
	if !names.IsValidApplication(c.EntityNames[0]) {
		return errors.NotValidf("application name %q", c.EntityNames[0])
	}
	if c.NumUnits <= 0 {
		return errors.New("specify the number of units (> 0) to remove using --num-units")
	}
	return nil
}

func (c *removeUnitCommand) validateIAASRemoval() error {
	if c.NumUnits != 0 {
		return errors.NotValidf("--num-units for non k8s models")
	}
	if len(c.EntityNames) == 0 {
		return errors.Errorf("no units specified")
	}
	for _, name := range c.EntityNames {
		if !names.IsValidUnit(name) {
			return errors.Errorf("invalid unit name %q", name)
		}
	}

	return nil
}

func (c *removeUnitCommand) getAPI() (RemoveApplicationAPI, int, error) {
	if c.api != nil {
		return c.api, c.api.BestAPIVersion(), nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	api := application.NewClient(root)
	return api, api.BestAPIVersion(), nil
}

// Run connects to the environment specified on the command line and destroys
// units therein.
func (c *removeUnitCommand) Run(ctx *cmd.Context) error {
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

	client, apiVersion, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	if apiVersion < 4 {
		return c.removeUnitsDeprecated(ctx, client)
	}

	if err := c.validateArgsByModelType(); err != nil {
		return errors.Trace(err)
	}

	modelType, err := c.ModelType()
	if err != nil {
		return err
	}
	if modelType == model.CAAS {
		return c.removeCaasUnits(ctx, client)
	}

	if c.DestroyStorage && apiVersion < 5 {
		return errors.New("--destroy-storage is not supported by this controller")
	}
	return c.removeUnits(ctx, client)
}

// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *removeUnitCommand) removeUnitsDeprecated(ctx *cmd.Context, client RemoveApplicationAPI) error {
	err := client.DestroyUnitsDeprecated(c.EntityNames...)
	return block.ProcessBlockedError(err, block.BlockRemove)
}

func (c *removeUnitCommand) removeUnits(ctx *cmd.Context, client RemoveApplicationAPI) error {
	var maxWait *time.Duration
	if c.Force {
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}

	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units:          c.EntityNames,
		DestroyStorage: c.DestroyStorage,
		Force:          c.Force,
		MaxWait:        maxWait,
	})
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockRemove)
	}
	anyFailed := false
	for i, name := range c.EntityNames {
		result := results[i]
		if result.Error != nil {
			anyFailed = true
			ctx.Infof("removing unit %s failed: %s", name, result.Error)
			continue
		}
		ctx.Infof("removing unit %s", name)
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

func (c *removeUnitCommand) removeCaasUnits(ctx *cmd.Context, client RemoveApplicationAPI) error {
	result, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: c.EntityNames[0],
		ScaleChange:     -c.NumUnits,
		Force:           c.Force,
	})
	if params.IsCodeNotSupported(err) {
		return errors.Annotate(err, "can not remove unit")
	}
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockRemove)
	}
	ctx.Infof("scaling down to %d units", result.Info.Scale)
	return nil
}
