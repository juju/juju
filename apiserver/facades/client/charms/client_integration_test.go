// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v12"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/core/permission"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
	jujuversion "github.com/juju/juju/version"
)

// clientMacaroonIntegrationSuite tests that Client endpoints that are
// independent of the RPC-based API work with
// macaroon authentication.
type clientMacaroonIntegrationSuite struct {
	jujutesting.MacaroonSuite
}

var _ = gc.Suite(&clientMacaroonIntegrationSuite{})

func (s *clientMacaroonIntegrationSuite) createTestClient(c *gc.C) *charms.LocalCharmClient {
	username := "testuser@somewhere"
	s.AddModelUser(c, username)
	s.AddControllerUser(c, username, permission.LoginAccess)
	cookieJar := jujutesting.NewClearableCookieJar()
	s.DischargerLogin = func() string { return username }
	api := s.OpenAPI(c, nil, cookieJar)
	httpPutter, err := charms.NewHTTPPutter(api)
	c.Assert(err, jc.ErrorIsNil)
	charmClient := charms.NewLocalCharmClient(api, httpPutter)

	// Even though we've logged into the API, we want
	// the tests below to exercise the discharging logic
	// so we clear the cookies.
	cookieJar.Clear()
	return charmClient
}

func (s *clientMacaroonIntegrationSuite) TestAddLocalCharmWithFailedDischarge(c *gc.C) {
	charmClient := s.createTestClient(c)
	s.DischargerLogin = func() string { return "" }
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	savedURL, err := charmClient.AddLocalCharm(curl, charmArchive, false, jujuversion.Current)
	c.Assert(err, gc.ErrorMatches, `Post https://.+: cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(savedURL, gc.IsNil)
}

func (s *clientMacaroonIntegrationSuite) TestAddLocalCharmSuccess(c *gc.C) {
	httpPutter, err := charms.NewHTTPPutter(s.OpenControllerModelAPI(c))
	c.Assert(err, jc.ErrorIsNil)
	charmClient := charms.NewLocalCharmClient(s.OpenControllerModelAPI(c), httpPutter)
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	testcharms.CheckCharmReady(c, charmArchive)

	// Upload an archive with its original revision.
	savedURL, err := charmClient.AddLocalCharm(curl, charmArchive, false, jujuversion.Current)
	// We know that in testing we occasionally see "zip: not a valid zip file" occur.
	// Even after many efforts, we haven't been able to find the source. It almost never
	// happens locally, and we don't see this in production.
	// TODO: remove the skip when we are using the fake charmstore.
	if err != nil {
		if strings.Contains(err.Error(), "zip: not a valid zip file") {
			c.Skip("intermittent charmstore upload issue")
		}
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())
}
