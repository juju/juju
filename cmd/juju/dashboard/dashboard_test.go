// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard_test

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/webbrowser"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/api/controller/controller"
	"github.com/juju/juju/v3/cmd/juju/dashboard"
	"github.com/juju/juju/v3/jujuclient"
	proxytesting "github.com/juju/juju/v3/proxy/testing"
	"github.com/juju/juju/v3/testing"
)

type baseDashboardSuite struct {
	testing.BaseSuite

	controllerAPI *mockControllerAPI
	tunnelProxier *proxytesting.MockTunnelProxier
	store         *jujuclient.MemStore
	signalCh      chan os.Signal
}

type mockControllerAPI struct {
	info controller.DashboardConnectionInfo
	err  error
}

func (m *mockControllerAPI) DashboardConnectionInfo(_ controller.ProxierFactory) (controller.DashboardConnectionInfo, error) {
	return m.info, m.err
}

func (m *mockControllerAPI) Close() error {
	return nil
}

// run executes the dashboard command passing the given args.
func (s *baseDashboardSuite) run(c *gc.C, args ...string) (string, error) {
	ctx, err := cmdtesting.RunCommand(c, dashboard.NewDashboardCommandForTest(s.store, s.controllerAPI, s.signalCh), args...)
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

var _ = gc.Suite(&dashboardSuite{})

func (s *dashboardSuite) SetUpTest(c *gc.C) {
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

func (s *dashboardSuite) TestDashboardSuccessWithBrowser(c *gc.C) {
	var browserURL string
	s.patchBrowser(func(u *url.URL) error {
		browserURL = u.String()
		return nil
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	dashboardURL := "http://10.1.1.1:6767"
	expectOut := "Opening the Juju Dashboard in your browser.\nIf it does not open, open this URL:\n" + dashboardURL + "\nReceived signal interrupt, stopping dashboard proxy connection"
	c.Assert(out, gc.Equals, expectOut)
	c.Assert(browserURL, gc.Equals, dashboardURL)
}

func (s *dashboardSuite) TestDashboardSuccessWithCredential(c *gc.C) {
	s.patchBrowser(nil)
	out, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.Contains, `
Your login credential is:
  username: admin
  password: s3kret!`[1:])
}

func (s *dashboardSuite) TestDashboardSuccessNoCredential(c *gc.C) {
	s.patchBrowser(nil)
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Not(jc.Contains), "Password")
}

func (s *dashboardSuite) TestDashboardSuccessNoBrowser(c *gc.C) {
	// There is no need to patch the browser open function here.
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.Contains, fmt.Sprintf(`
Dashboard for controller "kontroll" is enabled at:
  http://10.1.1.1:6767`[1:]))
}

func (s *dashboardSuite) TestDashboardSuccessBrowserNotFound(c *gc.C) {
	s.patchBrowser(func(u *url.URL) error {
		return webbrowser.ErrNoBrowser
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	expectOut := "Open this URL in your browser:\nhttp://10.1.1.1:6767"
	c.Assert(out, jc.Contains, expectOut)
}

func (s *dashboardSuite) TestDashboardErrorBrowser(c *gc.C) {
	s.patchBrowser(func(u *url.URL) error {
		return errors.New("bad wolf")
	})
	_, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, "cannot open web browser: bad wolf")
}

func (s *dashboardSuite) TestDashboardErrorUnavailable(c *gc.C) {
	s.controllerAPI.err = errors.NotFoundf("dashboard")
	_, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, `
The Juju dashboard is not yet deployed.
To deploy the Juju dashboard follow these steps:
  juju switch controller
  juju deploy juju-dashboard
  juju expose juju-dashboard
  juju relate juju-dashboard controller
`[1:])
}

func (s *dashboardSuite) TestDashboardError(c *gc.C) {
	s.controllerAPI.err = errors.New("bad wolf")
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, `getting dashboard address for controller "kontroll": bad wolf`)
	c.Assert(out, gc.Equals, "")
}
