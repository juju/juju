// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"

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
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&EndpointCommand{}))
	c.Assert(err, gc.IsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	output := string(ctx.Stdout.(*bytes.Buffer).Bytes())
	info := s.APIInfo(c)
	c.Assert(output, gc.Equals, fmt.Sprintf("%s\n", info.Addrs[0]))
}

func (s *EndpointSuite) TestMultipleEndpoints(c *gc.C) {
	// Run command once to create store in test.
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&EndpointCommand{}))
	c.Assert(err, gc.IsNil)
	info := s.APIInfo(c)
	s.setAPIAddresses(c, info.Addrs[0], "0.1.2.3:17070", "0.2.3.4:17070")
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&EndpointCommand{}))
	c.Assert(err, gc.IsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	output := string(ctx.Stdout.(*bytes.Buffer).Bytes())
	c.Assert(output, gc.Equals, fmt.Sprintf("%s\n", info.Addrs[0]))
}

func (s *EndpointSuite) TestNoEndpoints(c *gc.C) {
	// Run command once to create store in test.
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&EndpointCommand{}))
	c.Assert(err, gc.IsNil)
	info := s.APIInfo(c)
	s.setAPIAddresses(c)
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&EndpointCommand{}))
	c.Assert(err, gc.IsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	output := string(ctx.Stdout.(*bytes.Buffer).Bytes())
	c.Assert(output, gc.Equals, fmt.Sprintf("%s\n", info.Addrs[0]))
}

// setAPIAddresses changes the API endpoint addresses to the
// passed ones.
func (s *EndpointSuite) setAPIAddresses(c *gc.C, addresses ...string) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	info, err := s.ConfigStore.ReadInfo(env.Name())
	c.Assert(err, gc.IsNil)
	endpoint := info.APIEndpoint()
	endpoint.Addresses = addresses
	info.SetAPIEndpoint(endpoint)
	err = info.Write()
	c.Assert(err, gc.IsNil)
}
