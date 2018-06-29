// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/environs"
)

type environRawSuite struct {
	testing.IsolationSuite
	testing.Stub
	spec environs.CloudSpec
}

var _ = gc.Suite(&environRawSuite{})

func (s *environRawSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()

	cred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": "client.crt",
		"client-key":  "client.key",
		"server-cert": "server.crt",
	})
	s.spec = environs.CloudSpec{
		Type:       "lxd",
		Name:       "localhost",
		Endpoint:   "10.0.8.1",
		Credential: &cred,
	}
}

func (s *environRawSuite) TestGetRemoteConfig(c *gc.C) {
	cert, server, ok := getCertificates(s.spec)
	c.Assert(ok, jc.DeepEquals, true)
	c.Assert(cert, jc.DeepEquals, &lxd.Certificate{
		Name:    "juju",
		CertPEM: []byte("client.crt"),
		KeyPEM:  []byte("client.key"),
	})
	c.Assert(server, gc.Equals, "server.crt")
}
