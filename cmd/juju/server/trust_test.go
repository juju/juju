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

type TrustCommandSuite struct {
	BaseSuite
}

var (
	_ = gc.Suite(&TrustCommandSuite{})
)

func newTrustCommand(api server.AdminAPI) cmd.Command {
	return envcmd.Wrap(server.NewTrustCommand(api))
}

type fakeAdminAPI struct {
	fakePublicKey string
	fakeLocation  string
}

func (api *fakeAdminAPI) IdentityProvider() (*params.IdentityProviderInfo, error) {
	return &params.IdentityProviderInfo{
		PublicKey: api.fakePublicKey,
		Location:  api.fakeLocation,
	}, nil
}

func (api *fakeAdminAPI) SetIdentityProvider(publicKey, location string) error {
	api.fakePublicKey = publicKey
	api.fakeLocation = location
	return nil
}

func (*fakeAdminAPI) Close() error {
	return nil
}

type nilAdminAPI struct{}

func (*nilAdminAPI) IdentityProvider() (*params.IdentityProviderInfo, error) {
	return nil, nil
}

func (*nilAdminAPI) SetIdentityProvider(publicKey, location string) error {
	return nil
}

func (*nilAdminAPI) Close() error {
	return nil
}

func (s *TrustCommandSuite) TestTrust(c *gc.C) {
	api := &fakeAdminAPI{}

	// Not set
	context, err := testing.RunCommand(c, newTrustCommand(&nilAdminAPI{}))
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimSpace(testing.Stderr(context)), gc.Matches, ".*not set.*")

	// Invalid public key content (not base64)
	_, err = testing.RunCommand(c, newTrustCommand(api), "foo", "barrington")
	c.Assert(err, gc.ErrorMatches, ".*illegal base64 data at input byte 0.*")

	// Invalid public key content (zero-length)
	_, err = testing.RunCommand(c, newTrustCommand(api), "", "juju-land")
	c.Assert(err, gc.ErrorMatches, ".*invalid public key length.*")

	// Set the trusted public key & location
	context, err = testing.RunCommand(c, newTrustCommand(api), "anVzdGlmaWVkIGFuY2llbnRzIG9mIGp1anUgICAgICA=", "juju-land")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimSpace(testing.Stdout(context)), gc.Equals, "")

	// Read it back
	context, err = testing.RunCommand(c, newTrustCommand(api))
	c.Assert(err, gc.IsNil)
	c.Assert(strings.TrimSpace(testing.Stdout(context)), gc.Equals,
		"anVzdGlmaWVkIGFuY2llbnRzIG9mIGp1anUgICAgICA=\tjuju-land")
}
