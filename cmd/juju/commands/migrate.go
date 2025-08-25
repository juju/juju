// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/modelmanager"
	"github.com/juju/juju/api/client/usermanager"
	"github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

func newMigrateCommand() modelcmd.ModelCommand {
	var cmd migrateCommand
	cmd.newAPIRoot = cmd.CommandBase.NewAPIRoot
	return modelcmd.Wrap(&cmd,
		modelcmd.WrapSkipModelFlags,
	)
}

// migrateCommand initiates a model migration.
type migrateCommand struct {
	modelcmd.ModelCommandBase
	targetController string

	// Overridden by tests
	newAPIRoot func(jujuclient.ClientStore, string, string) (api.Connection, error)
	migAPI     map[string]migrateAPI
	modelAPI   modelInfoAPI
	userAPI    userListAPI
}

type migrateAPI interface {
	InitiateMigration(spec controller.MigrationSpec) (string, error)
	IdentityProviderURL() (string, error)
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
The ` + "`juju migrate`" + ` command begins the migration of a workload model from
its current controller to a new controller. This is useful for load
balancing when a controller is too busy, or as a way to upgrade a
model's controller to a newer Juju version.

In order to start a migration, the target controller must be in the
` + "`juju`" + ` client's local configuration cache. See the ` + "`login`" + ` command
for details of how to do this.

The ` + "`migrate`" + ` command only starts a model migration -- it does not wait
for its completion. The progress of a migration can be tracked using
the ` + "`status`" + ` command and by consulting the logs.

Once the migration is complete, the model's machine and unit agents
will be connected to the new controller. The model will no longer be
available at the source controller.

If the migration fails for some reason, the model is returned to its
original state where it is managed by the original
controller.

`

// Info implements cmd.Command.
func (c *migrateCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "migrate",
		Args:    "<model-name> <target-controller-name>",
		Purpose: "Migrate a workload model to another controller.",
		Doc:     migrateDoc,
		SeeAlso: []string{
			"login",
			"controllers",
			"status",
		},
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

	if err := c.SetModelIdentifier(args[0], false); err != nil {
		return errors.Trace(err)
	}

	c.targetController = args[1]
	return nil
}

// Run implements cmd.Command.
func (c *migrateCommand) Run(ctx *cmd.Context) error {
	spec, err := c.getMigrationSpec()
	if err != nil {
		return err
	}
	modelName, err := c.ModelIdentifier()
	if err != nil {
		return errors.Trace(err)
	}
	uuids, err := c.ModelUUIDs([]string{modelName})
	if err != nil {
		return errors.Trace(err)
	}
	spec.ModelUUID = uuids[0]
	if err := c.checkMigrationFeasibility(spec); err != nil {
		return errors.Trace(err)
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return err
	}
	api, err := c.getMigrationAPI(controllerName)
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
		TargetControllerUUID:  controllerInfo.ControllerUUID,
		TargetControllerAlias: c.targetController,
		TargetAddrs:           controllerInfo.APIEndpoints,
		TargetCACert:          controllerInfo.CACert,
		TargetUser:            accountInfo.User,
		TargetPassword:        accountInfo.Password,
		TargetMacaroons:       macs,
	}, nil
}

func (c *migrateCommand) getMigrationAPI(controllerName string) (migrateAPI, error) {
	if c.migAPI != nil && c.migAPI[controllerName] != nil {
		return c.migAPI[controllerName], nil
	}

	apiRoot, err := c.newAPIRoot(c.ClientStore(), controllerName, "")
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
		srcUsers, dstUsers set.Strings
		srcControllerName  string
		err                error
	)

	if srcUsers, err = c.getModelUsers(names.NewModelTag(spec.ModelUUID)); err != nil {
		return err
	}
	if dstUsers, err = c.getTargetControllerUsers(); err != nil {
		return err
	}

	// If external users have access to this model we can only allow the
	// migration to proceed if:
	// - the local users from src exist in the dst, and
	// - both controllers are configured with the same identity provider URL
	srcExtUsers := filterSet(srcUsers, func(u string) bool {
		return strings.Contains(u, "@")
	})

	if srcExtUsers.Size() != 0 {
		if srcControllerName, err = c.ControllerName(); err != nil {
			return err
		}
		srcIdentityURL, err := c.getIdentityProviderURL(srcControllerName)
		if err != nil {
			return errors.Annotate(err, "looking up source controller identity provider URL")
		}

		dstIdentityURL, err := c.getIdentityProviderURL(c.targetController)
		if err != nil {
			return errors.Annotate(err, "looking up target controller identity provider URL")
		}

		localSrcUsers := srcUsers.Difference(srcExtUsers)

		// In this case external user lookups will most likely not work.
		// Display an appropriate error message depending on whether
		// the local users are present in dst or not.
		if srcIdentityURL != dstIdentityURL {
			missing := localSrcUsers.Difference(dstUsers)
			if missing.Size() == 0 {
				return errors.Errorf(`cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you can remove the following users from the current model:
  - %s`, strings.Join(srcExtUsers.Values(), "\n  - "))
			}

			return errors.Errorf(`cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you need to remove the following users from the current model:
  - %s

and add the following users to the destination controller or remove them from
the current model:
  - %s`,
				strings.Join(srcExtUsers.Values(), "\n  - "),
				strings.Join(localSrcUsers.Difference(dstUsers).Values(), "\n  - "),
			)
		}

		// External user lookups will work out of the box. We only need
		// to ensure that the local model users are present in dst
		srcUsers = localSrcUsers
	}

	if missing := srcUsers.Difference(dstUsers); missing.Size() != 0 {
		return errors.Errorf(`cannot initiate migration as the users granted access to the model do not exist
on the destination controller. To resolve this issue you can add the following
users to the destination controller or remove them from the current model:
  - %s`, strings.Join(missing.Values(), "\n  - "))
	}

	return nil
}

func (c *migrateCommand) getIdentityProviderURL(controllerName string) (string, error) {
	api, err := c.getMigrationAPI(controllerName)
	if err != nil {
		return "", err
	}
	defer api.Close()

	return api.IdentityProviderURL()
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

func filterSet(s set.Strings, keep func(string) bool) set.Strings {
	out := set.NewStrings()
	for _, v := range s.Values() {
		if keep(v) {
			out.Add(v)
		}
	}

	return out
}
