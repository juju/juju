// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui_test

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/juju/httprequest"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	"github.com/juju/webbrowser"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/gui"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

type guiSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&guiSuite{})

// run executes the gui command passing the given args.
func (s *guiSuite) run(c *gc.C, args ...string) (string, error) {
	ctx, err := coretesting.RunCommand(c, gui.NewGUICommandForTest(
		func(connection api.Connection) ([]params.GUIArchiveVersion, error) {
			return []params.GUIArchiveVersion{
				{
					Version: version.MustParse("1.2.3"),
					Current: false,
				}, {
					Version: version.MustParse("4.5.6"),
					Current: true,
				},
			}, nil
		}), args...)
	return strings.Trim(coretesting.Stderr(ctx), "\n"), err
}

func (s *guiSuite) patchClient(f func(*httprequest.Client, string) error) {
	if f == nil {
		f = func(*httprequest.Client, string) error {
			return nil
		}
	}
	s.PatchValue(gui.ClientGet, f)
}

func (s *guiSuite) patchBrowser(f func(*url.URL) error) {
	if f == nil {
		f = func(*url.URL) error {
			return nil
		}
	}
	s.PatchValue(gui.WebbrowserOpen, f)
}

func (s *guiSuite) guiURL(c *gc.C) string {
	info := s.APIInfo(c)
	return fmt.Sprintf("https://%s/gui/%s/", info.Addrs[0], info.ModelTag.Id())
}

func (s *guiSuite) TestGUISuccessWithBrowser(c *gc.C) {
	var clientURL, browserURL string
	s.patchClient(func(client *httprequest.Client, u string) error {
		clientURL = u
		return nil
	})
	s.patchBrowser(func(u *url.URL) error {
		browserURL = u.String()
		return nil
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	guiURL := s.guiURL(c)
	expectOut := "Opening the Juju GUI in your browser.\nIf it does not open, open this URL:\n" + guiURL
	c.Assert(out, gc.Equals, expectOut)
	c.Assert(clientURL, gc.Equals, guiURL)
	c.Assert(browserURL, gc.Equals, guiURL)
}

func (s *guiSuite) TestGUISuccessWithCredential(c *gc.C) {
	s.patchClient(nil)
	s.patchBrowser(nil)
	out, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.Contains, `
Your login credential is:
  username: admin
  password: dummy-secret`[1:])
}

func (s *guiSuite) TestGUISuccessNoCredential(c *gc.C) {
	s.patchClient(nil)
	s.patchBrowser(nil)
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Not(jc.Contains), "Password")
}

func (s *guiSuite) TestGUISuccessNoBrowser(c *gc.C) {
	s.patchClient(nil)
	// There is no need to patch the browser open function here.
	out, err := s.run(c, "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
GUI 4.5.6 for model controller is enabled at:
  %s`[1:], s.guiURL(c)))
}

func (s *guiSuite) TestGUISuccessNoBrowserDeprecated(c *gc.C) {
	s.patchClient(nil)
	// There is no need to patch the browser open function here.
	out, err := s.run(c, "--no-browser", "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.Equals, fmt.Sprintf(`
GUI 4.5.6 for model controller is enabled at:
  %s`[1:], s.guiURL(c)))
}

func (s *guiSuite) TestGUISuccessBrowserNotFound(c *gc.C) {
	s.patchClient(nil)
	s.patchBrowser(func(u *url.URL) error {
		return webbrowser.ErrNoBrowser
	})
	out, err := s.run(c, "--browser", "--hide-credential")
	c.Assert(err, jc.ErrorIsNil)
	expectOut := "Open this URL in your browser:\n" + s.guiURL(c)
	c.Assert(out, gc.Equals, expectOut)
}

func (s *guiSuite) TestGUIErrorBrowser(c *gc.C) {
	s.patchClient(nil)
	s.patchBrowser(func(u *url.URL) error {
		return errors.New("bad wolf")
	})
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, "cannot open web browser: bad wolf")
	c.Assert(out, gc.Equals, "")
}

func (s *guiSuite) TestGUIErrorUnavailable(c *gc.C) {
	s.patchClient(func(client *httprequest.Client, u string) error {
		return errors.New("bad wolf")
	})
	out, err := s.run(c, "--browser")
	c.Assert(err, gc.ErrorMatches, "Juju GUI is not available: bad wolf")
	c.Assert(out, gc.Equals, "")
}
