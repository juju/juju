// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/cmd"
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
