// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/testcharms"
)

var _ = gc.Suite(&clientMacaroonSuite{})

// clientMacaroonSuite tests that Client endpoints that are
// independent of the RPC-based API work with
// macaroon authentication.
type clientMacaroonSuite struct {
	apitesting.MacaroonSuite
}

func (s *clientMacaroonSuite) createTestClient(c *gc.C) *api.Client {
	username := "testuser@somewhere"
	s.AddModelUser(c, username)
	s.AddControllerUser(c, username, permission.LoginAccess)
	cookieJar := apitesting.NewClearableCookieJar()
	s.DischargerLogin = func() string { return username }
	client := s.OpenAPI(c, nil, cookieJar).Client()

	// Even though we've logged into the API, we want
	// the tests below to exercise the discharging logic
	// so we clear the cookies.
	cookieJar.Clear()
	return client
}

func (s *clientMacaroonSuite) TestAddLocalCharmWithFailedDischarge(c *gc.C) {
	client := s.createTestClient(c)
	s.DischargerLogin = func() string { return "" }
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false)
	c.Assert(err, gc.ErrorMatches, `POST https://.+: cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(savedURL, gc.IsNil)
}

func (s *clientMacaroonSuite) TestAddLocalCharmSuccess(c *gc.C) {
	client := s.createTestClient(c)
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	testcharms.CheckCharmReady(c, charmArchive)

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())
}

func (s *clientMacaroonSuite) TestAddLocalCharmUnauthorized(c *gc.C) {
	client := s.createTestClient(c)
	s.DischargerLogin = func() string { return "baduser" }
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	// Upload an archive with its original revision.
	_, err := client.AddLocalCharm(curl, charmArchive, false)
	c.Assert(err, gc.ErrorMatches, `.*invalid entity name or password$`)
}
