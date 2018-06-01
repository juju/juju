// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/httprequest"
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
By default, the juju login command logs the user into a controller.
The argument to the command can be a public controller
host name or alias (see Aliases below).

If no argument is provided, the controller specified with
the -c argument will be used, or the current controller
if that's not provided.

On success, the current controller is switched to the logged-in
controller.

If the user is already logged in, the juju login command does nothing
except verify that fact.

If the -u flag is provided, the juju login command will attempt to log
into the controller as that user.

After login, a token ("macaroon") will become active. It has an expiration
time of 24 hours. Upon expiration, no further Juju commands can be issued
and the user will be prompted to log in again.

Aliases
-------

Public controller aliases are provided by a directory service
that is queried to find the host name for a given alias.
The URL for the directory service may be configured
by setting the environment variable JUJU_DIRECTORY.

Examples:

    juju login somepubliccontroller
    juju login jimm.jujucharms.com
    juju login -u bob

See also:
    disable-user
    enable-user
    logout
    register
    unregister
`

// Functions defined as variables so they can be overridden in tests.
var (
	apiOpen          = (*modelcmd.CommandBase).APIOpen
	newAPIConnection = juju.NewAPIConnection
	listModels       = func(c api.Connection, userName string) ([]apibase.UserModel, error) {
		return modelmanager.NewClient(c).ListModels(userName)
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
	domain   string
	username string

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
		Args:    "[controller host name or alias]",
		Purpose: "Logs a user in to a controller.",
		Doc:     loginDoc,
	}
}

func (c *loginCommand) SetFlags(fset *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(fset)
	fset.StringVar(&c.controllerName, "c", "", "Controller to operate in")
	fset.StringVar(&c.controllerName, "controller", "", "")
	fset.StringVar(&c.username, "u", "", "log in as this local user")
	fset.StringVar(&c.username, "user", "", "")
}

// Init implements Command.Init.
func (c *loginCommand) Init(args []string) error {
	domain, err := cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	c.domain = domain
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

func (c *loginCommand) run(ctx *cmd.Context) error {
	store := c.ClientStore()
	switch {
	case c.controllerName == "" && c.domain == "":
		current, err := store.CurrentController()
		if err != nil && !errors.IsNotFound(err) {
			return errors.Annotatef(err, "cannot get current controller")
		}
		c.controllerName = current
	case c.controllerName == "":
		c.controllerName = c.domain
	}
	if strings.Contains(c.controllerName, ":") {
		return errors.Errorf("cannot use %q as a controller name - use -c flag to choose a different one", c.controllerName)
	}

	// Find out details on the specified controller if there is one.
	var controllerDetails *jujuclient.ControllerDetails
	if c.controllerName != "" {
		d, err := store.ControllerByName(c.controllerName)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		controllerDetails = d
	}

	// Find out details of the controller domain if it's specified.
	var (
		conn                    api.Connection
		publicControllerDetails *jujuclient.ControllerDetails
		accountDetails          *jujuclient.AccountDetails
		oldAccountDetails       *jujuclient.AccountDetails
		err                     error
	)
	if controllerDetails != nil {
		// Fetch current details for the specified controller name so we
		// can tell if the logged in user has changed.
		d, err := store.AccountDetails(c.controllerName)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		oldAccountDetails = d
	}
	switch {
	case c.domain != "":
		// Note: the controller name is guaranteed to be non-empty
		// in this case via the test at the start of this function.
		conn, publicControllerDetails, accountDetails, err = c.publicControllerLogin(ctx, c.domain, c.controllerName, oldAccountDetails)
		if err != nil {
			return errors.Annotatef(err, "cannot log into %q", c.domain)
		}
	case controllerDetails == nil && c.controllerName != "":
		// No controller found and no domain specified - we
		// have no idea where we should be logging in.
		return errors.Errorf("controller %q does not exist", c.controllerName)
	case controllerDetails == nil:
		return errors.Errorf("no current controller")
	default:
		conn, accountDetails, err = c.existingControllerLogin(ctx, store, c.controllerName, oldAccountDetails)
		if err != nil {
			return errors.Annotatef(err, "cannot log into controller %q", c.controllerName)
		}
	}
	defer conn.Close()
	if controllerDetails != nil && publicControllerDetails != nil && controllerDetails.ControllerUUID != publicControllerDetails.ControllerUUID {
		// The domain we're trying to log into doesn't match the
		// existing controller.
		return errors.Errorf(`
