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
	cs.PatchValue(lxdNewClient, fakeNewClient)
	cs.PatchValue(lxdLoadConfig, fakeLoadConfig)

	// Empty remote means connect locally.
	client, err := Connect(Config{Remote: configIDForLocal})
	c.Assert(client, gc.IsNil)

	// Yes, the error message actually matters here... this is being displayed
	// to the user.
	c.Assert(err, gc.ErrorMatches, "can't connect to the local LXD server.*")
}

func (cs ConnectSuite) TestRemoteConnectError(c *gc.C) {
	cs.PatchValue(lxdNewClient, fakeNewClient)
	cs.PatchValue(lxdLoadConfig, fakeLoadConfig)

	client, err := Connect(Config{Remote: "foo"})
	c.Assert(client, gc.IsNil)

	c.Assert(errors.Cause(err), gc.Equals, testerr)
}

var testerr = errors.Errorf("boo!")

func fakeNewClient(config *lxd.Config, remote string) (*lxd.Client, error) {
	return nil, testerr
}

func fakeLoadConfig() (*lxd.Config, error) {
	return nil, nil
}
