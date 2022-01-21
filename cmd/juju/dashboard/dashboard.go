// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard

import (
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/signal"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/webbrowser"

	"github.com/juju/juju/api/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/ssh"
	"github.com/juju/juju/cmd/modelcmd"
)

// ControllerAPI is used to get dashboard info from the controller.
type ControllerAPI interface {
	DashboardAddresses() ([]string, bool, error)
	Close() error
}

// NewDashboardCommand creates and returns a new dashboard command.
func NewDashboardCommand() cmd.Command {
	d := &dashboardCommand{}
	d.newAPIFunc = func() (ControllerAPI, bool, error) {
		return d.newControllerAPI()
	}
	d.embeddedSSHCmd = ssh.NewSSHCommand(nil, nil, ssh.DefaultSSHRetryStrategy)
	return modelcmd.Wrap(d)
}

// dashboardCommand opens the Juju Dashboard in the default browser.
type dashboardCommand struct {
	modelcmd.ModelCommandBase

	hideCreds bool
	browser   bool

	newAPIFunc func() (ControllerAPI, bool, error)

	port           int
	embeddedSSHCmd cmd.Command
}

func (c *dashboardCommand) newControllerAPI() (ControllerAPI, bool, error) {
	root, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return controller.NewClient(root), root.IsProxied(), nil
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

An error is returned if the Juju Dashboard is not running.
`

const dashboardNotAvailableMessage = `The Juju dashboard is not yet deployed.
To deploy the Juju dashboard follow these steps:
  juju switch controller
  juju deploy juju-dashboard
  juju expose juju-dashboard
  juju relate juju-dashboard controller
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
	f.IntVar(&c.port, "port", 31666, "Local port used to serve the dashboard")
	f.BoolVar(&c.hideCreds, "hide-credential", false, "Do not show admin credential to use for logging into the Juju Dashboard")
	f.BoolVar(&c.browser, "browser", false, "Open the web browser, instead of just printing the Juju Dashboard URL")
}

// Run implements the cmd.Command interface.
func (c *dashboardCommand) Run(ctx *cmd.Context) error {
	api, isProxied, err := c.newAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = api.Close() }()

	// Check that the Juju Dashboard is available.
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	actualDashboardAddresses, useTunnel, err := api.DashboardAddresses()
	if err != nil {
		if errors.IsNotFound(err) {
			return errors.New(dashboardNotAvailableMessage)
		}
		return errors.Annotatef(err,
			"getting dashboard address for controller %q",
			controllerName,
		)
	}

	// Pick a random dashboard address.
	i := rand.Intn(len(actualDashboardAddresses))
	dashboardAddress := actualDashboardAddresses[i]
	dashboardURL := fmt.Sprintf("https://%s", dashboardAddress)

	var errCh <-chan error
	if useTunnel {
		errCh, err = c.openTunnel(dashboardAddress)
		if err != nil {
			return errors.Trace(err)
		}
		dashboardURL = fmt.Sprintf("http://localhost:%d", c.port)
	}
	// TODO(wallyworld) - support k8s dashboard charm properly
	if isProxied {
		//addr, err := proxierAddr(conn.Proxy())
		//if err != nil {
		//	return errors.Trace(err)
		//}
		//dashboardURL = fmt.Sprintf("http://%s", addr)
		return errors.NotSupportedf("k8s dashboard")
	}

	// Open the Juju Dashboard in the browser.
	if err = c.openBrowser(ctx, "Dashboard", dashboardURL); err != nil {
		return errors.Trace(err)
	}

	// Print login credentials if requested.
	if err = c.showCredentials(ctx); err != nil {
		return errors.Trace(err)
	}

	if !useTunnel && !isProxied {
		return nil
	}

	ctx.Infof("The dashboard connection for controller %q requires a proxied "+
		"connection. This command will hold the proxy connection open until "+
		"an interrupt or kill signal is sent.",
		controllerName,
	)

	signalCh := make(chan os.Signal)
	signal.Notify(signalCh, os.Interrupt, os.Kill)
	select {
	case waitSig := <-signalCh:
		ctx.Infof("Received signal %s, stopping dashboard proxy connection", waitSig)
	case err := <-errCh:
		return errors.Annotate(err, "error opening ssh tunnel")
	}
	return nil
}

// openTunnel creates a tunnel from localhost to the dashboard.
func (c *dashboardCommand) openTunnel(dashboardAddress string) (<-chan error, error) {
	host, _, err := net.SplitHostPort(dashboardAddress)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := c.embeddedSSHCmd.Init([]string{
		"ubuntu@" + host,
		"-N",
		"-L",
		fmt.Sprintf("%d:%s", c.port, dashboardAddress),
	}); err != nil {
		return nil, errors.Trace(err)
	}
	errCh := make(chan error)
	go func() {
		// TODO(wallyworld) - extract the core ssh machinery and use directly.
		ctx, _ := cmd.DefaultContext()
		errCh <- c.embeddedSSHCmd.Run(ctx)
	}()

	return errCh, nil
}

// openBrowser opens the Juju Dashboard at the given URL.
func (c *dashboardCommand) openBrowser(ctx *cmd.Context, label, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.Annotatef(err, "cannot parse Juju %s URL", label)
	}
	if !c.browser {
		controllerName, err := c.ControllerName()
		if err != nil {
			return errors.Trace(err)
		}
		ctx.Infof("%s for controller %q is enabled at:\n  %s", label, controllerName, u.String())
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

// webbrowserOpen is defined for testing purposes.
var webbrowserOpen = webbrowser.Open
