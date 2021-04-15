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

type RefreshClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RefreshClientSuite{})

func (s *RefreshClientSuite) TestLiveRefreshRequest(c *gc.C) {
	c.Skip("refresh is broken so skip")

	logger := &charmhub.FakeLogger{}

	config, err := charmhub.CharmHubConfig(logger)
	c.Assert(err, jc.ErrorIsNil)
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	refreshPath, err := basePath.Join("refresh")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := charmhub.NewAPIRequester(charmhub.DefaultHTTPTransport(), logger)
	restClient := charmhub.NewHTTPRESTClient(apiRequester, nil)

	client := charmhub.NewRefreshClient(refreshPath, restClient, logger)

	charmConfig, err := charmhub.RefreshOne("wordpress", 0, "latest/stable", charmhub.RefreshBase{
		Channel:      "kubernetes",
		Architecture: "all",
	})
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.Refresh(context.TODO(), charmConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response, gc.HasLen, 1)
	c.Assert(response[0].Result, gc.Equals, "refresh")
}

func (s *RefreshClientSuite) TestLiveRefreshManyRequest(c *gc.C) {
	c.Skip("install is not currently wired up, so the test fails")

	logger := &charmhub.FakeLogger{}

	config, err := charmhub.CharmHubConfig(logger)
	c.Assert(err, jc.ErrorIsNil)
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	refreshPath, err := basePath.Join("refresh")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := charmhub.NewAPIRequester(charmhub.DefaultHTTPTransport(), logger)
	restClient := charmhub.NewHTTPRESTClient(apiRequester, nil)

	client := charmhub.NewRefreshClient(refreshPath, restClient, logger)

	wordpressConfig, err := charmhub.RefreshOne("wordpress", 16, "latest/stable", charmhub.RefreshBase{
		Name:         "ubuntu",
		Channel:      "focal",
		Architecture: "amd64",
	})
	c.Assert(err, jc.ErrorIsNil)

	mysqlConfig, err := charmhub.RefreshOne("mysql", 1, "latest/stable", charmhub.RefreshBase{
		Name:         "ubuntu",
		Channel:      "focal",
		Architecture: "amd64",
	})
	c.Assert(err, jc.ErrorIsNil)

	charmsConfig := charmhub.RefreshMany(wordpressConfig, mysqlConfig)

	response, err := client.Refresh(context.TODO(), charmsConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response, gc.HasLen, 2)
	c.Assert(response[0].Result, gc.Equals, "refresh")
	c.Assert(response[1].Result, gc.Equals, "refresh")
}

func (s *RefreshClientSuite) TestLiveInstallRequest(c *gc.C) {
	c.Skip("install is not currently wired up, so the test fails")

	logger := &charmhub.FakeLogger{}

	config, err := charmhub.CharmHubConfig(logger)
	c.Assert(err, jc.ErrorIsNil)
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	refreshPath, err := basePath.Join("refresh")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := charmhub.NewAPIRequester(charmhub.DefaultHTTPTransport(), logger)
	restClient := charmhub.NewHTTPRESTClient(apiRequester, nil)

	client := charmhub.NewRefreshClient(refreshPath, restClient, logger)

	charmConfig, err := charmhub.InstallOneFromRevision("wordpress", 16, charmhub.RefreshBase{
		Name:         "ubuntu",
		Channel:      "focal",
		Architecture: "amd64",
	})
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.Refresh(context.TODO(), charmConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response[0].Result, gc.Equals, "install")
}
