// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/macaroon/bakery"
	"github.com/juju/names"
	//jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/state"
)

type remoteLoginSuite struct {
	baseLoginSuite

	remoteIdKey     *bakery.KeyPair
	remoteIdService *bakery.Service
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
	info, err := s.State.StateServingInfo()
	c.Assert(err, gc.IsNil)
	info.IdentityProvider = &state.IdentityProvider{
		PublicKey: s.remoteIdKey.Public,
		Location:  "remote-service-location",
	}
	info.TargetKeyPair, err = bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	err = s.State.SetStateServingInfo(info)
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

func (s *remoteLoginSuite) TestRemoteLogin(c *gc.C) {
	st, cleanup := s.setupServer(c)
	defer cleanup()
	remoteUser := names.NewUserTag("bob")
	reauth, err := st.Login(remoteUser.String(), "", "")
	c.Assert(err, gc.IsNil)
	c.Assert(reauth, gc.NotNil)
}
