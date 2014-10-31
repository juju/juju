// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package server_test

import (
	"strings"

	"github.com/juju/cmd"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/server"
	"github.com/juju/juju/testing"
)

type ServerTrustCommandSuite struct {
	BaseSuite
}

var (
	_ = gc.Suite(&ServerTrustCommandSuite{})
)

func newServerTrustCommand(api server.ServerAdminAPI) cmd.Command {
	return envcmd.Wrap(server.NewTrustCommand(api))
}

type fakeServerAdminAPI struct {
	fakePublicKey string
	fakeLocation  string
}

func (api *fakeServerAdminAPI) IdentityProvider() (*params.IdentityProviderInfo, error) {
	return &params.IdentityProviderInfo{
		PublicKey: api.fakePublicKey,
		Location:  api.fakeLocation,
	}, nil
}

func (api *fakeServerAdminAPI) SetIdentityProvider(publicKey, location string) error {
	api.fakePublicKey = publicKey
	api.fakeLocation = location
	return nil
}

func (*fakeServerAdminAPI) Close() error {
	return nil
}

type nilServerAdminAPI struct{}

func (*nilServerAdminAPI) IdentityProvider() (*params.IdentityProviderInfo, error) {
	return nil, nil
}

func (*nilServerAdminAPI) SetIdentityProvider(publicKey, location string) error {
	return nil
}

func (*nilServerAdminAPI) Close() error {
	return nil
}

func (s *ServerTrustCommandSuite) TestServerTrust(c *gc.C) {
	api := &fakeServerAdminAPI{}

	// Not set
	context, err := testing.RunCommand(c, newServerTrustCommand(&nilServerAdminAPI{}))
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimSpace(testing.Stderr(context)), gc.Matches, ".*not set.*")

	// Invalid public key content (not base64)
	_, err = testing.RunCommand(c, newServerTrustCommand(api), "foo", "barrington")
	c.Assert(err, gc.ErrorMatches, ".*illegal base64 data at input byte 0.*")

	// Invalid public key content (zero-length)
	_, err = testing.RunCommand(c, newServerTrustCommand(api), "", "juju-land")
	c.Assert(err, gc.ErrorMatches, ".*invalid public key length.*")

	// Set the trusted public key & location
	context, err = testing.RunCommand(c, newServerTrustCommand(api), "anVzdGlmaWVkIGFuY2llbnRzIG9mIGp1anUgICAgICA=", "juju-land")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimSpace(testing.Stdout(context)), gc.Equals, "")

	// Read it back
	context, err = testing.RunCommand(c, newServerTrustCommand(api))
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimSpace(testing.Stdout(context)), gc.Equals,
		"anVzdGlmaWVkIGFuY2llbnRzIG9mIGp1anUgICAgICA=\tjuju-land")
}
