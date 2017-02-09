// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/keyvalues"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	apibase "github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

const loginDoc = `
When a known domain is specified instead of a username or the
--host flag is specified the user is logged into
the controller associated with the given domain and the controller is registered
using the domain name (the -c flag can be used to choose a different controller
name). If a different controller is already registered with the same name,
it is an error.

After login, a token ("macaroon") will become active. It has an expiration
time of 24 hours. Upon expiration, no further Juju commands can be issued
and the user will be prompted to log in again.

Examples:

    juju login bob

See also:
    disable-user
    enable-user
    logout

Currently, the JUJU_PUBLIC_CONTROLLERS environment variable
is used to set the currently known public controllers.
For example:

	export JUJU_PUBLIC_CONTROLLERS="foo=foo.com"

will cause juju login to interpret "juju login foo" the same
as "juju login --host foo.com".
`

// Functions defined as variables so they can be overridden in tests.
var (
	apiOpen          = (*modelcmd.JujuCommandBase).APIOpen
	newAPIConnection = juju.NewAPIConnection
	listModels       = func(c *modelcmd.ControllerCommandBase, userName string) ([]apibase.UserModel, error) {
		api, err := c.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer api.Close()
		mm := modelmanager.NewClient(api)
		return mm.ListModels(userName)
	}
	// loginClientStore is used as the client store. When it is nil,
	// the default client store will be used.
	loginClientStore jujuclient.ClientStore
)

// NewLoginCommand returns a new cmd.Command to handle "juju login".
func NewLoginCommand() cmd.Command {
	var c loginCommand
	c.SetClientStore(loginClientStore)
	return modelcmd.WrapController(&c, modelcmd.WrapControllerSkipControllerFlags)
}

// loginCommand changes the password for a user.
type loginCommand struct {
	modelcmd.ControllerCommandBase
	userOrDomain string
	forceHost    bool

	// controllerName holds the name of the current controller.
	// We define this and the --controller flag here because
	// the controller does not necessarily exist when the command
	// is executed.
	controllerName string

	// onRunError is executed if non-nil if there is an error at the end
	// of the Run method.
	onRunError func()
}

// Info implements Command.Info.
func (c *loginCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "login",
		Args:    "[username|domain]",
		Purpose: "Logs a user in to a controller.",
		Doc:     loginDoc,
	}
}

func (c *loginCommand) SetFlags(fset *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(fset)
	fset.StringVar(&c.controllerName, "c", "", "Controller to operate in")
	fset.StringVar(&c.controllerName, "controller", "", "")
	fset.BoolVar(&c.forceHost, "host", false, "force the domain argument to be treated as the host name of a controller")
}

