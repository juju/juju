// build integration

// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub"
)

type ClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

func (s *ClientSuite) TestLiveInfoRequest(c *gc.C) {
	config, err := charmhub.CharmHubConfig(&charmhub.FakeLogger{})
	c.Assert(err, jc.ErrorIsNil)

	client, err := charmhub.NewClient(config)
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.Info(context.TODO(), "ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Name, gc.Equals, "ubuntu")
}

func (s *ClientSuite) TestLiveFindRequest(c *gc.C) {
	config, err := charmhub.CharmHubConfig(&charmhub.FakeLogger{})
	c.Assert(err, jc.ErrorIsNil)

	client, err := charmhub.NewClient(config)
	c.Assert(err, jc.ErrorIsNil)

	responses, err := client.Find(context.TODO(), "ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(responses), jc.GreaterThan, 1)
	c.Assert(responses[0].Name, gc.Equals, "ubuntu")
}
