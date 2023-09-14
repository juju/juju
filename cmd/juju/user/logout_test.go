// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	cookiejar "github.com/juju/persistent-cookiejar"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/user"
)

type LogoutCommandSuite struct {
	BaseSuite
}

var _ = gc.Suite(&LogoutCommandSuite{})

func (s *LogoutCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *LogoutCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd, _ := user.NewLogoutCommandForTest(s.store)
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *LogoutCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		errorString string
	}{
		{
			// no args is fine
		}, {
			args:        []string{"foobar"},
			errorString: `unrecognized args: \["foobar"\]`,
		}, {
			args:        []string{"--foobar"},
			errorString: "option provided but not defined: --foobar",
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, _ := user.NewLogoutCommandForTest(s.store)
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *LogoutCommandSuite) TestLogout(c *gc.C) {
	cookiefile := filepath.Join(c.MkDir(), ".go-cookies")
	jar, err := cookiejar.New(&cookiejar.Options{Filename: cookiefile})
	c.Assert(err, jc.ErrorIsNil)
	cont, err := s.store.CurrentController()
	c.Assert(err, jc.ErrorIsNil)
	host := s.store.Controllers[cont].APIEndpoints[0]
	u, err := url.Parse("https://" + host)
	c.Assert(err, jc.ErrorIsNil)
	other, err := url.Parse("https://www.example.com")
	c.Assert(err, jc.ErrorIsNil)

	// we hav to set the expiration or it's not considered a "persistent"
	// cookie, and the jar won't save it.
	jar.SetCookies(u, []*http.Cookie{{
		Name:    "foo",
		Value:   "bar",
		Expires: time.Now().Add(time.Hour * 24)}})
	jar.SetCookies(other, []*http.Cookie{{
		Name:    "baz",
		Value:   "bat",
		Expires: time.Now().Add(time.Hour * 24)}})
	err = jar.Save()
	c.Assert(err, jc.ErrorIsNil)

	s.setPassword(c, "testing", "")
	ctx, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Logged out. You are no longer logged into any controllers.
`[1:],
	)
	_, err = s.store.AccountDetails("testing")
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	jar, err = cookiejar.New(&cookiejar.Options{Filename: cookiefile})
	c.Assert(err, jc.ErrorIsNil)
	cookies := jar.Cookies(other)
	c.Assert(cookies, gc.HasLen, 1)
}

func (s *LogoutCommandSuite) TestLogoutCount(c *gc.C) {
	// Create multiple controllers. We'll log out of each one
	// to observe the messages printed out by "logout".
	s.setPassword(c, "testing", "")
	controllers := []string{"testing", "testing2", "testing3"}
	details := s.store.Accounts["testing"]
	for _, controller := range controllers {
		s.store.Controllers[controller] = s.store.Controllers["testing"]
		err := s.store.UpdateAccount(controller, details)
		c.Assert(err, jc.ErrorIsNil)
	}

	expected := []string{
		"Logged out. You are still logged into 2 controllers.\n",
		"Logged out. You are still logged into 1 controller.\n",
		"Logged out. You are no longer logged into any controllers.\n",
	}

	for i, controller := range controllers {
		ctx, err := s.run(c, "-c", controller)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
		c.Assert(cmdtesting.Stderr(ctx), gc.Equals, expected[i])
	}
}

func (s *LogoutCommandSuite) TestLogoutWithPassword(c *gc.C) {
	s.assertStorePassword(c, "current-user", "old-password", "")
	_, err := s.run(c)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, `preventing account loss

It appears that you have not changed the password for
your account. If this is the case, change the password
first before logging out, so that you can log in again
afterwards. To change your password, run the command
"juju change-user-password".

If you are sure you want to log out, and it is safe to
clear the credentials from the client, then you can run
this command again with the "--force" option.
`)
}

func (s *LogoutCommandSuite) TestLogoutWithPasswordForced(c *gc.C) {
	s.assertStorePassword(c, "current-user", "old-password", "")
	_, err := s.run(c, "--force")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.store.AccountDetails("testing")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *LogoutCommandSuite) TestLogoutNotLoggedIn(c *gc.C) {
	delete(s.store.Accounts, "testing")
	ctx, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
Logged out. You are no longer logged into any controllers.
`[1:],
	)
}
