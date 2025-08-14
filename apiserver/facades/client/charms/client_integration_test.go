// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/controllernode"
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

func TestClientMacaroonIntegrationSuite(t *testing.T) {
	tc.Run(t, &clientMacaroonIntegrationSuite{})
}
func (s *clientMacaroonIntegrationSuite) createTestClient(c *tc.C) *charms.LocalCharmClient {
	username := coreuser.GenName(c, "testuser@somewhere")
	s.AddModelUser(c, username)
	s.AddControllerUser(c, username, permission.LoginAccess)

	controllerNodeService := s.ControllerDomainServices(c).ControllerNode()
	addrs := network.SpaceHostPorts{
		{
			SpaceAddress: network.SpaceAddress{
				MachineAddress: network.MachineAddress{
					Value: "10.9.9.32",
				},
			},
			NetPort: 42,
		},
	}
	err := controllerNodeService.SetAPIAddresses(c.Context(), controllernode.SetAPIAddressArgs{
		APIAddresses: map[string]network.SpaceHostPorts{
			"0": addrs,
		},
	})
	c.Assert(err, tc.IsNil)

	cookieJar := jujutesting.NewClearableCookieJar()
	s.DischargerLogin = func() string { return username.Name() }
	api := s.OpenAPI(c, nil, cookieJar)
	charmClient, err := charms.NewLocalCharmClient(api)
	c.Assert(err, tc.ErrorIsNil)

	// Even though we've logged into the API, we want
	// the tests below to exercise the discharging logic
	// so we clear the cookies.
	cookieJar.Clear()
	return charmClient
}

func (s *clientMacaroonIntegrationSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Deploying a local charm using a macaroon
`)
}

func (s *clientMacaroonIntegrationSuite) TestAddLocalCharmWithFailedDischarge(c *tc.C) {
	charmClient := s.createTestClient(c)
	s.DischargerLogin = func() string { return "" }
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	savedURL, err := charmClient.AddLocalCharm(curl, charmArchive, false, jujuversion.Current)
	c.Assert(err, tc.ErrorMatches, `Put https://.+: cannot get discharge from "https://.*": third party refused discharge: cannot discharge: login denied by discharger`)
	c.Assert(savedURL, tc.IsNil)
}
