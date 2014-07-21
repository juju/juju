// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package identityprovider_test

import (
	"testing"

	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/apiserver/identityprovider"
	coretesting "github.com/juju/juju/testing"
)

type AgentProviderSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&AgentProviderSuite{})

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}

func (s *AgentProviderSuite) TestLookupProviderFails(c *gc.C) {
	tag := names.NewRelationTag("wordpress:mysql")
	_, err := identityprovider.LookupProvider(tag)
	c.Assert(err, gc.ErrorMatches, "Tag type 'relation' does not have an identity provider")
}

func (s *AgentProviderSuite) TestLookupProvider(c *gc.C) {
	tag := names.NewUserTag("bob")
	provider, err := identityprovider.LookupProvider(tag)
	c.Assert(err, gc.IsNil)
	c.Assert(provider, gc.NotNil)
}
