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

type InfoClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&InfoClientSuite{})

func (s *InfoClientSuite) TestLiveInfoRequest(c *gc.C) {
	config, err := charmhub.CharmHubConfig(&charmhub.FakeLogger{})
	c.Assert(err, jc.ErrorIsNil)
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	infoPath, err := basePath.Join("info")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := charmhub.NewAPIRequester(charmhub.DefaultHTTPTransport())
	restClient := charmhub.NewHTTPRESTClient(apiRequester, nil, &charmhub.FakeLogger{})

	client := charmhub.NewInfoClient(infoPath, restClient, &charmhub.FakeLogger{})
	response, err := client.Info(context.TODO(), "ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.Name, gc.Equals, "ubuntu")
}
