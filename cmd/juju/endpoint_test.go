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
	s.modifyAddresses(c, []string{"10.0.0.1:17070", "10.0.0.2:17070"})
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&EndpointCommand{}))
	c.Assert(err, gc.IsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	output := string(ctx.Stdout.(*bytes.Buffer).Bytes())
	info := s.APIInfo(c)
	c.Assert(output, gc.Equals, fmt.Sprintf("%s\n", info.Addrs[0]))
}

func (s *EndpointSuite) TestNoEndpoints(c *gc.C) {
	// Run command once to create store in test.
	_, err := coretesting.RunCommand(c, envcmd.Wrap(&EndpointCommand{}))
	c.Assert(err, gc.IsNil)
	s.modifyAddresses(c, nil)
	ctx, err := coretesting.RunCommand(c, envcmd.Wrap(&EndpointCommand{}))
	c.Assert(err, gc.IsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Equals, "")
	output := string(ctx.Stdout.(*bytes.Buffer).Bytes())
	info := s.APIInfo(c)
	c.Assert(output, gc.Equals, fmt.Sprintf("%s\n", info.Addrs[0]))
}

// modifyAddresses adds more endpoint addresses or removes all
// in case of nil.
func (s *EndpointSuite) modifyAddresses(c *gc.C, addresses []string) {
	env, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	info, err := s.ConfigStore.ReadInfo(env.Name())
	c.Assert(err, gc.IsNil)
	endpoint := info.APIEndpoint()
	if len(addresses) == 0 {
		// Remove all endpoint addresses.
		endpoint.Addresses = []string{}
	} else {
		// Add additional addresses.
		endpoint.Addresses = append(endpoint.Addresses, addresses...)
	}
	info.SetAPIEndpoint(endpoint)
	err = info.Write()
	c.Assert(err, gc.IsNil)
}
