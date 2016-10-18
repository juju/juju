// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"fmt"
	"net/url"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/httprequest"
	"github.com/juju/webbrowser"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewGUICommand creates and returns a new gui command.
func NewGUICommand() cmd.Command {
	return modelcmd.Wrap(&guiCommand{})
}

// guiCommand opens the Juju GUI in the default browser.
type guiCommand struct {
	modelcmd.ModelCommandBase

	showCreds bool
	noBrowser bool
}

const guiDoc = `
Open the Juju GUI in the default browser:

	juju gui

Open the GUI and show admin credentials to use to log into it:

	juju gui --show-credentials

Do not open the browser, just output the GUI URL:

	juju gui --no-browser

An error is returned if the Juju GUI is not available in the controller.
`

// Info implements the cmd.Command interface.
func (c *guiCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "gui",
		Purpose: "Open the Juju GUI in the default browser.",
		Doc:     guiDoc,
	}
}

// SetFlags implements the cmd.Command interface.
func (c *guiCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.showCreds, "show-credentials", false, "Show admin credentials to use for logging into the Juju GUI")
	f.BoolVar(&c.noBrowser, "no-browser", false, "Do not try to open the web browser, just print the Juju GUI URL")
}

// Run implements the cmd.Command interface.
func (c *guiCommand) Run(ctx *cmd.Context) error {
	// Retrieve model details.
	conn, err := c.NewAPIRoot()
	if err != nil {
		return errors.Annotate(err, "cannot establish API connection")
	}
	defer conn.Close()
	details, err := c.ClientStore().ModelByName(c.ControllerName(), c.ModelName())
	if err != nil {
		return errors.Annotate(err, "cannot retrieve model details")
	}
	rawURL := fmt.Sprintf("https://%s/gui/%s/", conn.Addr(), details.ModelUUID)

	// Check that the Juju GUI is available.
	if err = c.checkAvailable(rawURL, conn); err != nil {
		return errors.Trace(err)
	}

	// Open the Juju GUI in the browser.
	if err = c.openBrowser(ctx, rawURL); err != nil {
		return errors.Trace(err)
	}

	// Print login credentials if requested.
	if err = c.showCredentials(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// checkAvailable ensures the Juju GUI is available on the controller at the
// given URL.
func (c *guiCommand) checkAvailable(rawURL string, conn api.Connection) error {
	client, err := conn.HTTPClient()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve HTTP client")
	}
	if err = clientGet(client, rawURL); err != nil {
		return errors.Annotate(err, "Juju GUI is not available")
	}
	return nil
}

// openBrowser opens the Juju GUI at the given URL.
func (c *guiCommand) openBrowser(ctx *cmd.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.Annotate(err, "cannot parse Juju GUI URL")
	}
	if c.noBrowser {
		ctx.Infof(u.String())
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
	if !c.showCreds {
		return nil
	}
	// TODO(wallyworld) - what to do if we are using a macaroon.
	accountDetails, err := c.ClientStore().AccountDetails(c.ControllerName())
	if err != nil {
		return errors.Annotate(err, "cannot retrieve credentials")
	}
	ctx.Infof("Username: %s\nPassword: %s", accountDetails.User, accountDetails.Password)
	return nil
}

// clientGet is defined for testing purposes.
var clientGet = func(client *httprequest.Client, rawURL string) error {
	return client.Get(rawURL, nil)
}

// webbrowserOpen is defined for testing purposes.
var webbrowserOpen = webbrowser.Open
