// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd/lxdclient"
)

var (
	_ = gc.Suite(&remoteInfoSuite{})
	_ = gc.Suite(&remoteSuite{})
)

type remoteInfoSuite struct {
	lxdclient.BaseSuite
}

func (s *remoteInfoSuite) TestSetDefaultsNoop(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	}
	updated, err := info.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, info)
}

func (s *remoteInfoSuite) TestSetDefaultsMissingName(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "",
		Host: "some-host",
		Cert: s.Cert,
	}
	updated, err := info.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, info) // Name is not updated.
}

// TODO(ericsnow) Move this test to a functional suite.
func (s *remoteInfoSuite) TestSetDefaultsMissingCert(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: nil,
	}
	err := info.Validate()
	c.Assert(err, gc.NotNil) // Make sure the original is invalid.

	updated, err := info.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	updated.Cert = nil // Validate ensured that the cert was okay.
	c.Check(updated, jc.DeepEquals, lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: nil,
	})
}

func (s *remoteInfoSuite) TestSetDefaultsZeroValue(c *gc.C) {
	var info lxdclient.RemoteInfo
	updated, err := info.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.RemoteInfo{
		Name: "local",
		Host: "",
		Cert: nil,
	})
}

func (s *remoteInfoSuite) TestSetDefaultsLocalNoop(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "my-local",
		Host: "",
		Cert: nil,
	}
	updated, err := info.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.RemoteInfo{
		Name: "my-local",
		Host: "",
		Cert: nil,
	})
}

func (s *remoteInfoSuite) TestSetDefaultsLocalMissingName(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "",
		Host: "",
		Cert: nil,
	}
	updated, err := info.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.RemoteInfo{
		Name: "local",
		Host: "",
		Cert: nil,
	})
}

func (s *remoteInfoSuite) TestValidateOkay(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	}
	err := info.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteInfoSuite) TestValidateMissingName(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "",
		Host: "some-host",
		Cert: s.Cert,
	}
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteInfoSuite) TestValidateMissingCert(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: nil,
	}
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteInfoSuite) TestValidateBadCert(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: &lxdclient.Certificate{},
	}
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteInfoSuite) TestValidateLocalOkay(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "my-local",
		Host: "",
		Cert: nil,
	}
	err := info.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteInfoSuite) TestValidateLocalMissingName(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "",
		Host: "",
		Cert: nil,
	}
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteInfoSuite) TestValidateLocalWithCert(c *gc.C) {
	info := lxdclient.RemoteInfo{
		Name: "my-local",
		Host: "",
		Cert: &lxdclient.Certificate{},
	}
	err := info.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

type remoteSuite struct {
	lxdclient.BaseSuite
}

func (s *remoteSuite) TestLocal(c *gc.C) {
	expected := lxdclient.NewRemote(lxdclient.RemoteInfo{
		Name: "local",
		Host: "",
		Cert: nil,
	})
	c.Check(lxdclient.Local, jc.DeepEquals, expected)
}

func (s *remoteSuite) TestValidateOkay(c *gc.C) {
	remote := lxdclient.NewRemote(lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	})
	err := remote.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteSuite) TestValidateZeroValue(c *gc.C) {
	var remote lxdclient.Remote
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateInvalid(c *gc.C) {
	remote := lxdclient.NewRemote(lxdclient.RemoteInfo{
		Name: "",
		Host: "some-host",
		Cert: s.Cert,
	})
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestIDOkay(c *gc.C) {
	remote := lxdclient.NewRemote(lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	})
	id := remote.ID()

	c.Check(id, gc.Equals, "my-remote")
}

func (s *remoteSuite) TestIDLocal(c *gc.C) {
	remote := lxdclient.NewRemote(lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "",
		Cert: s.Cert,
	})
	id := remote.ID()

	c.Check(id, gc.Equals, "")
}

func (s *remoteSuite) TestCertOkay(c *gc.C) {
	remote := lxdclient.NewRemote(lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	})
	cert := remote.Cert()

	c.Check(cert, jc.DeepEquals, *s.Cert)
}

func (s *remoteSuite) TestCertMissing(c *gc.C) {
	remote := lxdclient.NewRemote(lxdclient.RemoteInfo{
		Name: "my-remote",
		Host: "some-host",
		Cert: nil,
	})
	cert := remote.Cert()

	c.Check(cert, jc.DeepEquals, lxdclient.Certificate{})
}