controller at %q does not match existing controller.
Please choose a different controller name with the -c flag, or
use "juju unregister %s" to remove the existing controller.`[1:], c.domain, c.controllerName)
	}
	if controllerDetails == nil {
		// The controller did not exist previously, so create it.
		// Note that the "controllerDetails == nil"
		// test above means that we will always have a valid publicControllerDetails
		// value here.
		if err := store.AddController(c.controllerName, *publicControllerDetails); err != nil {
			return errors.Trace(err)
		}
	}
	accountDetails.LastKnownAccess = conn.ControllerAccess()
	if err := store.UpdateAccount(c.controllerName, *accountDetails); err != nil {
		return errors.Annotatef(err, "cannot update account information: %v", err)
	}
	if err := store.SetCurrentController(c.controllerName); err != nil {
		return errors.Annotatef(err, "cannot switch")
	}
	if controllerDetails != nil && oldAccountDetails != nil && oldAccountDetails.User == accountDetails.User {
		// We're still using the same controller and the same user name,
		// so no need to list models or set the current controller
		return nil
	}
	// Now list the models available so we can show them and store their
	// details locally.
	models, err := listModels(conn, accountDetails.User)
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.SetControllerModels(store, c.controllerName, models); err != nil {
		return errors.Annotate(err, "storing model details")
	}
	fmt.Fprintf(
		ctx.Stderr, "Welcome, %s. You are now logged into %q.\n",
		friendlyUserName(accountDetails.User), c.controllerName,
	)
	return c.maybeSetCurrentModel(ctx, store, c.controllerName, accountDetails.User, models)
}

func (c *loginCommand) existingControllerLogin(ctx *cmd.Context, store jujuclient.ClientStore, controllerName string, currentAccountDetails *jujuclient.AccountDetails) (api.Connection, *jujuclient.AccountDetails, error) {
	dial := func(accountDetails *jujuclient.AccountDetails) (api.Connection, error) {
		args, err := c.NewAPIConnectionParams(store, controllerName, "", accountDetails)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newAPIConnection(args)
	}
	return c.login(ctx, currentAccountDetails, dial)
}

// publicControllerLogin logs into the public controller at the given
// host. The currentAccountDetails parameter holds existing account
// information about the controller account.
func (c *loginCommand) publicControllerLogin(
	ctx *cmd.Context,
	host string,
	controllerName string,
	currentAccountDetails *jujuclient.AccountDetails,
) (api.Connection, *jujuclient.ControllerDetails, *jujuclient.AccountDetails, error) {
	fail := func(err error) (api.Connection, *jujuclient.ControllerDetails, *jujuclient.AccountDetails, error) {
		return nil, nil, nil, err
	}
	if !strings.ContainsAny(host, ".:") {
		host1, err := c.getKnownControllerDomain(host, controllerName)
		if errors.IsNotFound(err) {
			return fail(errors.Errorf("%q is not a known public controller", host))
		}
		if err != nil {
			return fail(errors.Annotatef(err, "could not determine controller host name"))
		}
		host = host1
	} else if !strings.Contains(host, ":") {
		host += ":443"
	}

	// Make a direct API connection because we don't yet know the
	// controller UUID so can't store the thus-incomplete controller
	// details to make a conventional connection.
	//
	// Unfortunately this means we'll connect twice to the controller
	// but it's probably best to go through the conventional path the
	// second time.
	bclient, err := c.CommandBase.BakeryClient(c.ClientStore(), controllerName)
	if err != nil {
		return fail(errors.Trace(err))
	}
	dialOpts := api.DefaultDialOpts()
	dialOpts.BakeryClient = bclient

	dial := func(d *jujuclient.AccountDetails) (api.Connection, error) {
		var tag names.Tag
		if d.User != "" {
			tag = names.NewUserTag(d.User)
		}
		return apiOpen(&c.CommandBase, &api.Info{
			Tag:      tag,
			Password: d.Password,
			Addrs:    []string{host},
		}, dialOpts)
	}
	conn, accountDetails, err := c.login(ctx, currentAccountDetails, dial)
	if err != nil {
		return fail(errors.Trace(err))
	}
	// If we get to here, then we have a cached macaroon for the registered
	// user. If we encounter an error after here, we need to clear it.
	c.onRunError = func() {
		if err := c.ClearControllerMacaroons(c.ClientStore(), controllerName); err != nil {
			logger.Errorf("failed to clear macaroon: %v", err)
		}
	}
	return conn,
		&jujuclient.ControllerDetails{
			APIEndpoints:   []string{host},
			ControllerUUID: conn.ControllerTag().Id(),
		}, accountDetails, nil
}

// login logs into a controller using the given account details by
// default, but falling back to prompting for a username and password if
// necessary. The details of making an API connection are abstracted out
// into the dial function because we need to dial differently depending
// on whether we have some existing local controller information or not.
//
// The dial function should make API connection using the account
// details that it is passed.
func (c *loginCommand) login(
	ctx *cmd.Context,
	accountDetails *jujuclient.AccountDetails,
	dial func(*jujuclient.AccountDetails) (api.Connection, error),
) (api.Connection, *jujuclient.AccountDetails, error) {
	username := c.username
	if c.username != "" && accountDetails != nil && accountDetails.User != c.username {
		// The user has specified a different username than the
		// user we've found in the controller's account details.
		return nil, nil, errors.Errorf(`already logged in as %s.

