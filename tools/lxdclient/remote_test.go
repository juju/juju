// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient_test

import (
	"net"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/tools/lxdclient"
)

var (
	_ = gc.Suite(&remoteSuite{})
)

type remoteSuite struct {
	lxdclient.BaseSuite
}

const (
	testingCert = `
-----BEGIN CERTIFICATE-----
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
EiFwoj8EbmwqzbNpOTNAOdo=
-----END CERTIFICATE-----
`

	testingKey = `
-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----
`
)

func (s *remoteSuite) TestWithDefaultsNoop(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "my-remote",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     s.Cert,
	}
	updated, err := remote.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, remote)
}

func (s *remoteSuite) TestWithDefaultsMissingName(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     s.Cert,
	}
	updated, err := remote.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, remote) // Name is not updated.
}

// TODO(ericsnow) Move this test to a functional suite.
func (s *remoteSuite) TestWithDefaultsMissingCert(c *gc.C) {
	lxdclient.PatchGenerateCertificate(&s.CleanupSuite, testingCert, testingKey)
	remote := lxdclient.Remote{
		Name:     "my-remote",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	}
	updated, err := remote.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Assert(updated.Cert, gc.NotNil)
	c.Check(updated.Cert.Validate(), jc.ErrorIsNil)
	updated.Cert = nil // Validate ensured that the cert was okay.
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name:     "my-remote",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	})
}

func (s *remoteSuite) TestWithDefaultsMissingProtocol(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	}
	updated, err := remote.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Assert(updated.Cert, gc.NotNil)
	c.Check(updated.Cert.Validate(), jc.ErrorIsNil)
	updated.Cert = nil // Validate ensured that the cert was okay.
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name:     "my-remote",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	})
}

func (s *remoteSuite) TestWithDefaultsZeroValue(c *gc.C) {
	var remote lxdclient.Remote
	updated, err := remote.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name:     "local",
		Host:     "",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	})
}

func (s *remoteSuite) TestWithDefaultsLocalNoop(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-local",
		Host: "",
		Cert: nil,
	}
	updated, err := remote.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name:     "my-local",
		Host:     "",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	})
}

func (s *remoteSuite) TestWithDefaultsLocalMissingName(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "",
		Host: "",
		Cert: nil,
	}
	updated, err := remote.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name:     "local",
		Host:     "",
		Cert:     nil,
		Protocol: lxdclient.LXDProtocol,
	})
}

func (s *remoteSuite) TestValidateOkay(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "my-remote",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     s.Cert,
	}
	err := remote.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteSuite) TestValidateZeroValue(c *gc.C) {
	var remote lxdclient.Remote
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateMissingName(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     s.Cert,
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateMissingCert(c *gc.C) {
	// We can have "public" remotes that don't require a client certificate
	// to connect to and get images from.
	remote := lxdclient.Remote{
		Name:     "my-remote",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	}
	err := remote.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteSuite) TestValidateBadCert(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "my-remote",
		Host:     "some-host",
		Protocol: lxdclient.LXDProtocol,
		Cert:     &lxdclient.Cert{},
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateLocalOkay(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "my-local",
		Host:     "",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	}
	err := remote.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteSuite) TestValidateLocalMissingName(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "",
		Host:     "",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateLocalSimplestreamsInvalid(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "",
		Host:     "",
		Protocol: lxdclient.SimplestreamsProtocol,
		Cert:     nil,
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateLocalWithCert(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "my-local",
		Host:     "",
		Protocol: lxdclient.LXDProtocol,
		Cert:     &lxdclient.Cert{},
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateSimplestreamsOkay(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "remote",
		Host:     "http://somewhere/else",
		Protocol: lxdclient.SimplestreamsProtocol,
		Cert:     nil,
	}
	err := remote.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteSuite) TestValidateUnknownProtocol(c *gc.C) {
	remote := lxdclient.Remote{
		Name:     "remote",
		Host:     "http://somewhere/else",
		Protocol: "bogus-protocol",
		Cert:     nil,
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestLocal(c *gc.C) {
	expected := lxdclient.Remote{
		Name:     "local",
		Host:     "",
		Protocol: lxdclient.LXDProtocol,
		Cert:     nil,
	}
	c.Check(lxdclient.Local, jc.DeepEquals, expected)
}

func (s *remoteSuite) TestIDOkay(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	}
	id := remote.ID()

	c.Check(id, gc.Equals, "my-remote")
}

func (s *remoteSuite) TestIDLocal(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-remote",
		Host: "",
		Cert: s.Cert,
	}
	id := remote.ID()

	c.Check(id, gc.Equals, "local")
}

func isValidAddr(value interface{}) bool {
	addr, ok := value.(string)
	if !ok {
		return false
	}
	return net.ParseIP(addr) != nil
}
