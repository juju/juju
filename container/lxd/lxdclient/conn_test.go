// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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

	// Empty remote means connect locally.
	client, err := Connect(Config{Remote: ""})
	c.Assert(client, gc.IsNil)

	// Yes, the error message actually matters here... this is being displayed
	// to the user.
	c.Assert(err, gc.ErrorMatches, "can't connect to the local LXD server.*")
}

func (cs ConnectSuite) TestRemoteConnectError(c *gc.C) {
	cs.PatchValue(lxdNewClient, fakeNewClient)

	// Empty remote means connect locally.
	client, err := Connect(Config{Remote: "foo"})
	c.Assert(client, gc.IsNil)

	// Yes, the error message actually matters here... this is being displayed
	// to the user.
	c.Assert(err, gc.ErrorMatches, "boo!")
}

func fakeNewClient(config *lxd.Config, remote string) (*lxd.Client, error) {
	return nil, errors.Errorf("boo!")
}
