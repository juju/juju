// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/charm"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
)

// clientMacaroonIntegrationSuite tests that Client endpoints that are
// independent of the RPC-based API work with
// macaroon authentication.
type clientMacaroonIntegrationSuite struct {
	jujutesting.MacaroonSuite
}

var _ = gc.Suite(&clientMacaroonIntegrationSuite{})

func (s *clientMacaroonIntegrationSuite) createTestClient(c *gc.C) *charms.LocalCharmClient {
	username := usertesting.GenNewName(c, "testuser@somewhere")
	s.AddModelUser(c, username)
	s.AddControllerUser(c, username, permission.LoginAccess)
	cookieJar := jujutesting.NewClearableCookieJar()
	s.DischargerLogin = func() string { return username.Name() }
	api := s.OpenAPI(c, nil, cookieJar)
	charmClient, err := charms.NewLocalCharmClient(api)
	c.Assert(err, jc.ErrorIsNil)

	// Even though we've logged into the API, we want
	// the tests below to exercise the discharging logic
	// so we clear the cookies.
	cookieJar.Clear()
	return charmClient
}

func (s *clientMacaroonIntegrationSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Deploying a local charm using a macaroon
`)
}

func (s *clientMacaroonIntegrationSuite) TestAddLocalCharmWithFailedDischarge(c *gc.C) {
	charmClient := s.createTestClient(c)
	s.DischargerLogin = func() string { return "" }
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	savedURL, err := charmClient.AddLocalCharm(curl, charmArchive, false, jujuversion.Current)
	c.Assert(err, gc.ErrorMatches, `Put https://.+: cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(savedURL, gc.IsNil)
}
