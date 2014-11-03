// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"fmt"

	"github.com/juju/macaroon"
	"github.com/juju/macaroon/bakery"
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/state"
)

type remoteLoginSuite struct {
	baseLoginSuite

	info            state.StateServingInfo
	remoteIdKey     *bakery.KeyPair
	remoteIdService *bakery.Service
}

type loggedInChecker struct{}

func (*loggedInChecker) CheckThirdPartyCaveat(caveatId, condition string) ([]bakery.Caveat, error) {
	if condition == "logged-in-user" {
		return nil, nil
	}
	return nil, fmt.Errorf("unrecognized condition")
}

var _ = gc.Suite(&remoteLoginSuite{
	baseLoginSuite: baseLoginSuite{
		setAdminApi: func(srv *apiserver.Server) {
			apiserver.SetAdminApiVersions(srv, 1)
		},
	},
})

func (s *remoteLoginSuite) SetUpTest(c *gc.C) {
	s.baseLoginSuite.SetUpTest(c)

	var err error
	s.remoteIdKey, err = bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	s.remoteIdService, err = bakery.NewService(bakery.NewServiceParams{
		Location: "remote-service-location",
		Key:      s.remoteIdKey,
	})
	c.Assert(err, gc.IsNil)

	// Configure state server to trust this remote identity provider
	// and have its own target service public key identity.
	s.info, err = s.State.StateServingInfo()
	c.Assert(err, gc.IsNil)
	s.info.IdentityProvider = &state.IdentityProvider{
		PublicKey: s.remoteIdKey.Public,
		Location:  "remote-service-location",
	}
	s.info.TargetKeyPair, err = bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	err = s.State.SetStateServingInfo(s.info)
	c.Assert(err, gc.IsNil)
}

func (s *remoteLoginSuite) TearDownTest(c *gc.C) {
	info, err := s.State.StateServingInfo()
	c.Assert(err, gc.IsNil)
	info.IdentityProvider = nil
	info.TargetKeyPair = nil
	err = s.State.SetStateServingInfo(info)
	c.Assert(err, gc.IsNil)

	s.baseLoginSuite.TearDownTest(c)
}

func (s *remoteLoginSuite) TestRemoteLoginReauth(c *gc.C) {
	st, cleanup := s.setupServer(c)
	defer cleanup()

	// Try to log in as a remote identity.
	remoteUser := names.NewUserTag("bob")
	reauth, err := st.Login(remoteUser.String(), "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(reauth, gc.NotNil)

	// No API facade versions. We're not logged in yet.
	c.Check(st.AllFacadeVersions(), gc.HasLen, 0)

	// As the remote client, decode the reauth request, obtain a discharge
	// macaroon from the identity-providing service, bind and serialize the
	// followup credential.
	var remoteCreds authentication.RemoteCredentials
	err = remoteCreds.UnmarshalText([]byte(reauth.Prompt))
	c.Assert(err, gc.IsNil)
	remoteCreds.Discharges, err = bakery.DischargeAll(remoteCreds.Primary,
		func(loc string, cav macaroon.Caveat) (*macaroon.Macaroon, error) {
			//c.Assert(loc, gc.Equals, s.info.IdentityProvider.Location)
			return s.remoteIdService.Discharge(&loggedInChecker{}, cav.Id)
		},
	)
	c.Assert(err, gc.IsNil)
	// TODO (cmars): move this to RemoteCredentials.Bind?
	for _, dm := range remoteCreds.Discharges {
		dm.Bind(remoteCreds.Primary.Signature())
	}
	credBytes, err := remoteCreds.MarshalText()
	c.Assert(err, gc.IsNil)

	// Retry the remote login request
	reauth, err = st.Login(remoteUser.String(), string(credBytes), reauth.Nonce)
	c.Assert(err, gc.IsNil)
	c.Assert(reauth, gc.IsNil)

	// Should be logged in
	c.Assert(st.Ping(), gc.IsNil)
	c.Assert(st.AllFacadeVersions(), gc.Not(gc.HasLen), 0)
}
