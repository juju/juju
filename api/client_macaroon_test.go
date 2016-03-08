// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/testcharms"
)

var _ = gc.Suite(&clientMacaroonSuite{})

// clientMacaroonSuite tests that Client endpoints that are
// independent of the RPC-based API work with
// macaroon authentication.
type clientMacaroonSuite struct {
	apitesting.MacaroonSuite
	client    *api.Client
	cookieJar *apitesting.ClearableCookieJar
}

func (s *clientMacaroonSuite) SetUpTest(c *gc.C) {
	s.MacaroonSuite.SetUpTest(c)
	s.AddModelUser(c, "testuser@somewhere")
	s.cookieJar = apitesting.NewClearableCookieJar()
	s.DischargerLogin = func() string { return "testuser@somewhere" }
	s.client = s.OpenAPI(c, nil, s.cookieJar).Client()

	// Even though we've logged into the API, we want
	// the tests below to exercise the discharging logic
	// so we clear the cookies.
	s.cookieJar.Clear()
}

func (s *clientMacaroonSuite) TearDownTest(c *gc.C) {
	s.client.Close()
	s.MacaroonSuite.TearDownTest(c)
}

func (s *clientMacaroonSuite) TestAddLocalCharmWithFailedDischarge(c *gc.C) {
	s.DischargerLogin = func() string { return "" }
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	savedURL, err := s.client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, gc.ErrorMatches, `POST https://.*/model/deadbeef-0bad-400d-8000-4b1d0d06f00d/charms\?series=quantal: cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(savedURL, gc.IsNil)
}

func (s *clientMacaroonSuite) TestAddLocalCharmSuccess(c *gc.C) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	// Upload an archive with its original revision.
	savedURL, err := s.client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())
}

func (s *clientMacaroonSuite) TestAddLocalCharmUnauthorized(c *gc.C) {
	s.DischargerLogin = func() string { return "baduser" }
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	// Upload an archive with its original revision.
	_, err := s.client.AddLocalCharm(curl, charmArchive)
	c.Assert(err, gc.ErrorMatches, `POST https://.*/model/deadbeef-0bad-400d-8000-4b1d0d06f00d/charms\?series=quantal: invalid entity name or password`)
}
