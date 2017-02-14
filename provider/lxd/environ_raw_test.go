// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/tools/lxdclient"
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
	cfg, err := getRemoteConfig(s.spec)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, &lxdclient.Config{
		Remote: lxdclient.Remote{
			Name:     "remote",
			Host:     "10.0.8.1",
			Protocol: "lxd",
			Cert: &lxdclient.Cert{
				Name:    "juju",
				CertPEM: []byte("client.crt"),
				KeyPEM:  []byte("client.key"),
			},
			ServerPEMCert: "server.crt",
		},
	})
}
