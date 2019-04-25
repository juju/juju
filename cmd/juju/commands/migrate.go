// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

func newMigrateCommand() modelcmd.ModelCommand {
	var cmd migrateCommand
	cmd.newAPIRoot = cmd.CommandBase.NewAPIRoot
	return modelcmd.Wrap(&cmd, modelcmd.WrapSkipModelFlags)
}

// migrateCommand initiates a model migration.
type migrateCommand struct {
	modelcmd.ModelCommandBase
	targetController string

	// Overridden by tests
	newAPIRoot func(jujuclient.ClientStore, string, string) (api.Connection, error)
	migAPI     migrateAPI
	modelAPI   modelInfoAPI
	userAPI    userListAPI
}

type migrateAPI interface {
	InitiateMigration(spec controller.MigrationSpec) (string, error)
	Close() error
}

type modelInfoAPI interface {
	ModelInfo([]names.ModelTag) ([]params.ModelInfoResult, error)
	Close() error
}

type userListAPI interface {
	UserInfo([]string, usermanager.IncludeDisabled) ([]params.UserInfo, error)
	Close() error
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
	return jujucmd.Info(&cmd.Info{
		Name:    "migrate",
		Args:    "<model-name> <target-controller-name>",
		Purpose: "Migrate a hosted model to another controller.",
		Doc:     migrateDoc,
	})
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

	c.SetModelName(args[0], false)
	c.targetController = args[1]
	return nil
}

// Run implements cmd.Command.
func (c *migrateCommand) Run(ctx *cmd.Context) error {
	spec, err := c.getMigrationSpec()
	if err != nil {
		return err
	}
	modelName, err := c.ModelName()
	if err != nil {
		return errors.Trace(err)
	}
	uuids, err := c.ModelUUIDs([]string{modelName})
	if err != nil {
		return errors.Trace(err)
	}
	spec.ModelUUID = uuids[0]
	if err := c.checkMigrationFeasibility(spec); err != nil {
		return err
	}
	api, err := c.getMigrationAPI()
	if err != nil {
		return err
	}
	defer func() { _ = api.Close() }()
	id, err := api.InitiateMigration(*spec)
	if err != nil {
		return err
	}
	ctx.Infof("Migration started with ID %q", id)
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

func (c *migrateCommand) getMigrationAPI() (migrateAPI, error) {
	if c.migAPI != nil {
		return c.migAPI, nil
	}

	apiRoot, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return controller.NewClient(apiRoot), nil
}

func (c *migrateCommand) getModelAPI() (modelInfoAPI, error) {
	if c.modelAPI != nil {
		return c.modelAPI, nil
	}

	controllerName, err := c.ControllerName()
	if err != nil {
		return nil, err
	}

	apiRoot, err := c.newAPIRoot(c.ClientStore(), controllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(apiRoot), nil
}

func (c *migrateCommand) getTargetControllerUserAPI() (userListAPI, error) {
	if c.userAPI != nil {
		return c.userAPI, nil
	}

	apiRoot, err := c.newAPIRoot(c.ClientStore(), c.targetController, "")
	if err != nil {
		return nil, errors.Annotate(err, "connecting to target controller")
	}
	return usermanager.NewClient(apiRoot), nil
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

func (c *migrateCommand) checkMigrationFeasibility(spec *controller.MigrationSpec) error {
	var (
		srcControllerName, srcModelName string
		srcUsers, dstUsers              set.Strings
		err                             error
	)

	if srcControllerName, err = c.ControllerName(); err != nil {
		return err
	}
	if srcModelName, err = c.ModelName(); err != nil {
		return err
	}
	if srcUsers, err = c.getModelUsers(names.NewModelTag(spec.ModelUUID)); err != nil {
		return err
	}
	if dstUsers, err = c.getTargetControllerUsers(); err != nil {
		return err
	}

	if missing := srcUsers.Difference(dstUsers); missing.Size() != 0 {
		return errors.Errorf(`cannot initiate migration of model "%s:%s" to controller %q as some of the
model's users do not exist in the target controller. To resolve this issue you can
either migrate the following list of users to %q or remove them from "%s:%s":
  - %s`,
			srcControllerName, srcModelName, c.targetController,
			c.targetController, srcControllerName, srcModelName,
			strings.Join(missing.Values(), "\n  - "),
		)
	}

	return nil
}

func (c *migrateCommand) getModelUsers(modelTag names.ModelTag) (set.Strings, error) {
	api, err := c.getModelAPI()
	if err != nil {
		return nil, err
	}
	defer api.Close()

	infoRes, err := api.ModelInfo([]names.ModelTag{modelTag})
	if err != nil {
		return nil, err
	}

	if infoRes[0].Error != nil {
		return nil, infoRes[0].Error
	}

	users := set.NewStrings()
	for _, user := range infoRes[0].Result.Users {
		users.Add(user.UserName)
	}
	return users, nil
}

func (c *migrateCommand) getTargetControllerUsers() (set.Strings, error) {
	api, err := c.getTargetControllerUserAPI()
	if err != nil {
		return nil, err
	}
	defer api.Close()

	userInfo, err := api.UserInfo(nil, usermanager.AllUsers)
	if err != nil {
		return nil, errors.Annotate(err, "looking up model users in target controller")
	}

	users := set.NewStrings()
	for _, user := range userInfo {
		users.Add(user.Username)
	}

	return users, nil
}
