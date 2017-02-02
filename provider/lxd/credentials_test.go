// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"net"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/lxd"
)

type credentialsSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (s *credentialsSuite) TestCredentialSchemas(c *gc.C) {
	envtesting.AssertProviderAuthTypes(c, s.Provider, "certificate")
}

func (s *credentialsSuite) TestDetectCredentials(c *gc.C) {
	credentials, err := s.Provider.DetectCredentials()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(credentials, jc.DeepEquals, cloud.NewEmptyCloudCredential())
}

func (s *credentialsSuite) TestFinalizeCredential(c *gc.C) {
	out, err := s.Provider.FinalizeCredential(nil, environs.FinalizeCredentialParams{
		CloudEndpoint: "",
		Credential:    cloud.NewEmptyCredential(),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(out.AuthType(), gc.Equals, cloud.CertificateAuthType)
	c.Assert(out.Attributes(), jc.DeepEquals, map[string]string{
		"client-cert": "client.crt",
		"client-key":  "client.key",
		"server-cert": "server-cert",
	})
	s.Stub.CheckCallNames(c,
		"LookupHost", "InterfaceAddrs",
		"GenerateMemCert", "AddCert", "ServerStatus",
	)
}

func (s *credentialsSuite) TestFinalizeCredentialNonLocal(c *gc.C) {
	// Patch the interface addresses for the calling machine, so
	// it appears that we're not on the LXD server host.
	s.PatchValue(&s.InterfaceAddrs, []net.Addr{&net.IPNet{IP: net.ParseIP("8.8.8.8")}})
	out, err := s.Provider.FinalizeCredential(nil, environs.FinalizeCredentialParams{
		CloudEndpoint: "",
		Credential:    cloud.NewEmptyCredential(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out.AuthType(), gc.Equals, cloud.EmptyAuthType)
}
