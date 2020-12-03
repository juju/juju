// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dashboard_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	"github.com/juju/webbrowser"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/dashboard"
	jujutesting "github.com/juju/juju/juju/testing"
)

type baseDashboardSuite struct {
	jujutesting.JujuConnSuite
}

// run executes the dashboard command passing the given args.
func (s *baseDashboardSuite) run(c *gc.C, args ...string) (string, error) {
	ctx, err := cmdtesting.RunCommand(c, dashboard.NewDashboardCommandForTest(
		func(connection api.Connection) ([]params.DashboardArchiveVersion, error) {
			return []params.DashboardArchiveVersion{
				{
					Version: version.MustParse("1.2.3"),
					Current: false,
				}, {
					Version: version.MustParse("4.5.6"),
					Current: true,
				},
			}, nil
		}), args...)
	return strings.Trim(cmdtesting.Stderr(ctx), "\n"), err
}

func (s *baseDashboardSuite) patchClient(f func(context.Context, *httprequest.Client, string) error) {
	if f == nil {
		f = func(context.Context, *httprequest.Client, string) error {
			return nil
		}
	}
	s.PatchValue(dashboard.ClientGet, f)
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

func (s *dashboardSuite) dashboardURL(c *gc.C) string {
	info := s.APIInfo(c)
	return fmt.Sprintf("https://%s/dashboard", info.Addrs[0])
}

func (s *dashboardSuite) TestDashboardSuccessWithBrowser(c *gc.C) {
	var clientURL, browserURL string
	s.patchClient(func(_ context.Context, client *httprequest.Client, u string) error {
		clientURL = u
		return nil
	})
	s.patchBrowser(func(u *url.URL) error {
		browserURL = u.String()
		return nil
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	dashboardURL := s.dashboardURL(c)
	expectOut := "Opening the Juju Dashboard in your browser.\nIf it does not open, open this URL:\n" + dashboardURL
	c.Assert(out, gc.Equals, expectOut)
	c.Assert(clientURL, gc.Equals, dashboardURL)
	c.Assert(browserURL, gc.Equals, dashboardURL)
}

func (s *dashboardSuite) TestDashboardSuccessWithCredential(c *gc.C) {
	s.patchClient(nil)
	s.patchBrowser(nil)
	out, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.Contains, `
Your login credential is:
  username: admin
  password: dummy-secret`[1:])
}

func (s *dashboardSuite) TestDashboardSuccessNoCredential(c *gc.C) {
	s.patchClient(nil)
	s.patchBrowser(nil)
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Not(jc.Contains), "Password")
}

func (s *dashboardSuite) TestDashboardSuccessNoBrowser(c *gc.C) {
	s.patchClient(nil)
	// There is no need to patch the browser open function here.
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
Dashboard 4.5.6 for controller "kontroll" is enabled at:
  %s`[1:], s.dashboardURL(c)))
}

func (s *dashboardSuite) TestDashboardSuccessBrowserNotFound(c *gc.C) {
	s.patchClient(nil)
	s.patchBrowser(func(u *url.URL) error {
		return webbrowser.ErrNoBrowser
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	expectOut := "Open this URL in your browser:\n" + s.dashboardURL(c)
	c.Assert(out, gc.Equals, expectOut)
}

func (s *dashboardSuite) TestDashboardErrorBrowser(c *gc.C) {
	s.patchClient(nil)
	s.patchBrowser(func(u *url.URL) error {
		return errors.New("bad wolf")
	})
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, "cannot open web browser: bad wolf")
	c.Assert(out, gc.Equals, "")
}

func (s *dashboardSuite) TestDashboardErrorUnavailable(c *gc.C) {
	s.patchClient(func(_ context.Context, client *httprequest.Client, u string) error {
		return errors.New("404 not found")
	})
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, "Juju Dashboard is not available")
	c.Assert(out, gc.Equals, "")
}

func (s *dashboardSuite) TestDashboardError(c *gc.C) {
	s.patchClient(func(_ context.Context, client *httprequest.Client, u string) error {
		return errors.New("bad wolf")
	})
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, "Juju Dashboard is not available: bad wolf")
	c.Assert(out, gc.Equals, "")
}

type dashboardDNSSuite struct {
	baseDashboardSuite
}

var _ = gc.Suite(&dashboardDNSSuite{
	baseDashboardSuite: baseDashboardSuite{
		JujuConnSuite: jujutesting.JujuConnSuite{
			ControllerConfigAttrs: map[string]interface{}{
				"api-port":          443,
				"autocert-dns-name": "example.com",
			},
		},
	},
})

func (s *dashboardDNSSuite) TestDashboardSuccess(c *gc.C) {
	s.patchClient(nil)
	// There is no need to patch the browser open function here.
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, `
Dashboard 4.5.6 for controller "kontroll" is enabled at:
  https://example.com/dashboard`[1:])
}