// Init implements Command.Init.
func (c *loginCommand) Init(args []string) error {
	var err error
	c.userOrDomain, err = cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run implements Command.Run.
func (c *loginCommand) Run(ctx *cmd.Context) error {
	err := c.run(ctx)
	if err != nil && c.onRunError != nil {
		c.onRunError()
	}
	return err
}

var errNotControllerLogin = errors.New("not a controller login")

func (c *loginCommand) run(ctx *cmd.Context) error {
	store := c.ClientStore()
	err := c.controllerLogin(ctx, store)
	if errors.Cause(err) != errNotControllerLogin {
		return errors.Trace(err)
	}
	user := c.userOrDomain
	// Set the controller name as if this is a normal controller command.
	if err := c.SetControllerName(c.controllerName, true); err != nil {
		return errors.Trace(err)
	}
	controllerName := c.ControllerName()
	accountDetails, err := store.AccountDetails(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if user == "" && accountDetails == nil {
		// The username has not been specified, and there is no
		// current account. See if the user can log in with
		// macaroons.
		args, err := c.NewAPIConnectionParams(
			store, controllerName, "",
			&jujuclient.AccountDetails{},
		)
		if err != nil {
			return errors.Trace(err)
		}
		conn, err := newAPIConnection(args)
		if err == nil {
			authTag := conn.AuthTag()
			conn.Close()
			ctx.Infof("You are now logged in to %q as %q.", controllerName, authTag.Id())
			return nil
		}
		if !params.IsCodeNoCreds(err) {
			return errors.Annotate(err, "creating API connection")
		}
		// CodeNoCreds was returned, which means that external
		// users are not supported. Fall back to prompting the
		// user for their username and password.
	}

	if user == "" {
		// The username has not been specified, so prompt for it.
		fmt.Fprint(ctx.Stderr, "username: ")
		var err error
		user, err = readLine(ctx.Stdin)
		if err != nil {
			return errors.Trace(err)
		}
		if user == "" {
			return errors.Errorf("you must specify a username")
		}
	}
	if !names.IsValidUserName(user) {
		return errors.NotValidf("user name %q", user)
	}
	userTag := names.NewUserTag(user)

	// Make sure that the client is not already logged in,
	// or if it is, that it is logged in as the specified
	// user.
	if accountDetails != nil && accountDetails.User != userTag.Id() {
		return errors.New(`already logged in

Run "juju logout" first before attempting to log in as a different user.`)
	}

	// Log in without specifying a password in the account details. This
	// will trigger macaroon-based authentication, which will prompt the
	// user for their password.
	accountDetails = &jujuclient.AccountDetails{
		User: userTag.Id(),
	}
	params, err := c.NewAPIConnectionParams(store, controllerName, "", accountDetails)
	if err != nil {
		return errors.Trace(err)
	}
	conn, err := newAPIConnection(params)
	if err != nil {
		return errors.Annotate(err, "creating API connection")
	}
	defer conn.Close()

	accountDetails.LastKnownAccess = conn.ControllerAccess()
	if err := store.UpdateAccount(controllerName, *accountDetails); err != nil {
		return errors.Annotate(err, "failed to record temporary credential")
	}
	ctx.Infof("You are now logged in to %q as %q.", controllerName, userTag.Id())
	return nil
}

func (c *loginCommand) controllerLogin(ctx *cmd.Context, store jujuclient.ClientStore) error {
	knownDomains, err := c.getKnownControllerDomains()
	if err != nil {
		// TODO(rogpeppe) perhaps this shouldn't be fatal.
		return errors.Trace(err)
	}
	controllerHost := knownDomains[c.userOrDomain]
	if controllerHost == "" {
		if !c.forceHost {
			return errNotControllerLogin
		}
		controllerHost = c.userOrDomain
	}
	if c.controllerName == "" {
		// No explicitly specified controller name, so
		// derive it from the domain.
		if strings.Contains(c.userOrDomain, ":") {
			return errors.Errorf("cannot use %q as controller name - use -c flag to choose a different one", c.userOrDomain)
		}
		c.controllerName = c.userOrDomain
	}
	store = modelcmd.QualifyingClientStore{store}
	controllerDetails, accountDetails, err := c.publicControllerDetails(controllerHost)
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.updateController(
		store,
		c.controllerName,
		controllerDetails,
		accountDetails,
	); err != nil {
		return errors.Trace(err)
	}
	// Log into the controller to verify the credentials, and
	// list the models available.
	models, err := listModels(&c.ControllerCommandBase, accountDetails.User)
	if err != nil {
		return errors.Trace(err)
	}
	for _, model := range models {
		owner := names.NewUserTag(model.Owner)
		if err := store.UpdateModel(
			c.controllerName,
			jujuclient.JoinOwnerModelName(owner, model.Name),
			jujuclient.ModelDetails{model.UUID},
		); err != nil {
			return errors.Annotate(err, "storing model details")
		}
	}
	if err := store.SetCurrentController(c.controllerName); err != nil {
		return errors.Trace(err)
	}

	fmt.Fprintf(
		ctx.Stderr, "Welcome, %s. You are now logged into %q.\n",
		friendlyUserName(accountDetails.User), c.controllerName,
	)
	return c.maybeSetCurrentModel(ctx, store, c.controllerName, accountDetails.User, models)
}

// publicControllerDetails returns controller and account details to be registered
// for the given public controller host name.
func (c *loginCommand) publicControllerDetails(host string) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
	fail := func(err error) (jujuclient.ControllerDetails, jujuclient.AccountDetails, error) {
		return jujuclient.ControllerDetails{}, jujuclient.AccountDetails{}, err
	}
	apiAddr := host
	if !strings.Contains(apiAddr, ":") {
		apiAddr += ":443"
	}
	// Make a direct API connection because we don't yet know the
	// controller UUID so can't store the thus-incomplete controller
	// details to make a conventional connection.
	//
	// Unfortunately this means we'll connect twice to the controller
	// but it's probably best to go through the conventional path the
	// second time.
	bclient, err := c.BakeryClient()
	if err != nil {
		return fail(errors.Trace(err))
	}
	dialOpts := api.DefaultDialOpts()
	dialOpts.BakeryClient = bclient
	conn, err := apiOpen(&c.JujuCommandBase, &api.Info{
		Addrs: []string{apiAddr},
	}, dialOpts)
	if err != nil {
		return fail(errors.Trace(err))
	}
	defer conn.Close()
	user, ok := conn.AuthTag().(names.UserTag)
	if !ok {
		return fail(errors.Errorf("logged in as %v, not a user", conn.AuthTag()))
	}
	// If we get to here, then we have a cached macaroon for the registered
	// user. If we encounter an error after here, we need to clear it.
	c.onRunError = func() {
		if err := c.ClearControllerMacaroons([]string{apiAddr}); err != nil {
			logger.Errorf("failed to clear macaroon: %v", err)
		}
	}
	return jujuclient.ControllerDetails{
			APIEndpoints:   []string{apiAddr},
			ControllerUUID: conn.ControllerTag().Id(),
		}, jujuclient.AccountDetails{
			User:            user.Id(),
			LastKnownAccess: conn.ControllerAccess(),
		}, nil
}

// updateController updates the controller and account details in the given client store,
// using the given controller name and adding the controller if necessary.
func (c *loginCommand) updateController(
	store jujuclient.ClientStore,
	controllerName string,
	controllerDetails jujuclient.ControllerDetails,
	accountDetails jujuclient.AccountDetails,
) error {
	// Check that the same controller isn't already stored, so that we
	// can avoid needlessly asking for a controller name in that case.
	all, err := store.AllControllers()
	if err != nil {
		return errors.Trace(err)
	}
	for name, ctl := range all {
		if ctl.ControllerUUID == controllerDetails.ControllerUUID {
			// TODO(rogpeppe) lp#1614010 Succeed but override the account details in this case?
			return errors.Errorf("controller is already registered as %q", name)
		}
	}
	if err := store.AddController(controllerName, controllerDetails); err != nil {
		return errors.Trace(err)
	}
	if err := store.UpdateAccount(controllerName, accountDetails); err != nil {
		return errors.Annotatef(err, "cannot update account information: %v", err)
	}
	return nil
}

const noModelsMessage = `
There are no models available. You can add models with
"juju add-model", or you can ask an administrator or owner
of a model to grant access to that model with "juju grant".
`

func (c *loginCommand) maybeSetCurrentModel(ctx *cmd.Context, store jujuclient.ClientStore, controllerName, userName string, models []apibase.UserModel) error {
	if len(models) == 0 {
		fmt.Fprint(ctx.Stderr, noModelsMessage)
		return nil
	}

	// If we get to here, there is at least one model.
	if len(models) == 1 {
		// There is exactly one model shared,
		// so set it as the current model.
		model := models[0]
		owner := names.NewUserTag(model.Owner)
		modelName := jujuclient.JoinOwnerModelName(owner, model.Name)
		err := store.SetCurrentModel(controllerName, modelName)
		if err != nil {
			return errors.Trace(err)
		}
		fmt.Fprintf(ctx.Stderr, "\nCurrent model set to %q.\n", modelName)
		return nil
	}
	fmt.Fprintf(ctx.Stderr, `
There are %d models available. Use "juju switch" to select
one of them:
`, len(models))
	user := names.NewUserTag(userName)
	ownerModelNames := make(set.Strings)
	otherModelNames := make(set.Strings)
	for _, model := range models {
		if model.Owner == userName {
			ownerModelNames.Add(model.Name)
			continue
		}
		owner := names.NewUserTag(model.Owner)
		modelName := common.OwnerQualifiedModelName(model.Name, owner, user)
		otherModelNames.Add(modelName)
	}
	for _, modelName := range ownerModelNames.SortedValues() {
		fmt.Fprintf(ctx.Stderr, "  - juju switch %s\n", modelName)
	}
	for _, modelName := range otherModelNames.SortedValues() {
		fmt.Fprintf(ctx.Stderr, "  - juju switch %s\n", modelName)
	}
	return nil
}

// getKnownControllerDomains returns the list of known
// controller domain aliases.
func (c *loginCommand) getKnownControllerDomains() (map[string]string, error) {
	controllers := os.Getenv("JUJU_PUBLIC_CONTROLLERS")
	m, err := keyvalues.Parse(strings.Fields(controllers), false)
	if err != nil {
		return nil, errors.Annotatef(err, "bad value for JUJU_PUBLIC_CONTROLLERS")
	}
	return m, nil
}

func friendlyUserName(user string) string {
	u := names.NewUserTag(user)
	if u.IsLocal() {
		return u.Name()
	}
	return u.Id()
}
