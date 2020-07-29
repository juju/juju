// +build integration

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

type FindClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FindClientSuite{})

func (s *FindClientSuite) TestLiveFindRequest(c *gc.C) {
	config := charmhub.CharmhubConfig()
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	findPath, err := basePath.Join("find")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := charmhub.NewAPIRequester(charmhub.DefaultHTTPTransport())
	restClient := charmhub.NewHTTPRESTClient(apiRequester, nil)

	client := charmhub.NewFindClient(findPath, restClient)
	responses, err := client.Find(context.TODO(), "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(responses), jc.GreaterThan, 1)
	c.Assert(responses[0].Name, gc.Equals, "wordpress")
}
