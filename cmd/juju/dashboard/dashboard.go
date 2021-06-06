// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

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

// NewDashboardCommand creates and returns a new dashboard command.
func NewDashboardCommand() cmd.Command {
	return modelcmd.Wrap(&dashboardCommand{})
}

// dashboardCommand opens the Juju Dashboard in the default browser.
type dashboardCommand struct {
	modelcmd.ModelCommandBase

	hideCreds bool
	browser   bool

	getDashboardVersions func(connection api.Connection) ([]params.DashboardArchiveVersion, error)
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
func (c *dashboardCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "dashboard",
		Purpose: "Print the Juju Dashboard URL, or open the Juju Dashboard in the default browser.",
		Doc:     dashboardDoc,
	})
}

// SetFlags implements the cmd.Command interface.
func (c *dashboardCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.hideCreds, "hide-credential", false, "Do not show admin credential to use for logging into the Juju Dashboard")
	f.BoolVar(&c.browser, "browser", false, "Open the web browser, instead of just printing the Juju Dashboard URL")
}

func (c *dashboardCommand) dashboardVersions(conn api.Connection) ([]params.DashboardArchiveVersion, error) {
	if c.getDashboardVersions == nil {
		client := controller.NewClient(conn)
		return client.DashboardArchives()
	}
	return c.getDashboardVersions(conn)
}

// Run implements the cmd.Command interface.
func (c *dashboardCommand) Run(ctx *cmd.Context) error {
	// Retrieve model details.
	conn, err := c.NewControllerAPIRoot()
	if err != nil {
		return errors.Annotate(err, "cannot establish API connection")
	}
	defer conn.Close()

	// Check that the Juju Dashboard is available.
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	addr, ignoreCertError, err := dashboardAddr(conn)
	if err != nil {
		return errors.Annotatef(err,
			"getting dashboard address for controller %q",
			controllerName,
		)
	}
	dashboardURL := fmt.Sprintf("https://%s/dashboard", addr)
	if err = c.checkAvailable(conn, ignoreCertError, dashboardURL); err != nil {
		return errors.Trace(err)
	}

	// Get the Dashboard version to print.
	versions, err := c.dashboardVersions(conn)
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
	if err = c.openBrowser(ctx, "Dashboard", dashboardURL, vers); err != nil {
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

// dashboardAddr returns an address where the Dashboard is available.
func dashboardAddr(conn api.Connection) (string, bool, error) {
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

// checkAvailable ensures the Juju Dashboard is available on the controller at
// the given URL.
func (c *dashboardCommand) checkAvailable(conn api.Connection, ignoreCertError bool, URL string) error {
	client, err := conn.HTTPClient()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve HTTP client")
	}
	err = clientGet(c.StdContext, client, URL)
	// We don't have access to the http error code, but make a best effort to
	// handle a missing dashboard as opposed to a connection error
	if err != nil {
		if strings.Contains(err.Error(), "404 ") {
			return errors.New("Juju Dashboard is not available")
		}
		// TODO - fix this workaround for k8s clouds
		if ignoreCertError && strings.Contains(err.Error(), "x509: ") {
			return nil
		}
	}
	return errors.Annotate(err, "Juju Dashboard is not available")
}

// openBrowser opens the Juju Dashboard at the given URL.
func (c *dashboardCommand) openBrowser(ctx *cmd.Context, label, rawURL string, vers *version.Number) error {
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
func (c *dashboardCommand) showCredentials(ctx *cmd.Context) error {
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
