// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/macaroon.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// randomPasswordNotify is called when a random password is generated.
var randomPasswordNotify = func(string) {}

const userChangePasswordDoc = `
Change the password for the user you are currently logged in as,
or as an admin, change the password for another user.

Examples:
  # You will be prompted to enter a password.
  juju change-user-password

  # Change the password to a random strong password.
  juju change-user-password --generate

  # Change the password for bob, this always uses a random password
  juju change-user-password bob

`

func NewChangePasswordCommand() cmd.Command {
	return modelcmd.WrapController(&changePasswordCommand{})
}

// changePasswordCommand changes the password for a user.
type changePasswordCommand struct {
	modelcmd.ControllerCommandBase
	api      ChangePasswordAPI
	Generate bool
	User     string
}

// Info implements Command.Info.
func (c *changePasswordCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "change-user-password",
		Args:    "[username]",
		Purpose: "changes the password for a user",
		Doc:     userChangePasswordDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *changePasswordCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Generate, "generate", false, "generate a new strong password")
}

// Init implements Command.Init.
func (c *changePasswordCommand) Init(args []string) error {
	var err error
	c.User, err = cmd.ZeroOrOneArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ChangePasswordAPI defines the usermanager API methods that the change
// password command uses.
type ChangePasswordAPI interface {
	CreateLocalLoginMacaroon(names.UserTag) (*macaroon.Macaroon, error)
	SetPassword(username, password string) error
	Close() error
}

// Run implements Command.Run.
func (c *changePasswordCommand) Run(ctx *cmd.Context) error {
	if c.api == nil {
		api, err := c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		c.api = api
		defer c.api.Close()
	}

	newPassword, err := generateOrReadPassword(ctx, c.Generate)
	if err != nil {
		return errors.Trace(err)
	}

	var accountName string
	controllerName := c.ControllerName()
	store := c.ClientStore()
	if c.User != "" {
		if !names.IsValidUserName(c.User) {
			return errors.NotValidf("user name %q", c.User)
		}
		accountName = names.NewUserTag(c.User).Canonical()
	} else {
		accountName, err = store.CurrentAccount(controllerName)
		if err != nil {
			return errors.Trace(err)
		}
	}
	accountDetails, err := store.AccountByName(controllerName, accountName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	if accountDetails != nil && accountDetails.Macaroon == "" {
		// Generate a macaroon first to guard against I/O failures
		// occurring after the password has been changed, preventing
		// future logins.
		userTag := names.NewUserTag(accountName)
		macaroon, err := c.api.CreateLocalLoginMacaroon(userTag)
		if err != nil {
			return errors.Trace(err)
		}
		accountDetails.Password = ""

		// TODO(axw) update jujuclient with code for marshalling
		// and unmarshalling macaroons as YAML.
		macaroonJSON, err := macaroon.MarshalJSON()
		if err != nil {
			return errors.Trace(err)
		}
		accountDetails.Macaroon = string(macaroonJSON)

		if err := store.UpdateAccount(controllerName, accountName, *accountDetails); err != nil {
			return errors.Annotate(err, "failed to update client credentials")
		}
	}

	if err := c.api.SetPassword(accountName, newPassword); err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	if accountDetails == nil {
		ctx.Infof("Password for %q has been updated.", c.User)
	} else {
		ctx.Infof("Your password has been updated.")
	}
	return nil
}

func generateOrReadPassword(ctx *cmd.Context, generate bool) (string, error) {
	if generate {
		password, err := utils.RandomPassword()
		if err != nil {
			return "", errors.Annotate(err, "failed to generate random password")
		}
		randomPasswordNotify(password)
		return password, nil
	}
	return readAndConfirmPassword(ctx)
}

func readAndConfirmPassword(ctx *cmd.Context) (string, error) {
	// Don't add the carriage returns before readPassword, but add
	// them directly after the readPassword so any errors are output
	// on their own lines.
	fmt.Fprint(ctx.Stderr, "password: ")
	password, err := readPassword(ctx.Stdin)
	fmt.Fprint(ctx.Stderr, "\n")
	if err != nil {
		return "", errors.Trace(err)
	}
	if password == "" {
		return "", errors.Errorf("you must enter a password")
	}

	fmt.Fprint(ctx.Stderr, "type password again: ")
	verify, err := readPassword(ctx.Stdin)
	fmt.Fprint(ctx.Stderr, "\n")
	if err != nil {
		return "", errors.Trace(err)
	}
	if password != verify {
		return "", errors.New("Passwords do not match")
	}
	return password, nil
}

func readPassword(stdin io.Reader) (string, error) {
	if f, ok := stdin.(*os.File); ok && terminal.IsTerminal(int(f.Fd())) {
		password, err := terminal.ReadPassword(int(f.Fd()))
		if err != nil {
			return "", errors.Trace(err)
		}
		return string(password), nil
	}
	return readLine(stdin)
}

func readLine(stdin io.Reader) (string, error) {
	// Read one byte at a time to avoid reading beyond the delimiter.
	line, err := bufio.NewReader(byteAtATimeReader{stdin}).ReadString('\n')
	if err != nil {
		return "", errors.Trace(err)
	}
	return line[:len(line)-1], nil
}

type byteAtATimeReader struct {
	io.Reader
}

func (r byteAtATimeReader) Read(out []byte) (int, error) {
	return r.Reader.Read(out[:1])
}
