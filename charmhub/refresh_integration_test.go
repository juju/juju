// +build integration

// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"github.com/kr/pretty"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type RefreshClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RefreshClientSuite{})

func (s *RefreshClientSuite) TestLiveRefreshRequest(c *gc.C) {
	c.Skip("It works on the cli with curl using the created req.Body.Reader data.")
	logger := &FakeLogger{}

	config, err := CharmHubConfig(logger)
	c.Assert(err, jc.ErrorIsNil)
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	refreshPath, err := basePath.Join("refresh")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := NewAPIRequester(DefaultHTTPTransport(logger), logger)
	restClient := NewHTTPRESTClient(apiRequester, nil)

	client := NewRefreshClient(refreshPath, restClient, logger)

	charmConfig, err := RefreshOne("wordpress", 0, "latest/stable", RefreshBase{
		Channel:      "18.04",
		Name:         "ubuntu",
		Architecture: "amd64",
	})
	c.Assert(err, jc.ErrorIsNil)
	charmConfig = DefineID(c, charmConfig, "mny7cXFEre1BFZQnXyyyIhCHBpiLTRNi")

	response, err := client.Refresh(context.TODO(), charmConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response, gc.HasLen, 1)
	c.Assert(response[0].Result, gc.Equals, "refresh", gc.Commentf("%s", pretty.Sprint(response)))
}

func (s *RefreshClientSuite) TestLiveRefreshManyRequest(c *gc.C) {
	c.Skip("It works on the cli with curl using the created req.Body.Reader data.")
	logger := &FakeLogger{}

	config, err := CharmHubConfig(logger)
	c.Assert(err, jc.ErrorIsNil)
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	refreshPath, err := basePath.Join("refresh")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := NewAPIRequester(DefaultHTTPTransport(logger), logger)
	restClient := NewHTTPRESTClient(apiRequester, nil)

	client := NewRefreshClient(refreshPath, restClient, logger)

	wordpressConfig, err := RefreshOne("wordpress", 0, "latest/stable", RefreshBase{
		Name:         "ubuntu",
		Channel:      "18.04",
		Architecture: "amd64",
	})
	c.Assert(err, jc.ErrorIsNil)
	wordpressConfig = DefineID(c, wordpressConfig, "mny7cXFEre1BFZQnXyyyIhCHBpiLTRNi")

	mysqlConfig, err := RefreshOne("mysql", 58, "latest/candidate", RefreshBase{
		Name:         "ubuntu",
		Channel:      "18.04",
		Architecture: "amd64",
	})
	c.Assert(err, jc.ErrorIsNil)
	mysqlConfig = DefineID(c, mysqlConfig, "XcESKcQ4R00AM6dOUpCl9YY4QpAEjnXe")

	charmsConfig := RefreshMany(wordpressConfig, mysqlConfig)

	response, err := client.Refresh(context.TODO(), charmsConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response, gc.HasLen, 2)
	c.Assert(response[0].Result, gc.Equals, "refresh", gc.Commentf("[0] failed %s", pretty.Sprint(response)))
	c.Assert(response[1].Result, gc.Equals, "refresh", gc.Commentf("[1] failed %s", pretty.Sprint(response)))
}

func (s *RefreshClientSuite) TestLiveInstallRequest(c *gc.C) {
	logger := &FakeLogger{}

	config, err := CharmHubConfig(logger)
	c.Assert(err, jc.ErrorIsNil)
	basePath, err := config.BasePath()
	c.Assert(err, jc.ErrorIsNil)

	refreshPath, err := basePath.Join("refresh")
	c.Assert(err, jc.ErrorIsNil)

	apiRequester := NewAPIRequester(DefaultHTTPTransport(logger), logger)
	restClient := NewHTTPRESTClient(apiRequester, nil)

	client := NewRefreshClient(refreshPath, restClient, logger)

	charmConfig, err := InstallOneFromRevision("wordpress", 0, RefreshBase{
		Name:         "ubuntu",
		Channel:      "18.04",
		Architecture: "amd64",
	})
	c.Assert(err, jc.ErrorIsNil)

	response, err := client.Refresh(context.TODO(), charmConfig)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response[0].Result, gc.Equals, "install", gc.Commentf("%s", pretty.Sprint(response)))
}

func DefineID(c *gc.C, config RefreshConfig, id string) RefreshConfig {
	switch t := config.(type) {
	case refreshOne:
		t.ID = id
		return t
	default:
		c.Fatalf("unexpected config %T", config)
	}
	return nil
}
