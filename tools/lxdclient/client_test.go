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

func (cs ConnectSuite) TestLocalConnectError(c *gc.C) {
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

func (cs ConnectSuite) TestRemoteConnectError(c *gc.C) {
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

func (cs ConnectSuite) TestLXDClientForCloudImagesDefault(c *gc.C) {
	// Note: this assumes current LXD behavior to not actually connect to
	// the remote host until we try to perform an action.
	client, err := lxdClientForCloudImages(Config{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(client.BaseURL, gc.Equals, lxd.UbuntuRemote.Addr)
}

func (cs ConnectSuite) TestLXDClientForCloudImagesDaily(c *gc.C) {
	client, err := lxdClientForCloudImages(Config{
		ImageStream: StreamDaily,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(client.BaseURL, gc.Equals, lxd.UbuntuDailyRemote.Addr)
}

func (cs ConnectSuite) TestLXDClientForCloudImagesReleases(c *gc.C) {
	client, err := lxdClientForCloudImages(Config{
		ImageStream: StreamReleases,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(client.BaseURL, gc.Equals, lxd.UbuntuRemote.Addr)
}

var testerr = errors.Errorf("boo!")

func fakeNewClientFromInfo(info lxd.ConnectInfo) (*lxd.Client, error) {
	return nil, testerr
}
