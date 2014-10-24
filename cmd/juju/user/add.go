// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/environs/configstore"
)

const userAddCommandDoc = `
Add users to an existing environment.

The user information is stored within an existing environment, and will be
lost when the environent is destroyed.  An environment file (.jenv) will be
written out in the current directory.  You can control the name and location
of this file using the --output option.

Examples:
  # Add user "foobar". You will be prompted to enter a password.
  juju user add foobar

  # Add user "foobar" with a strong random password is generated.
  juju user add foobar --generate


See Also:
  juju user change-password
`

// AddCommand adds new users into a Juju Server.
type AddCommand struct {
	UserCommandBase
	User        string
	DisplayName string
	Password    string
	OutPath     string
	Generate    bool
}

// Info implements Command.Info.
func (c *AddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add",
		Args:    "<username> [<display name>]",
		Purpose: "adds a user",
		Doc:     userAddCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *AddCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Generate, "generate", false, "generate a new strong password")
	f.StringVar(&c.OutPath, "o", "", "specify the environment file for new user")
	f.StringVar(&c.OutPath, "output", "", "")
}

// Init implements Command.Init.
func (c *AddCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no username supplied")
	}
	c.User, args = args[0], args[1:]
	if len(args) > 0 {
		c.DisplayName, args = args[0], args[1:]
	}
	return cmd.CheckEmpty(args)
}

// AddUserAPI defines the usermanager API methods that the add command uses.
type AddUserAPI interface {
	AddUser(username, displayName, password string) (names.UserTag, error)
	Close() error
}

// ShareEnvironmentAPI defines the client API methods that the add command uses.
type ShareEnvironmentAPI interface {
	ShareEnvironment(users []names.UserTag) error
	Close() error
}

func (c *AddCommand) getAddUserAPI() (AddUserAPI, error) {
	return c.NewUserManagerClient()
}

func (c *AddCommand) getShareEnvAPI() (ShareEnvironmentAPI, error) {
	return c.NewAPIClient()
}

var (
	getAddUserAPI  = (*AddCommand).getAddUserAPI
	getShareEnvAPI = (*AddCommand).getShareEnvAPI
)

// Run implements Command.Run.
func (c *AddCommand) Run(ctx *cmd.Context) error {
	client, err := getAddUserAPI(c)
	if err != nil {
		return err
	}
	defer client.Close()

	if !c.Generate {
		ctx.Infof("To generate a random strong password, use the --generate flag.")
	}

	shareClient, err := getShareEnvAPI(c)
	if err != nil {
		return err
	}
	defer shareClient.Close()

	c.Password, err = c.generateOrReadPassword(ctx, c.Generate)
	if err != nil {
		return errors.Trace(err)
	}

	tag, err := client.AddUser(c.User, c.DisplayName, c.Password)
	if err != nil {
		return err
	}
	// Until we have multiple environments stored in a state server
	// it makes no sense at all to create a user and not have that user
	// able to log in and use the one and only environment.
	// So we share the existing environment with the user here and now.
	err = shareClient.ShareEnvironment([]names.UserTag{tag})
	if err != nil {
		return err
	}

	user := c.User
	if c.DisplayName != "" {
		user = fmt.Sprintf("%s (%s)", c.DisplayName, user)
	}

	fmt.Fprintf(ctx.Stdout, "user %q added\n", user)
	if c.OutPath == "" {
		c.OutPath = c.User + ".jenv"
	}

	outPath := normaliseJenvPath(ctx, c.OutPath)
	err = generateUserJenv(c.ConnectionName(), c.User, c.Password, outPath)
	if err == nil {
		fmt.Fprintf(ctx.Stdout, "environment file written to %s\n", outPath)
	}

	return err
}

func normaliseJenvPath(ctx *cmd.Context, outPath string) string {
	if !strings.HasSuffix(outPath, ".jenv") {
		outPath = outPath + ".jenv"
	}
	return ctx.AbsPath(outPath)
}

func generateUserJenv(envName, user, password, outPath string) error {
	store, err := configstore.Default()
	if err != nil {
		return errors.Trace(err)
	}
	storeInfo, err := store.ReadInfo(envName)
	if err != nil {
		return errors.Trace(err)
	}
	endpoint := storeInfo.APIEndpoint()
	outputInfo := configstore.EnvironInfoData{
		User:         user,
		Password:     password,
		EnvironUUID:  endpoint.EnvironUUID,
		StateServers: endpoint.Addresses,
		CACert:       endpoint.CACert,
	}
	yaml, err := cmd.FormatYaml(outputInfo)
	if err != nil {
		return errors.Trace(err)
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return errors.Trace(err)
	}
	defer outFile.Close()
	outFile.Write(yaml)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
