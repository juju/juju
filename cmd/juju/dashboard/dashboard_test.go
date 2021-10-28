// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard_test

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/webbrowser"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/dashboard"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type baseDashboardSuite struct {
	testing.BaseSuite

	controllerAPI *mockControllerAPI
	store         *jujuclient.MemStore
}

type mockControllerAPI struct {
	addresses []string
	tunnel    bool
	err       error
}

func (m *mockControllerAPI) DashboardAddresses() ([]string, bool, error) {
	return m.addresses, m.tunnel, m.err
}

func (m *mockControllerAPI) Close() error {
	return nil
}

// run executes the dashboard command passing the given args.
func (s *baseDashboardSuite) run(c *gc.C, args ...string) (string, error) {
	ctx, err := cmdtesting.RunCommand(c, dashboard.NewDashboardCommandForTest(s.store, s.controllerAPI), args...)
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

type dashboardSuite struct {
	baseDashboardSuite
}

var _ = gc.Suite(&dashboardSuite{})

func (s *dashboardSuite) SetUpTest(c *gc.C) {
	s.controllerAPI = &mockControllerAPI{
		addresses: []string{"10.1.1.1"},
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
	dashboardURL := "https://10.1.1.1"
	expectOut := "Opening the Juju Dashboard in your browser.\nIf it does not open, open this URL:\n" + dashboardURL
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
	c.Assert(out, gc.Equals, fmt.Sprintf(`
Dashboard for controller "kontroll" is enabled at:
  https://10.1.1.1`[1:]))
}

func (s *dashboardSuite) TestDashboardSuccessBrowserNotFound(c *gc.C) {
	s.patchBrowser(func(u *url.URL) error {
		return webbrowser.ErrNoBrowser
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	expectOut := "Open this URL in your browser:\nhttps://10.1.1.1"
	c.Assert(out, gc.Equals, expectOut)
}

func (s *dashboardSuite) TestDashboardErrorBrowser(c *gc.C) {
	s.patchBrowser(func(u *url.URL) error {
		return errors.New("bad wolf")
	})
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, "cannot open web browser: bad wolf")
	c.Assert(out, gc.Equals, "")
}

func (s *dashboardSuite) TestDashboardErrorUnavailable(c *gc.C) {
	s.controllerAPI.err = errors.NotFoundf("dashboard")
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, `
The Juju dashboard is not yet deployed.
To deploy the Juju dashboard follow these steps:
  juju switch controller
  juju deploy juju-dashboard
  juju expose juju-dashboard
  juju relate juju-dashboard controller
`[1:])
	c.Assert(out, gc.Equals, "")
}

func (s *dashboardSuite) TestDashboardError(c *gc.C) {
	s.controllerAPI.err = errors.New("bad wolf")
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, `getting dashboard address for controller "kontroll": bad wolf`)
	c.Assert(out, gc.Equals, "")
}
