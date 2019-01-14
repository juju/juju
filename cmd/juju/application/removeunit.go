// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/storage"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
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
	api            removeApplicationAPI

	unknownModel bool
}

const removeUnitDoc = `
Remove application units from the model.

The usage of this command differs depending on whether it is being used on a
Kubernetes or cloud model.

Removing all units of a application is not equivalent to removing the
application itself; for that, the ` + "`juju remove-application`" + ` command
is used.

For Kubernetes models only a single application can be supplied and only the
--num-units argument supported.
Specific units cannot be targeted for removal as that is handled by Kubernetes,
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

Examples:

    juju remove-unit wordpress/2 wordpress/3 wordpress/4

    juju remove-unit wordpress/2 --destroy-storage

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
	f.IntVar(&c.NumUnits, "num-units", 0, "Number of units to remove (kubernetes models only)")
	f.BoolVar(&c.DestroyStorage, "destroy-storage", false, "Destroy storage attached to the unit")
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
		return errors.New("Kubernetes models only support --num-units")
	}
	if len(c.EntityNames) != 1 {
		return errors.Errorf("only single application supported")
	}
	if !names.IsValidApplication(c.EntityNames[0]) {
		return errors.NotValidf("application name %q", c.EntityNames[0])
	}
	if c.NumUnits <= 0 {
		return errors.NotValidf("removing %d units", c.NumUnits)
	}

	return nil
}

func (c *removeUnitCommand) validateIAASRemoval() error {
	if c.NumUnits != 0 {
		return errors.NotValidf("--num-units for non kubernetes models")
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

func (c *removeUnitCommand) getAPI() (removeApplicationAPI, int, error) {
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

func (c *removeUnitCommand) getStorageAPI() (storageAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return storage.NewClient(root), nil
}

func (c *removeUnitCommand) unitsHaveStorage(unitNames []string) (bool, error) {
	client, err := c.getStorageAPI()
	if err != nil {
		return false, errors.Trace(err)
	}
	defer client.Close()

	storage, err := client.ListStorageDetails()
	if err != nil {
		return false, errors.Trace(err)
	}
	namesSet := set.NewStrings(unitNames...)
	for _, s := range storage {
		if s.OwnerTag == "" {
			continue
		}
		owner, err := names.ParseTag(s.OwnerTag)
		if err != nil {
			return false, errors.Trace(err)
		}
		if owner.Kind() == names.UnitTagKind && namesSet.Contains(owner.Id()) {
			return true, nil
		}
	}
	return false, nil
}

// Run connects to the environment specified on the command line and destroys
// units therein.
func (c *removeUnitCommand) Run(ctx *cmd.Context) error {
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
func (c *removeUnitCommand) removeUnitsDeprecated(ctx *cmd.Context, client removeApplicationAPI) error {
	err := client.DestroyUnitsDeprecated(c.EntityNames...)
	return block.ProcessBlockedError(err, block.BlockRemove)
}

func (c *removeUnitCommand) removeUnits(ctx *cmd.Context, client removeApplicationAPI) error {
	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units:          c.EntityNames,
		DestroyStorage: c.DestroyStorage,
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

func (c *removeUnitCommand) removeCaasUnits(ctx *cmd.Context, client removeApplicationAPI) error {
	result, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: c.EntityNames[0],
		ScaleChange:     -c.NumUnits,
	})
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockRemove)
	}
	ctx.Infof("scaling down to %d units", result.Info.Scale)
	return nil
}
