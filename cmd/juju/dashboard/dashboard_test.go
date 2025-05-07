// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard_test

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/tc"
	"github.com/juju/webbrowser"

	"github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/cmd/juju/dashboard"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	proxytesting "github.com/juju/juju/internal/proxy/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type baseDashboardSuite struct {
	testing.BaseSuite

	controllerAPI *mockControllerAPI
	tunnelProxier *proxytesting.MockTunnelProxier
	store         *jujuclient.MemStore
	signalCh      chan os.Signal
	sshCmd        cmd.Command
}

type mockControllerAPI struct {
	info controller.DashboardConnectionInfo
	err  error
}

func (m *mockControllerAPI) DashboardConnectionInfo(_ context.Context, _ controller.ProxierFactory) (controller.DashboardConnectionInfo, error) {
	return m.info, m.err
}

func (m *mockControllerAPI) Close() error {
	return nil
}

// run executes the dashboard command passing the given args.
func (s *baseDashboardSuite) run(c *tc.C, args ...string) (string, error) {
	ctx, err := cmdtesting.RunCommand(c, dashboard.NewDashboardCommandForTest(s.store, s.controllerAPI, s.signalCh, s.sshCmd), args...)
	return strings.Trim(cmdtesting.Stderr(ctx), "\n"), err
}

func (s *baseDashboardSuite) patchBrowser(f func(*url.URL) error) {
	if f == nil {
		f = func(*url.URL) error {
			return nil
		}
	}
	s.PatchValue(dashboard.WebbrowserOpen, f)
}

func (s *baseDashboardSuite) sendInterrupt() {
	s.signalCh <- os.Interrupt
}

type dashboardSuite struct {
	baseDashboardSuite
}

var _ = tc.Suite(&dashboardSuite{})

func (s *dashboardSuite) SetUpTest(c *tc.C) {
	s.signalCh = make(chan os.Signal, 1)

	s.tunnelProxier = proxytesting.NewMockTunnelProxier()
	s.tunnelProxier.HostFn = func() string { return "10.1.1.1" }
	s.tunnelProxier.PortFn = func() string {
		defer s.sendInterrupt()
		return "6767"
	}

	s.controllerAPI = &mockControllerAPI{
		info: controller.DashboardConnectionInfo{
			Proxier: s.tunnelProxier,
		},
	}
	s.store = jujuclient.NewMemStore()
	s.store.Controllers["kontroll"] = jujuclient.ControllerDetails{}
	s.store.Models["kontroll"] = &jujuclient.ControllerModels{
		CurrentModel: "bar",
	}
	s.store.CurrentControllerName = "kontroll"
	s.store.Accounts["kontroll"] = jujuclient.AccountDetails{
		User:     "admin",
		Password: "s3kret!",
	}
}

func (s *dashboardSuite) TestDashboardSuccessWithBrowser(c *tc.C) {
	var browserURL string
	s.patchBrowser(func(u *url.URL) error {
		browserURL = u.String()
		return nil
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, tc.ErrorIsNil)
	dashboardURL := "http://10.1.1.1:6767"
	expectOut := "Opening the Juju Dashboard in your browser.\nIf it does not open, open this URL:\n" + dashboardURL + "\nReceived signal interrupt, stopping dashboard proxy connection"
	c.Assert(out, tc.Equals, expectOut)
	c.Assert(browserURL, tc.Equals, dashboardURL)
}

func (s *dashboardSuite) TestDashboardSuccessWithCredential(c *tc.C) {
	s.patchBrowser(nil)
	out, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.Contains, `
Your login credential is:
  username: admin
  password: s3kret!`[1:])
}

func (s *dashboardSuite) TestDashboardSuccessNoCredential(c *tc.C) {
	s.patchBrowser(nil)
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.Not(tc.Contains), "Password")
}

