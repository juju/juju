// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"errors"
	"net"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/network"
)

type UtilsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UtilsSuite{})

func (s *UtilsSuite) TestSupportsIPv6Error(c *gc.C) {
	s.PatchValue(network.NetListen, func(netFamily, bindAddress string) (net.Listener, error) {
		c.Check(netFamily, gc.Equals, "tcp6")
		c.Check(bindAddress, gc.Equals, "[::1]:0")
		return nil, errors.New("boom!")
	})
	c.Check(network.SupportsIPv6(), jc.IsFalse)
}

func (s *UtilsSuite) TestSupportsIPv6OK(c *gc.C) {
	s.PatchValue(network.NetListen, func(_, _ string) (net.Listener, error) {
		return &mockListener{}, nil
	})
	c.Check(network.SupportsIPv6(), jc.IsTrue)
}

type mockListener struct {
	net.Listener
}

func (*mockListener) Close() error {
	return nil
}
