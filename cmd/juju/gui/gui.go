// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"context"
	"fmt"
	"net/url"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/version"
	"github.com/juju/webbrowser"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewGUICommand creates and returns a new dashboard command.
func NewGUICommand() cmd.Command {
	return modelcmd.Wrap(&guiCommand{})
}

// guiCommand opens the Juju Dashboard in the default browser.
type guiCommand struct {
	modelcmd.ModelCommandBase

	hideCreds bool
	browser   bool

	getGUIVersions func(connection api.Connection) ([]params.GUIArchiveVersion, error)
}

const dashboardDoc = `
Print the Juju Dashboard URL and show admin credential to use to log into it:

	juju dashboard

Print the Juju Dashboard URL only:

	juju dashboard --hide-credential

Open the Juju Dashboard in the default browser and show admin credential to use to log into it:

	juju dashboard --browser

Open the Juju Dashboard in the default browser without printing the login credential:

	juju dashboard --hide-credential --browser

An error is returned if the Juju Dashboard is not available in the controller.
`

// Info implements the cmd.Command interface.
func (c *guiCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "dashboard",
		Purpose: "Print the Juju Dashboard URL, or open the Juju Dashboard in the default browser.",
		Doc:     dashboardDoc,
		Aliases: []string{"gui"},
	})
}

// SetFlags implements the cmd.Command interface.
func (c *guiCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.hideCreds, "hide-credential", false, "Do not show admin credential to use for logging into the Juju GUI")
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

	addr := guiAddr(conn)
	rawURL := fmt.Sprintf("https://%s/dashboard", addr)

	// Check that the Juju Dashboard is available.
	var dashboardURL string
	if dashboardURL, err = c.checkAvailable(rawURL, conn); err != nil {
		return errors.Trace(err)
	}

	// Get the Dashboard version to print.
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

	// Open the Juju Dashboard in the browser.
	if err = c.openBrowser(ctx, dashboardURL, vers); err != nil {
		return errors.Trace(err)
	}

	// Print login credentials if requested.
	if err = c.showCredentials(ctx); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// guiAddr returns an address where the GUI is available.
func guiAddr(conn api.Connection) string {
	if dnsName := conn.PublicDNSName(); dnsName != "" {
		return dnsName
	}
	return conn.Addr()
}

// checkAvailable ensures the Juju Dashboard is available on the controller at
// one of the given URLs, returning the successful URL.
func (c *guiCommand) checkAvailable(rawURL string, conn api.Connection) (string, error) {
	client, err := conn.HTTPClient()
	if err != nil {
		return "", errors.Annotate(err, "cannot retrieve HTTP client")
	}
	if err = clientGet(c.StdContext, client, rawURL); err != nil {
		return "", errors.Annotate(err, "Juju Dashboard is not available")
	}
	return rawURL, nil
}

// openBrowser opens the Juju Dashboard at the given URL.
func (c *guiCommand) openBrowser(ctx *cmd.Context, rawURL string, vers *version.Number) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.Annotate(err, "cannot parse Juju Dashboard URL")
	}
	if !c.browser {
		versInfo := ""
		if vers != nil {
			versInfo = fmt.Sprintf("%v", vers)
		}
		controllerName, err := c.ControllerName()
		if err != nil {
			return errors.Trace(err)
		}
		ctx.Infof("Dashboard %s for controller %q is enabled at:\n  %s", versInfo, controllerName, u.String())
		return nil
	}
	err = webbrowserOpen(u)
	if err == nil {
		ctx.Infof("Opening the Juju Dashboard in your browser.")
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
	if c.hideCreds {
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
	ctx.Infof("Your login credentials are:\n  username: %s\n  password: %s", accountDetails.User, password)
	return nil
}

// clientGet is defined for testing purposes.
var clientGet = func(ctx context.Context, client *httprequest.Client, rawURL string) error {
	return client.Get(ctx, rawURL, nil)
}

// webbrowserOpen is defined for testing purposes.
var webbrowserOpen = webbrowser.Open
