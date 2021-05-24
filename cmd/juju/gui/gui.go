// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"reflect"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/version/v2"
	"github.com/juju/webbrowser"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/proxy"
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
	f.BoolVar(&c.hideCreds, "hide-credential", false, "Do not show admin credential to use for logging into the Juju Dashboard")
	f.BoolVar(&c.browser, "browser", false, "Open the web browser, instead of just printing the Juju Dashboard URL")
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
		store = modelcmd.QualifyingClientStore{
			ClientStore: c.ClientStore(),
		}
	}
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	modelName, details, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve model details: please make sure you switched to a valid model")
	}

	// Make 2 GUI URLs to try - the old and the new.
	addr, ignoreCertError, err := guiAddr(conn)
	if err != nil {
		return errors.Annotatef(err,
			"getting dashboard address for controller %q",
			controllerName,
		)
	}

	rawGUIURL := fmt.Sprintf("https://%s/gui/%s/", addr, details.ModelUUID)
	qualifiedModelName, err := store.QualifiedModelName(controllerName, modelName)
	if err != nil {
		return errors.Annotate(err, "cannot construct model name")
	}
	// Do not include any possible "@external" fragment in the path.
	qualifiedModelName = strings.Replace(qualifiedModelName, "@external/", "/", 1)
	newRawGUIURL := fmt.Sprintf("https://%s/gui/u/%s", addr, qualifiedModelName)

	rawDashboardURL := fmt.Sprintf("https://%s/dashboard", addr)

	// Check that the Juju Dashboard (or a legacy GUI) is available.
	var dashboardURL string
	if dashboardURL, err = c.checkAvailable(conn, ignoreCertError, rawDashboardURL, newRawGUIURL, rawGUIURL); err != nil {
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
	label := "Dashboard"
	if dashboardURL != rawDashboardURL {
		label = "GUI"
	}
	if err = c.openBrowser(ctx, label, dashboardURL, vers); err != nil {
		return errors.Trace(err)
	}

	// Print login credentials if requested.
	if err = c.showCredentials(ctx); err != nil {
		return errors.Trace(err)
	}

	if !conn.IsProxied() {
		return nil
	}

	ctx.Infof("The dashboard connection for controller %q requires a proxied "+
		"connection. This command will hold the proxy connection open until "+
		"an interrupt or kill signal is sent.",
		controllerName,
	)

	signalCh := make(chan os.Signal)
	signal.Notify(signalCh, os.Interrupt, os.Kill)
	waitSig := <-signalCh

	ctx.Infof("Received signal %s, stopping dashboard proxy connection", waitSig)
	return nil
}

// guiAddr returns an address where the Dashboard is available.
func guiAddr(conn api.Connection) (string, bool, error) {
	if dnsName := conn.PublicDNSName(); dnsName != "" {
		return dnsName, false, nil
	}
	// The CLI k8s clouds connect via a proxy running on localhost.
	// The dashboard still needs to go via the controller IP.
	// TODO - this is a temporary workaround which will not work
	// on k8s clouds like minikube, kind etc.
	isLocal := func(host string) bool {
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	addr := conn.Addr()
	if conn.IsProxied() {
		var err error
		addr, err = proxierAddr(conn.Proxy())
		if err != nil {
			return "", false, err
		}
		return addr, false, nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil || !isLocal(host) {
		return addr, false, nil
	}
	for _, hps := range conn.APIHostPorts() {
		for _, hp := range hps {
			if host := hp.Host(); !isLocal(host) {
				return hp.String(), true, nil
			}
		}
	}
	return addr, false, nil
}

func proxierAddr(proxier proxy.Proxier) (string, error) {
	switch p := proxier.(type) {
	case proxy.TunnelProxier:
		return fmt.Sprintf("%s:%s", p.Host(), p.Port()), nil
	default:
		return "", errors.NotImplementedf(
			"cannot extract proxy address for proxy type %s",
			reflect.TypeOf(proxier))
	}
}

// checkAvailable ensures the Juju Dashboard/GUI is available on the controller at
// one of the given URLs, returning the successful URL.
func (c *guiCommand) checkAvailable(conn api.Connection, ignoreCertError bool, rawURLs ...string) (string, error) {
	client, err := conn.HTTPClient()
	if err != nil {
		return "", errors.Annotate(err, "cannot retrieve HTTP client")
	}
	var firstErr error
	for _, URL := range rawURLs {
		if err = clientGet(c.StdContext, client, URL); err == nil {
			return URL, nil
		}
		// TODO - fix this workaround for k8s clouds
		if ignoreCertError && strings.Contains(err.Error(), "x509: ") {
			return URL, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	// We don't have access to the http error code, but make a best effort to
	// handle a missing dashboard as opposed to a connection error
	if firstErr != nil && strings.Contains(firstErr.Error(), "404 ") {
		return "", errors.New("Juju Dashboard is not available")
	}
	return "", errors.Annotate(firstErr, "Juju Dashboard is not available")
}

// openBrowser opens the Juju Dashboard at the given URL.
func (c *guiCommand) openBrowser(ctx *cmd.Context, label, rawURL string, vers *version.Number) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.Annotatef(err, "cannot parse Juju %s URL", label)
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
		ctx.Infof("%s %s for controller %q is enabled at:\n  %s", label, versInfo, controllerName, u.String())
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
	ctx.Infof("Your login credential is:\n  username: %s\n  password: %s", accountDetails.User, password)
	return nil
}

// clientGet is defined for testing purposes.
var clientGet = func(ctx context.Context, client *httprequest.Client, rawURL string) error {
	return client.Get(ctx, rawURL, nil)
}

// webbrowserOpen is defined for testing purposes.
var webbrowserOpen = webbrowser.Open
