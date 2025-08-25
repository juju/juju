// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/authentication"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
)

const userChangePasswordDoc = `
The user is, by default, the current user. The latter can be confirmed with
the ` + "`juju show-user`" + ` command.

If no controller is specified, the current controller will be used.

A controller administrator can change the password for another user
by providing desired username as an argument.

A controller administrator can also reset the password with a ` + "`--reset`" + ` option.
This will invalidate any passwords that were previously set
and registration strings that were previously issued for a user.
This option will issue a new registration string to be used with
` + "`juju register`" + `.`

const userChangePasswordExamples = `
    juju change-user-password
    juju change-user-password bob
    juju change-user-password bob --reset
    juju change-user-password -c another-known-controller
    juju change-user-password bob --controller another-known-controller
`

func NewChangePasswordCommand() cmd.Command {
	var cmd changePasswordCommand
	cmd.newAPIConnection = juju.NewAPIConnection
	return modelcmd.WrapController(&cmd)
}

// changePasswordCommand changes the password for a user.
type changePasswordCommand struct {
	modelcmd.ControllerCommandBase
	newAPIConnection func(juju.NewAPIConnectionParams) (api.Connection, error)
	api              ChangePasswordAPI

	// Input arguments
	User  string
	Reset bool

	noPrompt bool

	// Internally initialised and used during run
	controllerName string
	userTag        names.UserTag
	accountDetails *jujuclient.AccountDetails
}

func (c *changePasswordCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Reset, "reset", false, "Reset user password.")
	f.BoolVar(&c.noPrompt, "no-prompt", false, "Don't prompt for password; instead, read a line from stdin.")
}

// Info implements Command.Info.
func (c *changePasswordCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "change-user-password",
		Args:     "[username]",
		Purpose:  "Changes the password for the current or specified Juju user.",
		Doc:      userChangePasswordDoc,
		Examples: userChangePasswordExamples,
		SeeAlso: []string{
			"add-user",
			"register",
		},
	})
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
	SetPassword(username, password string) error
	ResetPassword(username string) ([]byte, error)
	Close() error
}

// Run implements Command.Run.
func (c *changePasswordCommand) Run(ctx *cmd.Context) error {
	if err := c.prepareRun(); err != nil {
		return errors.Trace(err)
	}
	if c.api == nil {
		api, err := c.NewUserManagerAPIClient()
		if err != nil {
			return errors.Trace(err)
		}
		c.api = api
		defer c.api.Close()
	}

	if c.Reset {
		if c.User == "" || (c.accountDetails != nil && c.User == c.accountDetails.User) {
			ctx.Infof("You cannot reset your own password.\nIf you want to change it, please call `juju change-user-password` without --reset option.")
			return nil
		}
		return c.resetUserPassword(ctx)
	}
	return c.updateUserPassword(ctx)
}

func (c *changePasswordCommand) prepareRun() error {
	err := c.ensureControllerName()
	if err != nil {
		return errors.Trace(err)
	}

	c.accountDetails, err = c.ClientStore().AccountDetails(c.controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	if c.User != "" {
		if !names.IsValidUserName(c.User) {
			return errors.NotValidf("user name %q", c.User)
		}
		c.userTag = names.NewUserTag(c.User)
		if c.userTag.Id() != c.accountDetails.User {
			// The account details don't correspond to the username
			// being changed, so we don't need to update the account
			// locally.
			c.accountDetails = nil
		}
	} else {
		if !names.IsValidUser(c.accountDetails.User) {
			return errors.Errorf("invalid user in account %q", c.accountDetails.User)
		}
		c.userTag = names.NewUserTag(c.accountDetails.User)
		if !c.userTag.IsLocal() {
			operation := "change"
			if c.Reset {
				operation = "reset"
			}
			return errors.Errorf("cannot %v password for external user %q", operation, c.userTag)
		}
	}
	return nil
}

func (c *changePasswordCommand) ensureControllerName() error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	c.controllerName = controllerName
	return nil
}

func (c *changePasswordCommand) resetUserPassword(ctx *cmd.Context) error {
	key, err := c.api.ResetPassword(c.userTag.Id())
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	ctx.Infof("Password for %q has been reset.", c.User)
	base64RegistrationData, err := generateUserControllerAccessToken(
		&c.ControllerCommandBase,
		c.userTag.Id(),
		key,
	)
	if err != nil {
		return errors.Annotate(err, "generating controller user access token")
	}
	ctx.Infof("Ask the user to run:\n     juju register %s\n", base64RegistrationData)
	return nil
}

func (c *changePasswordCommand) updateUserPassword(ctx *cmd.Context) error {
	var err error
	var newPassword string
	if c.noPrompt {
		fmt.Fprintln(ctx.Stderr, "reading password from stdin...")
		newPassword, err = readLine(ctx.Stdin)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		newPassword, err = readAndConfirmPassword(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}

	if newPassword == "" {
		return errors.Errorf("password cannot be empty")
	}

	if err := c.api.SetPassword(c.userTag.Id(), newPassword); err != nil {
		return block.ProcessBlockedError(err, block.BlockChange)
	}
	if c.accountDetails == nil {
		ctx.Infof("Password for %q has been changed.", c.User)
	} else {
		if c.accountDetails.Password != "" {
			// Log back in with macaroon authentication, so we can
			// discard the password without having to log back in
			// immediately.
			if err := c.recordMacaroon(newPassword); err != nil {
				return errors.Annotate(err, "recording macaroon")
			}
			// Wipe the password from disk. In the event of an
			// error occurring after SetPassword and before the
			// account details being updated, the user will be
			// able to recover by running "juju login".
			c.accountDetails.Password = ""
			if err := c.ClientStore().UpdateAccount(c.controllerName, *c.accountDetails); err != nil {
				return errors.Annotate(err, "failed to update client credentials")
			}
		}
		ctx.Infof("Your password has been changed.")
	}
	return nil
}

func (c *changePasswordCommand) recordMacaroon(password string) error {
	accountDetails := &jujuclient.AccountDetails{User: c.accountDetails.User}
	args, err := c.NewAPIConnectionParams(
		c.ClientStore(), c.controllerName, "", accountDetails,
	)
	if err != nil {
		return errors.Trace(err)
	}
	args.DialOpts.BakeryClient.InteractionMethods = []httpbakery.Interactor{
		authentication.NewInteractor(accountDetails.User, func(string) (string, error) {
			return password, nil
		}),
		httpbakery.WebBrowserInteractor{},
	}
	api, err := c.newAPIConnection(args)
	if err != nil {
		return errors.Annotate(err, "connecting to API")
	}
	return api.Close()
}

func readAndConfirmPassword(ctx *cmd.Context) (string, error) {
	// Don't add the carriage returns before readPassword, but add
	// them directly after the readPassword so any errors are output
	// on their own lines.
	//
	// TODO(axw) retry/loop on failure
	fmt.Fprint(ctx.Stderr, "new password: ")
	password, err := readPassword(ctx.Stdin)
	fmt.Fprint(ctx.Stderr, "\n")
	if err != nil {
		return "", errors.Trace(err)
	}
	if password == "" {
		return "", errors.Errorf("you must enter a password")
	}

	fmt.Fprint(ctx.Stderr, "type new password again: ")
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
