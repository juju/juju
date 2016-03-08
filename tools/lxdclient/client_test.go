// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"github.com/lxc/lxd"
	gc "gopkg.in/check.v1"
)

type ConnectSuite struct {
	testing.IsolationSuite
}

func (cs ConnectSuite) TestLocalConnectError(c *gc.C) {
	cs.PatchValue(lxdNewClientFromInfo, fakeNewClientFromInfo)
	cs.PatchValue(lxdLoadConfig, fakeLoadConfig)

	cfg := Config{
		Remote: Local,
	}
	_, err := Connect(cfg)

	// Yes, the error message actually matters here... this is being displayed
	// to the user.
	c.Assert(err, gc.ErrorMatches, "can't connect to the local LXD server.*")
}

func (cs ConnectSuite) TestRemoteConnectError(c *gc.C) {
	cs.PatchValue(lxdNewClientFromInfo, fakeNewClientFromInfo)
	cs.PatchValue(lxdLoadConfig, fakeLoadConfig)

	cfg := Config{
		Remote: Remote{
			Name: "foo",
			Host: "a.b.c",
		},
	}
	_, err := Connect(cfg)

	c.Assert(errors.Cause(err), gc.Equals, testerr)
}

var testerr = errors.Errorf("boo!")

func fakeNewClientFromInfo(config *lxd.Config, remote string) (*lxd.Client, error) {
	return nil, testerr
}

func fakeLoadConfig() (*lxd.Config, error) {
	return nil, nil
}
