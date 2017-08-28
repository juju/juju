// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/httprequest"
	"github.com/juju/version"
	"github.com/juju/webbrowser"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewGUICommand creates and returns a new gui command.
func NewGUICommand() cmd.Command {
	return modelcmd.Wrap(&guiCommand{})
}

// guiCommand opens the Juju GUI in the default browser.
type guiCommand struct {
	modelcmd.ModelCommandBase

	// Deprecated - used with --no-browser
	noBrowser bool

	// Deprecated - used with --show-credentials
	showCreds bool

	hideCreds bool
	browser   bool

	getGUIVersions func(connection api.Connection) ([]params.GUIArchiveVersion, error)
}

const guiDoc = `
Print the Juju GUI URL and show admin credential to use to log into it:

	juju gui

Print the Juju GUI URL only:

	juju gui --hide-credential

Open the Juju GUI in the default browser and show admin credential to use to log into it:

	juju gui --browser

Open the Juju GUI in the default browser without printing the login credential:

	juju gui --hide-credential --browser

An error is returned if the Juju GUI is not available in the controller.
`

// Info implements the cmd.Command interface.
func (c *guiCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "gui",
		Purpose: "Print the Juju GUI URL, or open the Juju GUI in the default browser.",
		Doc:     guiDoc,
	}
}

// SetFlags implements the cmd.Command interface.
func (c *guiCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.hideCreds, "hide-credential", false, "Do not show admin credential to use for logging into the Juju GUI")
	f.BoolVar(&c.showCreds, "show-credentials", true, "DEPRECATED. Show admin credential to use for logging into the Juju GUI")
	f.BoolVar(&c.noBrowser, "no-browser", true, "DEPRECATED. --no-browser is now the default. Use --browser to open the web browser")
	f.BoolVar(&c.browser, "browser", false, "Open the web browser, instead of just printing the Juju GUI URL")
}

func (c *guiCommand) guiVersions(conn api.Connection) ([]params.GUIArchiveVersion, error) {
	if c.getGUIVersions == nil {
		client := controller.NewClient(conn)
		return client.GUIArchives()
	}
	return c.getGUIVersions(conn)
}

// Run implements the cmd.Command interface.
func (c *guiCommand) Run(ctx *cmd.Context) error {
	// Retrieve model details.
	conn, err := c.NewControllerAPIRoot()
	if err != nil {
		return errors.Annotate(err, "cannot establish API connection")
	}
	defer conn.Close()

	store, ok := c.ClientStore().(modelcmd.QualifyingClientStore)
	if !ok {
		store = modelcmd.QualifyingClientStore{c.ClientStore()}
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	modelName, details, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve model details: please make sure you switched to a valid model")
	}

	// Make 2 URLs to try - the old and the new.
	rawURL := fmt.Sprintf("https://%s/gui/%s/", conn.Addr(), details.ModelUUID)
	qualifiedModelName, err := store.QualifiedModelName(controllerName, modelName)
	if err != nil {
		return errors.Annotate(err, "cannot construct model name")
	}
	// Do not include any possible "@external" fragment in the path.
	qualifiedModelName = strings.Replace(qualifiedModelName, "@external/", "/", 1)
	newRawURL := fmt.Sprintf("https://%s/gui/u/%s", conn.Addr(), qualifiedModelName)

	// Check that the Juju GUI is available.
	var guiURL string
	if guiURL, err = c.checkAvailable(rawURL, newRawURL, conn); err != nil {
		return errors.Trace(err)
	}

	// Get the GUI version to print.
	versions, err := c.guiVersions(conn)
	if err != nil {
		return errors.Trace(err)
	}
	var vers *version.Number
	for _, v := range versions {
		if v.Current {
			vers = &v.Version
			break
		}
	}

	// Open the Juju GUI in the browser.
	if err = c.openBrowser(ctx, guiURL, vers); err != nil {
		return errors.Trace(err)
	}

	// Print login credentials if requested.
	if err = c.showCredentials(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// checkAvailable ensures the Juju GUI is available on the controller at
// one of the given URLs, returning the successful URL.
func (c *guiCommand) checkAvailable(rawURL, newRawURL string, conn api.Connection) (string, error) {
	client, err := conn.HTTPClient()
	if err != nil {
		return "", errors.Annotate(err, "cannot retrieve HTTP client")
	}
	if err = clientGet(client, newRawURL); err == nil {
		return newRawURL, nil
	}
	if err = clientGet(client, rawURL); err != nil {
		return "", errors.Annotate(err, "Juju GUI is not available")
	}
	return rawURL, nil
}

// openBrowser opens the Juju GUI at the given URL.
func (c *guiCommand) openBrowser(ctx *cmd.Context, rawURL string, vers *version.Number) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.Annotate(err, "cannot parse Juju GUI URL")
	}
	if c.noBrowser && !c.browser {
		versInfo := ""
		if vers != nil {
			versInfo = fmt.Sprintf("%v ", vers)
		}
		modelName, err := c.ModelName()
		if err != nil {
			return errors.Trace(err)
		}
		ctx.Infof("GUI %sfor model %q is enabled at:\n  %s", versInfo, modelName, u.String())
		return nil
	}
	err = webbrowserOpen(u)
	if err == nil {
		ctx.Infof("Opening the Juju GUI in your browser.")
		ctx.Infof("If it does not open, open this URL:\n%s", u)
		return nil
	}
	if err == webbrowser.ErrNoBrowser {
		ctx.Infof("Open this URL in your browser:\n%s", u)
		return nil
	}
	return errors.Annotate(err, "cannot open web browser")
}

// showCredentials shows the admin username and password.
func (c *guiCommand) showCredentials(ctx *cmd.Context) error {
	if c.hideCreds || !c.showCreds {
		return nil
	}
	// TODO(wallyworld) - what to do if we are using a macaroon.
	accountDetails, err := c.CurrentAccountDetails()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve credentials")
	}
	password := accountDetails.Password
	if password == "" {
		// TODO(wallyworld) - fix this
		password = "<unknown> (password has been changed by the user)"
	}
	ctx.Infof("Your login credential is:\n  username: %s\n  password: %s", accountDetails.User, password)
	return nil
}

// clientGet is defined for testing purposes.
var clientGet = func(client *httprequest.Client, rawURL string) error {
	return client.Get(rawURL, nil)
}

// webbrowserOpen is defined for testing purposes.
var webbrowserOpen = webbrowser.Open
