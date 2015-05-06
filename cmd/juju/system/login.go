// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	goyaml "gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/network"
)

// ServerFile format
// This will need to move when the user manager commands generate
// this file format.  The file format is expected to be YAML.
type ServerFile struct {
	Addresses []string `yaml:"addresses"`
	CACert    string   `yaml:"ca-cert,omitempty"`
	Username  string   `yaml:"username"`
	Password  string   `yaml:"password"`
}

// LoginCommand logs in to a Juju system and caches the connection
// information.
type LoginCommand struct {
	cmd.CommandBase
	// TODO (thumper): when we support local cert definitions
	// allow the use to specify the user and server address.
	// user      string
	// address   string
	Server cmd.FileVar
	Name   string
}

var loginDoc = `TODO: add more documentation...`

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
}

// SetFlags implements Command.Init.
func (c *LoginCommand) Init(args []string) error {
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

	var serverDetails ServerFile
	if err := goyaml.Unmarshal(serverYAML, &serverDetails); err != nil {
		return errors.Annotate(err, "unexpected YAML content")
	}

	// Construct the api.Info struct from the provided values
	// and attempt to connect to the remote server before we do anything else.
	if !names.IsValidUser(serverDetails.Username) {
		return errors.Errorf("%q is not a valid username", serverDetails.Username)
	}

	var info api.Info
	info.Addrs = serverDetails.Addresses
	info.CACert = serverDetails.CACert
	info.Tag = names.NewUserTag(serverDetails.Username)
	info.Password = serverDetails.Password

	apiState, err := api.Open(&info, api.DefaultDialOpts())
	if err != nil {
		return errors.Trace(err)
	}
	defer apiState.Close()

	// If we get to here, the credentials supplied were sufficient to connect
	// to the Juju System and login. Now we cache the details.
	store, err := configstore.Default()
	if err != nil {
		return errors.Trace(err)
	}
	serverInfo := store.CreateInfo(c.Name)

	connectedAddresses, err := network.ParseHostPorts(apiState.Addr())
	if err != nil {
		// Should never happen, since we've just connected with it.
		return errors.Annotatef(err, "invalid API address %q", apiState.Addr())
	}
	addressConnectedTo := connectedAddresses[0]

	addrs, hosts, changed := juju.PrepareEndpointsForCaching(serverInfo, apiState.APIHostPorts(), addressConnectedTo)
	if !changed {
		logger.Infof("api addresses: %v", apiState.APIHostPorts())
		logger.Infof("address connected to: %v", addressConnectedTo)
		return errors.New("no addresses returned from prepare for caching")
	}

	serverInfo.SetAPICredentials(
		configstore.APICredentials{
			User:     serverDetails.Username,
			Password: serverDetails.Password,
		})
	serverTag, err := apiState.ServerTag()
	if err != nil {
		return errors.Wrap(err, errors.New("juju system too old to support login"))
	}

	serverInfo.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:  addrs,
		Hostnames:  hosts,
		CACert:     serverDetails.CACert,
		ServerUUID: serverTag.Id(),
	})

	return serverInfo.Write()

	// TODO: write out the current-system file.
}
