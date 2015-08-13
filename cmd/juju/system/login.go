// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/api/usermanager"
	"github.com/juju/names"
	"github.com/juju/utils"
	goyaml "gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/network"
)

// GetUserManagerFunc defines a function that takes an api connection
// and returns the (locally defined) UserManager interface.
type GetUserManagerFunc func(conn api.Connection) (UserManager, error)

// LoginCommand logs in to a Juju system and caches the connection
// information.
type LoginCommand struct {
	cmd.CommandBase
	apiOpen        api.OpenFunc
	getUserManager GetUserManagerFunc
	// TODO (thumper): when we support local cert definitions
	// allow the use to specify the user and server address.
	// user      string
	// address   string
	Server       cmd.FileVar
	Name         string
	KeepPassword bool
}

var loginDoc = `
login connects to a juju system and caches the information that juju
needs to connect to the api server in the $(JUJU_HOME)/environments directory.

In order to login to a system, you need to have a user already created for you
in that system. The way that this occurs is for an existing user on the system
to create you as a user. This will generate a file that contains the
information needed to connect.

If you have been sent one of these server files, you can login by doing the
following:

    # if you have saved the server file as ~/erica.server
    juju system login --server=~/erica.server test-system

A new strong random password is generated to replace the password defined in
the server file. The 'test-system' will also become the current system that
the juju command will talk to by default.

If you have used the 'api-info' command to generate a copy of your current
credentials for a system, you should use the --keep-password option as it will
mean that you will still be able to connect to the api server from the
computer where you ran api-info.

See Also:
    juju help system environments
    juju help system use-environment
    juju help system create-environment
    juju help user add
    juju help switch
`

// Info implements Command.Info
func (c *LoginCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name: "login",
		// TODO(thumper): support user and address options
		// Args: "<name> [<server address>[:<server port>]]"
		Args:    "<name>",
		Purpose: "login to a Juju System",
		Doc:     loginDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *LoginCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.Server, "server", "path to yaml-formatted server file")
	f.BoolVar(&c.KeepPassword, "keep-password", false, "do not generate a new random password")
}

// SetFlags implements Command.Init.
func (c *LoginCommand) Init(args []string) error {
	if c.apiOpen == nil {
		c.apiOpen = apiOpen
	}
	if c.getUserManager == nil {
		c.getUserManager = getUserManager
	}
	if len(args) == 0 {
		return errors.New("no name specified")
	}

	c.Name, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

// Run implements Command.Run
func (c *LoginCommand) Run(ctx *cmd.Context) error {
	// TODO(thumper): as we support the user and address
	// change this check here.
	if c.Server.Path == "" {
		return errors.New("no server file specified")
	}

	serverYAML, err := c.Server.Read(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	var serverDetails envcmd.ServerFile
	if err := goyaml.Unmarshal(serverYAML, &serverDetails); err != nil {
		return errors.Trace(err)
	}

	// Construct the api.Info struct from the provided values
	// and attempt to connect to the remote server before we do anything else.
	if !names.IsValidUser(serverDetails.Username) {
		return errors.Errorf("%q is not a valid username", serverDetails.Username)
	}

	userTag := names.NewUserTag(serverDetails.Username)
	if userTag.Provider() != names.LocalProvider {
		// Remove users do not have their passwords stored in Juju
		// so we never attempt to change them.
		c.KeepPassword = true
	}

	info := api.Info{
		Addrs:    serverDetails.Addresses,
		CACert:   serverDetails.CACert,
		Tag:      userTag,
		Password: serverDetails.Password,
	}

	apiState, err := c.apiOpen(&info, api.DefaultDialOpts())
	if err != nil {
		return errors.Trace(err)
	}
	defer apiState.Close()

	// If we get to here, the credentials supplied were sufficient to connect
	// to the Juju System and login. Now we cache the details.
	serverInfo, err := c.cacheConnectionInfo(serverDetails, apiState)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.Infof("cached connection details as system %q", c.Name)

	// If we get to here, we have been able to connect to the API server, and
	// also have been able to write the cached information. Now we can change
	// the user's password to a new randomly generated strong password, and
	// update the cached information knowing that the likelihood of failure is
	// minimal.
	if !c.KeepPassword {
		if err := c.updatePassword(ctx, apiState, userTag, serverInfo); err != nil {
			return errors.Trace(err)
		}
	}

	return errors.Trace(envcmd.SetCurrentSystem(ctx, c.Name))
}

func (c *LoginCommand) cacheConnectionInfo(serverDetails envcmd.ServerFile, apiState api.Connection) (configstore.EnvironInfo, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serverInfo := store.CreateInfo(c.Name)

	serverTag, err := apiState.ServerTag()
	if err != nil {
		return nil, errors.Wrap(err, errors.New("juju system too old to support login"))
	}

	connectedAddresses, err := network.ParseHostPorts(apiState.Addr())
	if err != nil {
		// Should never happen, since we've just connected with it.
		return nil, errors.Annotatef(err, "invalid API address %q", apiState.Addr())
	}
	addressConnectedTo := connectedAddresses[0]

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(serverInfo, apiState.APIHostPorts(), addressConnectedTo)
	if !changed {
		logger.Infof("api addresses: %v", apiState.APIHostPorts())
		logger.Infof("address connected to: %v", addressConnectedTo)
		return nil, errors.New("no addresses returned from prepare for caching")
	}

	serverInfo.SetAPICredentials(
		configstore.APICredentials{
			User:     serverDetails.Username,
			Password: serverDetails.Password,
		})

	serverInfo.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:  addrs,
		Hostnames:  hosts,
		CACert:     serverDetails.CACert,
		ServerUUID: serverTag.Id(),
	})

	if err = serverInfo.Write(); err != nil {
		return nil, errors.Trace(err)
	}
	return serverInfo, nil
}

func (c *LoginCommand) updatePassword(ctx *cmd.Context, conn api.Connection, userTag names.UserTag, serverInfo configstore.EnvironInfo) error {
	password, err := utils.RandomPassword()
	if err != nil {
		return errors.Annotate(err, "failed to generate random password")
	}

	userManager, err := c.getUserManager(conn)
	if err != nil {
		return errors.Trace(err)
	}
	if err := userManager.SetPassword(userTag.Name(), password); err != nil {
		errors.Trace(err)
	}
	ctx.Infof("password updated\n")
	creds := serverInfo.APICredentials()
	creds.Password = password
	serverInfo.SetAPICredentials(creds)
	if err = serverInfo.Write(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func apiOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	return api.Open(info, opts)
}

// UserManager defines the calls that the Login command makes to the user
// manager client. It is returned by a helper function that is overridden in
// tests.
type UserManager interface {
	SetPassword(username, password string) error
}

func getUserManager(conn api.Connection) (UserManager, error) {
	return usermanager.NewClient(conn), nil
}
