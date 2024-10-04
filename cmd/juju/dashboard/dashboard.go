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
	"strconv"
	"sync"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/webbrowser"

	"github.com/juju/juju/api/controller/controller"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/ssh"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/proxy"
	proxyfactory "github.com/juju/juju/internal/proxy/factory"
)

// ControllerAPI is used to get dashboard info from the controller.
type ControllerAPI interface {
	DashboardConnectionInfo(context.Context, controller.ProxierFactory) (controller.DashboardConnectionInfo, error)
	Close() error
}

// NewDashboardCommand creates and returns a new dashboard command.
func NewDashboardCommand() cmd.Command {
	d := &dashboardCommand{}
	d.newAPIFunc = func(ctx context.Context) (ControllerAPI, bool, error) {
		return d.newControllerAPI(ctx)
	}
	d.embeddedSSHCmd = ssh.NewSSHCommand(nil, nil, ssh.DefaultSSHRetryStrategy, ssh.DefaultSSHPublicKeyRetryStrategy)
	d.signalCh = make(chan os.Signal)
	return modelcmd.Wrap(d)
}

// dashboardCommand opens the Juju Dashboard in the default browser.
type dashboardCommand struct {
	modelcmd.ModelCommandBase

	hideCreds bool
	browser   bool

	newAPIFunc func(ctx context.Context) (ControllerAPI, bool, error)

	port           int
	embeddedSSHCmd cmd.Command
	signalCh       chan os.Signal
}

type urlCallBack func(url string)
type connectionRunner func(ctx context.Context, callBack urlCallBack) error

func (c *dashboardCommand) newControllerAPI(ctx context.Context) (ControllerAPI, bool, error) {
	root, err := c.NewControllerAPIRoot(ctx)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return controller.NewClient(root), root.IsProxied(), nil
}

const dashboardDoc = `
`

const dashboardExamples = `
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
To deploy the Juju dashboard, follow these steps:
  juju switch controller
  juju deploy juju-dashboard
  juju expose juju-dashboard
  juju relate juju-dashboard controller
`

// Info implements the cmd.Command interface.
func (c *dashboardCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "dashboard",
		Purpose:  "Print the Juju Dashboard URL, or open the Juju Dashboard in the default browser.",
		Doc:      dashboardDoc,
		Examples: dashboardExamples,
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
	api, _, err := c.newAPIFunc(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = api.Close() }()

	// Check that the Juju Dashboard is available.
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}

	factory, err := proxyfactory.NewDefaultFactory()
	if err != nil {
		return errors.Annotate(err, "creating default proxy factory to support dashboard connection")
	}

	res, err := api.DashboardConnectionInfo(ctx, factory)
	if errors.Is(err, errors.NotFound) {
		return errors.New(dashboardNotAvailableMessage)
	} else if err != nil {
		return errors.Annotatef(err,
			"getting dashboard address for controller %q",
			controllerName,
		)
	}

	var runner connectionRunner

	if res.Proxier != nil {
		tunnelProxy, ok := res.Proxier.(proxy.TunnelProxier)
		if !ok {
			return errors.Annotatef(err, "unsupported proxy type %q for dashboard", res.Proxier.Type())
		}

		runner = tunnelProxyRunner(ctx, tunnelProxy)
	} else if res.SSHTunnel != nil {
		runner = tunnelSSHRunner(*res.SSHTunnel, c.port, c.embeddedSSHCmd)
	} else {
		return errors.NotValidf("dashboard connection has no proxying or ssh connection information")
	}

	urlCh := make(chan string)
	defer close(urlCh)
	runnerURLCallBack := func(url string) {
		urlCh <- url
	}

	stdctx, cancel := context.WithCancel(context.Background())
	cancelOnce := sync.Once{}
	defer cancelOnce.Do(cancel)
	finishCh := make(chan error)
	go func() {
		defer close(finishCh)
		err := runner(stdctx, runnerURLCallBack)
		finishCh <- errors.Annotate(err, "running connection runner")
	}()

	// We need to wait for either the runner to blow up or tell us wha the
	// dashboard url is before processing the os signals
	var userErr error
	select {
	case url := <-urlCh:
		if userErr = c.openBrowser(ctx, "Dashboard", url); userErr != nil {
			cancelOnce.Do(cancel)
			break
		}
		if userErr = c.showCredentials(ctx); userErr != nil {
			cancelOnce.Do(cancel)
		}
	case err, ok := <-finishCh:
		if ok {
			return errors.Trace(err)
		}
		return nil
	}

	signal.Notify(c.signalCh, os.Interrupt, os.Kill)
	for {
		select {
		case waitSig := <-c.signalCh:
			ctx.Infof("Received signal %s, stopping dashboard proxy connection", waitSig)
			cancelOnce.Do(cancel)
		case err, ok := <-finishCh:
			if ok && err != nil {
				return errors.Wrap(userErr, err)
			}
			return userErr
		}
	}
}

func tunnelSSHRunner(
	tunnel controller.DashboardConnectionSSHTunnel,
	localPort int,
	sshCommand cmd.Command,
) connectionRunner {

	args := []string{}
	if tunnel.Entity == "" || tunnel.Model == "" {
		// Backwards compatibility with 3.0.0 controllers that only provide IP address
		args = append(args, "ubuntu@"+tunnel.Host)
	} else {
		args = append(args, "-m", tunnel.Model, tunnel.Entity)
	}
	args = append(args, "-N", "-L",
		fmt.Sprintf("%d:%s", localPort, net.JoinHostPort(tunnel.Host, tunnel.Port)))

	return func(ctx context.Context, callBack urlCallBack) error {
		f := &gnuflag.FlagSet{}
		sshCommand.SetFlags(f)
		err := f.Parse(false, args)
		if err != nil {
			return errors.Trace(err)
		}

		if err := sshCommand.Init(f.Args()); err != nil {
			return errors.Trace(err)
		}

		u := url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort("localhost", strconv.Itoa(localPort)),
		}
		callBack(u.String())

		// TODO(wallyworld) - extract the core ssh machinery and use directly.
		defCtx, err := cmd.DefaultContext()
		if err != nil {
			return errors.Trace(err)
		}
		cmdCtx := defCtx.With(ctx)
		return sshCommand.Run(cmdCtx)
	}
}

func tunnelProxyRunner(ctx context.Context, p proxy.TunnelProxier) connectionRunner {
	return func(ctx context.Context, callBack urlCallBack) error {
		if err := p.Start(ctx); err != nil {
			return errors.Annotate(err, "starting tunnel proxy")
		}
		defer p.Stop()

		u := url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(p.Host(), p.Port()),
		}
		callBack(u.String())
		select {
		case <-ctx.Done():
		}
		return nil
	}
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
