// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/lxc/lxd"
	gc "gopkg.in/check.v1"
)

type ConnectSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ConnectSuite{})

func (cs *ConnectSuite) TestLocalConnectError(c *gc.C) {
	cs.PatchValue(&lxdNewClientFromInfo, fakeNewClientFromInfo)

	cfg, err := Config{
		Remote: Local,
	}.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	_, err = Connect(cfg)

	// Yes, the error message actually matters here... this is being displayed
	// to the user.
	c.Assert(err, gc.ErrorMatches, "can't connect to the local LXD server.*")
}

func (cs *ConnectSuite) TestRemoteConnectError(c *gc.C) {
	cs.PatchValue(&lxdNewClientFromInfo, fakeNewClientFromInfo)

	cfg, err := Config{
		Remote: Remote{
			Name: "foo",
			Host: "a.b.c",
			Cert: &Cert{
				Name:    "really-valid",
				CertPEM: []byte("kinda-public"),
				KeyPEM:  []byte("super-secret"),
			},
		},
	}.WithDefaults()
	c.Assert(err, jc.ErrorIsNil)
	_, err = Connect(cfg)

	c.Assert(errors.Cause(err), gc.Equals, testerr)
}

var testerr = errors.Errorf("boo!")

func fakeNewClientFromInfo(info lxd.ConnectInfo) (*lxd.Client, error) {
	return nil, testerr
}
