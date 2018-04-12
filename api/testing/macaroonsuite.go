// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

// MacaroonSuite wraps a JujuConnSuite with macaroon authentication
// enabled.
type MacaroonSuite struct {
	jujutesting.JujuConnSuite

	// discharger holds the third-party discharger used
	// for authentication.
	discharger *bakerytest.Discharger

	// DischargerLogin is called by the discharger when an
	// API macaroon is discharged. It should either return
	// the chosen username or an empty string, in which case
	// the discharge is denied.
	// If this is nil, func() {return ""} is implied.
	DischargerLogin func() string
}

func (s *MacaroonSuite) SetUpTest(c *gc.C) {
	s.DischargerLogin = nil
	s.discharger = bakerytest.NewDischarger(nil, func(req *http.Request, cond, arg string) ([]checkers.Caveat, error) {
		if cond != "is-authenticated-user" {
			return nil, errors.New("unknown caveat")
		}
		var username string
		if s.DischargerLogin != nil {
			username = s.DischargerLogin()
		}
		if username == "" {
			return nil, errors.New("login denied by discharger")
		}
		return []checkers.Caveat{checkers.DeclaredCaveat("username", username)}, nil
	})
	s.JujuConnSuite.ControllerConfigAttrs = map[string]interface{}{
		controller.IdentityURL: s.discharger.Location(),
	}
	s.JujuConnSuite.SetUpTest(c)
}

func (s *MacaroonSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.JujuConnSuite.TearDownTest(c)
}

// DischargerLocation returns the URL of the third party caveat
// discharger.
func (s *MacaroonSuite) DischargerLocation() string {
	return s.discharger.Location()
}

// AddModelUser is a convenience function that adds an external
// user to the current model. It will panic
// if the user name is local.
func (s *MacaroonSuite) AddModelUser(c *gc.C, username string) {
	if names.NewUserTag(username).IsLocal() {
		panic("cannot use MacaroonSuite.AddModelUser to add a local name")
	}
	s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User: username,
	})
}

// AddControllerUser is a convenience funcation that adds
// a controller user with the specified access.
func (s *MacaroonSuite) AddControllerUser(c *gc.C, username string, access permission.Access) {
	_, err := s.State.AddControllerUser(state.UserAccessSpec{
		User:      names.NewUserTag(username),
		CreatedBy: s.AdminUserTag(c),
		Access:    access,
	})
	c.Assert(err, jc.ErrorIsNil)
}

// OpenAPI opens a connection to the API using the given information.
// and empty DialOpts. If info is nil, s.APIInfo(c) is used.
// If jar is non-nil, it will be used as the store for the cookies created
// as a result of API interaction.
func (s *MacaroonSuite) OpenAPI(c *gc.C, info *api.Info, jar http.CookieJar) api.Connection {
	if info == nil {
		info = s.APIInfo(c)
	}
	bakeryClient := httpbakery.NewClient()
	if jar != nil {
		bakeryClient.Client.Jar = jar
	}
	conn, err := api.Open(info, api.DialOpts{
		BakeryClient: bakeryClient,
	})
	c.Assert(err, gc.IsNil)
	return conn
}

// APIInfo returns API connection info suitable for
// connecting to the API using macaroon authentication.
func (s *MacaroonSuite) APIInfo(c *gc.C) *api.Info {
	info := s.JujuConnSuite.APIInfo(c)
	info.Tag = nil
	info.Password = ""
	// Fill in any old macaroon to ensure we don't attempt
	// an anonymous login.
	mac, err := macaroon.New(nil, "test", "")
	c.Assert(err, jc.ErrorIsNil)
	info.Macaroons = []macaroon.Slice{{mac}}
	return info
}

// NewClearableCookieJar returns a new ClearableCookieJar.
func NewClearableCookieJar() *ClearableCookieJar {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	return &ClearableCookieJar{
		jar: jar,
	}
}

// ClearableCookieJar implements a cookie jar
// that can be cleared of all cookies for testing purposes.
type ClearableCookieJar struct {
	jar http.CookieJar
}

// Clear clears all the cookies in the jar.
// It is not OK to call Clear concurrently
// with the other methods.
func (jar *ClearableCookieJar) Clear() {
	newJar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	jar.jar = newJar
}

// Cookies implements http.CookieJar.Cookies.
func (jar *ClearableCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return jar.jar.Cookies(u)
}

// Cookies implements http.CookieJar.SetCookies.
func (jar *ClearableCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	jar.jar.SetCookies(u, cookies)
}