func (s *dashboardSuite) TestDashboardSuccessNoBrowser(c *tc.C) {
	// There is no need to patch the browser open function here.
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.Contains, `
Dashboard for controller "kontroll" is enabled at:
  http://10.1.1.1:6767`[1:])
}

func (s *dashboardSuite) TestDashboardSuccessBrowserNotFound(c *tc.C) {
	s.patchBrowser(func(u *url.URL) error {
		return webbrowser.ErrNoBrowser
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, tc.ErrorIsNil)
	expectOut := "Open this URL in your browser:\nhttp://10.1.1.1:6767"
	c.Assert(out, tc.Contains, expectOut)
}

func (s *dashboardSuite) TestDashboardErrorBrowser(c *tc.C) {
	s.patchBrowser(func(u *url.URL) error {
		return errors.New("bad wolf")
	})
	_, err := s.run(c, "--browser")
	c.Assert(err, tc.ErrorMatches, "cannot open web browser: bad wolf")
}

func (s *dashboardSuite) TestDashboardErrorUnavailable(c *tc.C) {
	s.controllerAPI.err = errors.NotFoundf("dashboard")
	_, err := s.run(c, "--browser")
	c.Assert(err, tc.ErrorMatches, `
The Juju dashboard is not yet deployed.
To deploy the Juju dashboard, follow these steps:
  juju switch controller
  juju deploy juju-dashboard
  juju expose juju-dashboard
  juju relate juju-dashboard controller
`[1:])
}

func (s *dashboardSuite) TestDashboardError(c *tc.C) {
	s.controllerAPI.err = errors.New("bad wolf")
	out, err := s.run(c, "--browser")
	c.Assert(err, tc.ErrorMatches, `getting dashboard address for controller "kontroll": bad wolf`)
	c.Assert(out, tc.Equals, "")
}

func (s *dashboardSuite) TestResolveSSHTarget(c *tc.C) {
	s.testResolveSSHTarget(c,
		&controller.DashboardConnectionSSHTunnel{
			Model:  "c:controller",
			Entity: "dashboard/leader",
			Host:   "10.35.42.151",
			Port:   "8080",
		},
		"c:controller",
		[]string{"dashboard/leader", "-N", "-L", "31666:10.35.42.151:8080"})
}

func (s *dashboardSuite) TestResolveSSHTargetLegacy(c *tc.C) {
	s.testResolveSSHTarget(c,
		&controller.DashboardConnectionSSHTunnel{
			Host: "10.35.42.151",
			Port: "8080",
		},
		"",
		[]string{"ubuntu@10.35.42.151", "-N", "-L", "31666:10.35.42.151:8080"})
}

func (s *dashboardSuite) testResolveSSHTarget(
	c *tc.C, sshTunnel *controller.DashboardConnectionSSHTunnel, model string, args []string) {

	s.controllerAPI = &mockControllerAPI{
		info: controller.DashboardConnectionInfo{
			SSHTunnel: sshTunnel,
		},
	}
	fakeSSHCmd := newFakeSSHCmd()
	s.sshCmd = fakeSSHCmd

	_, err := s.run(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fakeSSHCmd.model, tc.Equals, model)
	c.Check(fakeSSHCmd.args, tc.DeepEquals, args)
}

func newFakeSSHCmd() *fakeSSHCmd {
	return &fakeSSHCmd{}
}

type fakeSSHCmd struct {
	model string
	args  []string
}

func (c *fakeSSHCmd) IsSuperCommand() bool {
	panic("method shouldn't be called")
}

func (c *fakeSSHCmd) Info() *cmd.Info {
	panic("method shouldn't be called")
}

func (c *fakeSSHCmd) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.model, "m", "", "")
}

func (c *fakeSSHCmd) Init(args []string) error {
	c.args = args
	return nil
}

func (c *fakeSSHCmd) Run(ctx *cmd.Context) error {
	return nil
}

func (c *fakeSSHCmd) AllowInterspersedFlags() bool {
	panic("method shouldn't be called")
}