Run "juju logout" first before attempting to log in as a different user.`,
			accountDetails.User)
	}

	if accountDetails != nil && accountDetails.Password != "" {
		// We've been provided some account details that
		// contain a password, so try that first.
		conn, err := dial(accountDetails)
		if err == nil {
			return conn, accountDetails, nil
		}
		if !errors.IsUnauthorized(err) {
			return nil, nil, errors.Trace(err)
		}
	}
	if c.username == "" {
		// No username specified, so try external-user login first.
		conn, err := dial(&jujuclient.AccountDetails{})
		if err == nil {
			user, ok := conn.AuthTag().(names.UserTag)
			if !ok {
				conn.Close()
				return nil, nil, errors.Errorf("logged in as %v, not a user", conn.AuthTag())
			}
			return conn, &jujuclient.AccountDetails{
				User: user.Id(),
			}, nil
		}
		if !params.IsCodeNoCreds(err) {
			return nil, nil, errors.Trace(err)
		}
		// CodeNoCreds was returned, which means that external
		// users are not supported. Fall back to prompting the
		// user for their username and password.

		fmt.Fprint(ctx.Stderr, "username: ")
		u, err := readLine(ctx.Stdin)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		if u == "" {
			return nil, nil, errors.Errorf("you must specify a username")
		}
		username = u
	}
	// Log in without specifying a password in the account details. This
	// will trigger macaroon-based authentication, which will prompt the
	// user for their password.
	accountDetails = &jujuclient.AccountDetails{
		User: username,
	}
	conn, err := dial(accountDetails)
	return conn, accountDetails, errors.Trace(err)
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

type controllerDomainResponse struct {
	Host string `json:"host"`
}

const defaultJujuDirectory = "https://api.jujucharms.com/directory"

// getKnownControllerDomain returns the list of known
// controller domain aliases.
func (c *loginCommand) getKnownControllerDomain(name, controllerName string) (string, error) {
	if strings.Contains(name, ".") || strings.Contains(name, ":") {
		return "", errors.NotFoundf("controller %q", name)
	}
	baseURL := defaultJujuDirectory
	if u := os.Getenv("JUJU_DIRECTORY"); u != "" {
		baseURL = u
	}
	client, err := c.CommandBase.BakeryClient(c.ClientStore(), controllerName)
	if err != nil {
		return "", errors.Trace(err)
	}
	req, err := http.NewRequest("GET", baseURL+"/v1/controller/"+name, nil)
	if err != nil {
		return "", errors.Trace(err)
	}
	httpResp, err := client.Do(req)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		if httpResp.StatusCode == http.StatusNotFound {
			return "", errors.NotFoundf("controller %q", name)
		}
		return "", errors.Errorf("unexpected HTTP response %q", httpResp.Status)
	}
	var resp controllerDomainResponse
	if err := httprequest.UnmarshalJSONResponse(httpResp, &resp); err != nil {
		return "", errors.Trace(err)
	}
	if resp.Host == "" {
		return "", errors.Errorf("no host field found in response")
	}
	return resp.Host, nil
}

func friendlyUserName(user string) string {
	u := names.NewUserTag(user)
	if u.IsLocal() {
		return u.Name()
	}
	return u.Id()
}
