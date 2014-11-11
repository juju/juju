// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package serveradmin_test

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/serveradmin"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

var _ serveradmin.ServerAdmin = (*serveradmin.API)(nil)

type serveradminSuite struct {
	jujutesting.JujuConnSuite

	serveradmin *serveradmin.API
	authorizer  apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&serveradminSuite{})

func (s *serveradminSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:            names.NewUserTag("athena"),
		EnvironManager: true,
	}
	serveradmin, err := serveradmin.NewAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.serveradmin = serveradmin
}

func (s *serveradminSuite) TestIdentityProviderRoundTrip(c *gc.C) {
	result, err := s.serveradmin.IdentityProvider()
	c.Assert(err, gc.IsNil)
	c.Assert(result.IdentityProvider, gc.IsNil)

	err = s.serveradmin.SetIdentityProvider(params.SetIdentityProvider{
		IdentityProvider: &params.IdentityProviderInfo{
			PublicKey: "jRaoRRoLZNaBRZLCDNyWLtCw9A1Gx2hf7ImPXbqMPDA=",
			Location:  "elpis",
		},
	})
	c.Assert(err, gc.IsNil)

	result, err = s.serveradmin.IdentityProvider()
	c.Assert(err, gc.IsNil)
	c.Assert(result.IdentityProvider, gc.NotNil)
	c.Assert(result.IdentityProvider.PublicKey, gc.Equals, "jRaoRRoLZNaBRZLCDNyWLtCw9A1Gx2hf7ImPXbqMPDA=")
	c.Assert(result.IdentityProvider.Location, gc.Equals, "elpis")
}
