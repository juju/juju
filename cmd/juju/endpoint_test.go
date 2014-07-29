// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
)

type EndpointSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&EndpointSuite{})

func (s *EndpointSuite) TestEndpoint(c *gc.C) {
	ctx := coretesting.Context(c)
	code := cmd.Main(envcmd.Wrap(&EndpointCommand{}), ctx, []string{})
	c.Check(code, gc.Equals, 0)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	output := string(ctx.Stdout.(*bytes.Buffer).Bytes())
	info := s.APIInfo(c)
	c.Assert(output, gc.Equals, fmt.Sprintf("%s\n", info.Addrs[0]))
}

func (s *EndpointSuite) TestPublicIPv4Filtering(c *gc.C) {
	tests := []struct {
		addr         string
		isPublicIPv4 bool
	}{
		{"127.0.0.1:17070", true},
		{"91.189.94.156:17070", true},
		{"10.0.0.1:17070", false},
		{"172.16.1.1:17070", false},
		{"192.168.1.1:17070", false},
		{"[0:0:0:0:0:0:0:1]:17070", false},
		{"[::1]:17070", false},
	}
	for _, test := range tests {
		ep, err := newEndpointIP(test.addr)
		c.Assert(err, gc.IsNil)
		c.Assert(ep.isPublicV4(), gc.Equals, test.isPublicIPv4)
	}
}
