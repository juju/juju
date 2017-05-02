// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func newMigrateCommand() cmd.Command {
	var cmd migrateCommand
	cmd.newAPIRoot = cmd.CommandBase.NewAPIRoot
	return modelcmd.WrapController(&cmd)
}

// migrateCommand initiates a model migration.
type migrateCommand struct {
	modelcmd.ControllerCommandBase
	newAPIRoot       func(jujuclient.ClientStore, string, string) (api.Connection, error)
	api              migrateAPI
	model            string
	targetController string
}

type migrateAPI interface {
	AllModels() ([]base.UserModel, error)
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

	controllerInfo, err := store.ControllerByName(c.targetController)
	if err != nil {
		return nil, err
	}

	accountInfo, err := store.AccountDetails(c.targetController)
	if err != nil {
		return nil, err
	}

	var macs []macaroon.Slice
	if accountInfo.Password == "" {
		var err error
		macs, err = c.getTargetControllerMacaroons()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return &controller.MigrationSpec{
		TargetControllerUUID: controllerInfo.ControllerUUID,
		TargetAddrs:          controllerInfo.APIEndpoints,
		TargetCACert:         controllerInfo.CACert,
		TargetUser:           accountInfo.User,
		TargetPassword:       accountInfo.Password,
		TargetMacaroons:      macs,
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
	spec.ModelUUID, err = c.findModelUUID(ctx, api)
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

func (c *migrateCommand) findModelUUID(ctx *cmd.Context, api migrateAPI) (string, error) {
	models, err := api.AllModels()
	if err != nil {
		return "", errors.Trace(err)
	}
	// Look for the uuid based on name. If the model name doesn't container a
	// slash, then only accept the model name if there exists only one model
	// with that name.
	owner := ""
	name := c.model
	if strings.Contains(name, "/") {
		values := strings.SplitN(name, "/", 2)
		owner = values[0]
		name = values[1]
	}
	var matches []base.UserModel
	for _, model := range models {
		if model.Name == name && (owner == "" || model.Owner == owner) {
			matches = append(matches, model)
		}
	}
	switch len(matches) {
	case 0:
		return "", errors.NotFoundf("model matching %q", c.model)
	case 1:
		return matches[0].UUID, nil
	default:
		ctx.Infof("Multiple potential matches found, please specify owner to disambiguate:")
		for _, match := range matches {
			ctx.Infof("  %s/%s", match.Owner, match.Name)
		}
		return "", errors.New("multiple models match name")
	}
}

func (c *migrateCommand) getAPI() (migrateAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewControllerAPIClient()
}

func (c *migrateCommand) getTargetControllerMacaroons() ([]macaroon.Slice, error) {
	jar, err := c.CommandBase.CookieJar(c.ClientStore(), c.targetController)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Connect to the target controller, ensuring up-to-date macaroons,
	// and return the macaroons in the cookie jar for the controller.
	//
	// TODO(axw,mjs) add a controller API that returns a macaroon that
	// may be used for the sole purpose of migration.
	api, err := c.newAPIRoot(c.ClientStore(), c.targetController, "")
	if err != nil {
		return nil, errors.Annotate(err, "connecting to target controller")
	}
	defer api.Close()
	return httpbakery.MacaroonsForURL(jar, api.CookieURL()), nil
}
