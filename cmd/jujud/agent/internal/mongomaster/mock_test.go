// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongomaster_test

import "github.com/juju/testing"

type mockConn struct {
	testing.Stub
	master bool
}

func (c *mockConn) IsMaster() (bool, error) {
	c.MethodCall(c, "IsMaster")
	return c.master, c.NextErr()
}

func (c *mockConn) Ping() error {
	c.MethodCall(c, "Ping")
	return c.NextErr()
}
