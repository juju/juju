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
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveUnitCommand returns a command which removes an application's units.
func NewRemoveUnitCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&removeUnitCommand{})
}

// removeUnitCommand is responsible for destroying application units.
type removeUnitCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand
	DestroyStorage bool
	UnitNames      []string
	api            removeApplicationAPI
}

const removeUnitDoc = `
Remove application units from the model.

Units of a application are numbered in sequence upon creation. For example, the
fourth unit of wordpress will be designated "wordpress/3". These identifiers
can be supplied in a space delimited list to remove unwanted units from the
model.

Juju will also remove the machine if the removed unit was the only unit left
on that machine (including units in containers).

Removing all units of a application is not equivalent to removing the
application itself; for that, the ` + "`juju remove-application`" + ` command
is used.

Examples:

    juju remove-unit wordpress/2 wordpress/3 wordpress/4

See also:
    remove-application
`

func (c *removeUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-unit",
		Args:    "<unit> [...]",
		Purpose: "Remove application units from the model.",
		Doc:     removeUnitDoc,
	}
}

// IncompatibleModel returns an error if the command is being run against
// a model with which it is not compatible.
func (c *removeUnitCommand) IncompatibleModel(err error) error {
	if err == nil {
		return nil
	}
	msg := `
remove-unit is not allowed on Kubernetes models.
Instead, use juju scale-application.
See juju help scale-application.
`[1:]
	return errors.New(msg)
}

func (c *removeUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.DestroyStorage, "destroy-storage", false, "Destroy storage attached to the unit")
}

func (c *removeUnitCommand) Init(args []string) error {
	c.UnitNames = args
	if len(c.UnitNames) == 0 {
		return errors.Errorf("no units specified")
	}
	for _, name := range c.UnitNames {
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
	if c.DestroyStorage && apiVersion < 5 {
		return errors.New("--destroy-storage is not supported by this controller")
	}
	return c.removeUnits(ctx, client)
}

// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *removeUnitCommand) removeUnitsDeprecated(ctx *cmd.Context, client removeApplicationAPI) error {
	err := client.DestroyUnitsDeprecated(c.UnitNames...)
	return block.ProcessBlockedError(err, block.BlockRemove)
}

func (c *removeUnitCommand) removeUnits(ctx *cmd.Context, client removeApplicationAPI) error {
	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units:          c.UnitNames,
		DestroyStorage: c.DestroyStorage,
	})
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockRemove)
	}
	anyFailed := false
	for i, name := range c.UnitNames {
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
