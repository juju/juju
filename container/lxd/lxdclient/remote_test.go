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
	_ = gc.Suite(&remoteSuite{})
)

type remoteSuite struct {
	lxdclient.BaseSuite
}

func (s *remoteSuite) TestSetDefaultsNoop(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
	}
	updated, err := remote.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, remote)
}

func (s *remoteSuite) TestSetDefaultsMissingName(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "",
		Host: "some-host",
		Cert: s.Cert,
	}
	updated, err := remote.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(updated, jc.DeepEquals, remote) // Name is not updated.
}

// TODO(ericsnow) Move this test to a functional suite.
func (s *remoteSuite) TestSetDefaultsMissingCert(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: nil,
	}
	err := remote.Validate()
	c.Assert(err, gc.NotNil) // Make sure the original is invalid.

	updated, err := remote.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	updated.Cert = nil // Validate ensured that the cert was okay.
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: nil,
	})
}

func (s *remoteSuite) TestSetDefaultsZeroValue(c *gc.C) {
	var remote lxdclient.Remote
	updated, err := remote.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name: "local",
		Host: "",
		Cert: nil,
	})
}

func (s *remoteSuite) TestSetDefaultsLocalNoop(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-local",
		Host: "",
		Cert: nil,
	}
	updated, err := remote.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name: "my-local",
		Host: "",
		Cert: nil,
	})
}

func (s *remoteSuite) TestSetDefaultsLocalMissingName(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "",
		Host: "",
		Cert: nil,
	}
	updated, err := remote.SetDefaults()
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Validate()

	c.Check(err, jc.ErrorIsNil)
	c.Check(updated, jc.DeepEquals, lxdclient.Remote{
		Name: "local",
		Host: "",
		Cert: nil,
	})
}

func (s *remoteSuite) TestValidateOkay(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: s.Cert,
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
		Name: "",
		Host: "some-host",
		Cert: s.Cert,
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateMissingCert(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: nil,
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateBadCert(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-remote",
		Host: "some-host",
		Cert: &lxdclient.Cert{},
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateLocalOkay(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-local",
		Host: "",
		Cert: nil,
	}
	err := remote.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *remoteSuite) TestValidateLocalMissingName(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "",
		Host: "",
		Cert: nil,
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestValidateLocalWithCert(c *gc.C) {
	remote := lxdclient.Remote{
		Name: "my-local",
		Host: "",
		Cert: &lxdclient.Cert{},
	}
	err := remote.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *remoteSuite) TestLocal(c *gc.C) {
	expected := lxdclient.Remote{
		Name: "local",
		Host: "",
		Cert: nil,
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

	c.Check(id, gc.Equals, "")
}
