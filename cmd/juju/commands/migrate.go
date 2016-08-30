// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
)

func newMigrateCommand() cmd.Command {
	return modelcmd.WrapController(&migrateCommand{})
}

// migrateCommand initiates a model migration.
type migrateCommand struct {
	modelcmd.ControllerCommandBase
	api migrateAPI

	model            string
	targetController string
}

type migrateAPI interface {
	InitiateMigration(spec controller.MigrationSpec) (string, error)
}

const migrateDoc = `
migrate begins the migration of a model from its current controller to
a new controller. This is useful for load balancing when a controller
is too busy, or as a way to upgrade a model's controller to a newer
Juju version. Once complete, the model's machine and and unit agents
will be connected to the new controller. The model will no longer be
available at the source controller.

Note that only hosted models can be migrated. Controller models can
not be migrated.

If the migration fails for some reason, the model be returned to its
original state with the model being managed by the original
controller.

In order to start a migration, the target controller must be in the
juju client's local configuration cache. See the juju "login" command
for details of how to do this.

This command only starts a model migration - it does not wait for its
completion. The progress of a migration can be tracked using the
"status" command and by consulting the logs.

See also:
    login
    controllers
    status
`

// Info implements cmd.Command.
func (c *migrateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "migrate",
		Args:    "<model-name> <target-controller-name>",
		Purpose: "Migrate a hosted model to another controller.",
		Doc:     migrateDoc,
	}
}

// Init implements cmd.Command.
func (c *migrateCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("model not specified")
	}
	if len(args) < 2 {
		return errors.New("target controller not specified")
	}
	if len(args) > 2 {
		return errors.New("too many arguments specified")
	}

	c.model = args[0]
	c.targetController = args[1]
	return nil
}

func (c *migrateCommand) getMigrationSpec() (*controller.MigrationSpec, error) {
	store := c.ClientStore()

	modelUUIDs, err := c.ModelUUIDs([]string{c.model})
	if err != nil {
		return nil, err
	}
	modelUUID := modelUUIDs[0]

	controllerInfo, err := store.ControllerByName(c.targetController)
	if err != nil {
		return nil, err
	}

	accountInfo, err := store.AccountDetails(c.targetController)
	if err != nil {
		return nil, err
	}

	return &controller.MigrationSpec{
		ModelUUID:            modelUUID,
		TargetControllerUUID: controllerInfo.ControllerUUID,
		TargetAddrs:          controllerInfo.APIEndpoints,
		TargetCACert:         controllerInfo.CACert,
		TargetUser:           accountInfo.User,
		TargetPassword:       accountInfo.Password,
		TargetMacaroon:       accountInfo.Macaroon,
	}, nil
}

// Run implements cmd.Command.
func (c *migrateCommand) Run(ctx *cmd.Context) error {
	spec, err := c.getMigrationSpec()
	if err != nil {
		return err
	}
	api, err := c.getAPI()
	if err != nil {
		return err
	}
	id, err := api.InitiateMigration(*spec)
	if err != nil {
		return err
	}
	ctx.Infof("Migration started with ID %q", id)
	return nil
}

func (c *migrateCommand) getAPI() (migrateAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewControllerAPIClient()
}
