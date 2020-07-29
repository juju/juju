// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"

	gomock "github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	path "github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
)

type RefreshSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RefreshSuite{})

func (s *RefreshSuite) TestRefresh(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"
	body := transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{{
			InstanceKey: "foo-bar",
			ID:          name,
			Revision:    1,
			Platform: transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "all",
			},
			TrackingChannel: "latest/stable",
		}},
		Actions: []transport.RefreshRequestAction{{
			Action:      "refresh",
			InstanceKey: "foo-bar",
			ID:          name,
		}},
	}

	config, err := RefreshOne(name, 1, "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	restClient := NewMockRESTClient(ctrl)
	s.expectPost(c, restClient, path, name, body)

	client := NewRefreshClient(path, restClient)
	responses, err := client.Refresh(context.TODO(), config)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(responses), gc.Equals, 1)
	c.Assert(responses[0].Name, gc.Equals, name)
}

func (s *RefreshSuite) TestRefreshFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	baseURL := MustParseURL(c, "http://api.foo.bar")

	path := path.MakePath(baseURL)
	name := "meshuggah"

	config, err := RefreshOne(name, 1, "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	restClient := NewMockRESTClient(ctrl)
	s.expectPostFailure(c, restClient)

	client := NewRefreshClient(path, restClient)
	_, err = client.Refresh(context.TODO(), config)
	c.Assert(err, gc.Not(jc.ErrorIsNil))
}

func (s *RefreshSuite) expectPost(c *gc.C, client *MockRESTClient, p path.Path, name string, body interface{}) {
	client.EXPECT().Post(gomock.Any(), p, body, gomock.Any()).Do(func(_ context.Context, _ path.Path, _ transport.RefreshRequest, responses *transport.RefreshResponses) {
		responses.Results = []transport.RefreshResponse{{
			InstanceKey: "foo-bar",
			Name:        name,
		}}
	}).Return(nil)
}

func (s *RefreshSuite) expectPostFailure(c *gc.C, client *MockRESTClient) {
	client.EXPECT().Post(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.Errorf("boom"))
}

func DefineInstanceKey(c *gc.C, config RefreshConfig, key string) RefreshConfig {
	switch t := config.(type) {
	case refreshOne:
		t.instanceKey = key
		return t
	case executeOne:
		t.instanceKey = key
		return t
	default:
		c.Fatalf("unexpected config %T", config)
	}
	return nil
}

type RefreshConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RefreshConfigSuite{})

func (s *RefreshConfigSuite) TestRefreshOneBuild(c *gc.C) {
	config, err := RefreshOne("foo", 1, "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	req, err := config.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(req, gc.DeepEquals, transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{{
			InstanceKey: "foo-bar",
			ID:          "foo",
			Revision:    1,
			Platform: transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "all",
			},
			TrackingChannel: "latest/stable",
		}},
		Actions: []transport.RefreshRequestAction{{
			Action:      "refresh",
			InstanceKey: "foo-bar",
			ID:          "foo",
		}},
	})
}

func (s *RefreshConfigSuite) TestRefreshOneEnsure(c *gc.C) {
	config, err := RefreshOne("foo", 1, "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	err = config.Ensure([]transport.RefreshResponse{{
		InstanceKey: "foo-bar",
	}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshConfigSuite) TestInstallOneBuild(c *gc.C) {
	revision := 1
	channel := "latest/stable"

	config, err := InstallOne("foo", revision, channel, "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	req, err := config.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(req, gc.DeepEquals, transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{},
		Actions: []transport.RefreshRequestAction{{
			Action:      "install",
			InstanceKey: "foo-bar",
			ID:          "foo",
			Revision:    &revision,
			Channel:     &channel,
			Platform: &transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "all",
			},
		}},
	})
}

func (s *RefreshConfigSuite) TestInstallOneEnsure(c *gc.C) {
	config, err := InstallOne("foo", 1, "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	err = config.Ensure([]transport.RefreshResponse{{
		InstanceKey: "foo-bar",
	}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshConfigSuite) TestInstallOneFromChannelBuild(c *gc.C) {
	channel := "latest/stable"

	config, err := InstallOneFromChannel("foo", channel, "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	req, err := config.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(req, gc.DeepEquals, transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{},
		Actions: []transport.RefreshRequestAction{{
			Action:      "install",
			InstanceKey: "foo-bar",
			ID:          "foo",
			Channel:     &channel,
			Platform: &transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "all",
			},
		}},
	})
}

func (s *RefreshConfigSuite) TestInstallOneFromChannelEnsure(c *gc.C) {
	config, err := InstallOneFromChannel("foo", "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	err = config.Ensure([]transport.RefreshResponse{{
		InstanceKey: "foo-bar",
	}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshConfigSuite) TestDownloadOneEnsure(c *gc.C) {
	config, err := DownloadOne("foo", 1, "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	err = config.Ensure([]transport.RefreshResponse{{
		InstanceKey: "foo-bar",
	}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshConfigSuite) TestDownloadOneFromChannelBuild(c *gc.C) {
	channel := "latest/stable"

	config, err := DownloadOneFromChannel("foo", channel, "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	req, err := config.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(req, gc.DeepEquals, transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{},
		Actions: []transport.RefreshRequestAction{{
			Action:      "download",
			InstanceKey: "foo-bar",
			ID:          "foo",
			Channel:     &channel,
			Platform: &transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "all",
			},
		}},
	})
}

func (s *RefreshConfigSuite) TestDownloadOneFromChannelEnsure(c *gc.C) {
	config, err := DownloadOneFromChannel("foo", "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)

	config = DefineInstanceKey(c, config, "foo-bar")

	err = config.Ensure([]transport.RefreshResponse{{
		InstanceKey: "foo-bar",
	}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshConfigSuite) TestRefreshManyBuild(c *gc.C) {
	config1, err := RefreshOne("foo", 1, "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)
	config1 = DefineInstanceKey(c, config1, "foo-bar")

	config2, err := RefreshOne("bar", 2, "latest/edge", "ubuntu", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	config2 = DefineInstanceKey(c, config2, "foo-baz")

	revision := 3
	channel := "1/stable"

	config3, err := InstallOne("baz", 3, "1/stable", "ubuntu", "disco")
	c.Assert(err, jc.ErrorIsNil)

	config3 = DefineInstanceKey(c, config3, "foo-taz")

	config := RefreshMany(config1, config2, config3)

	req, err := config.Build()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(req, gc.DeepEquals, transport.RefreshRequest{
		Context: []transport.RefreshRequestContext{{
			InstanceKey: "foo-bar",
			ID:          "foo",
			Revision:    1,
			Platform: transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "focal",
				Architecture: "all",
			},
			TrackingChannel: "latest/stable",
		}, {
			InstanceKey: "foo-baz",
			ID:          "bar",
			Revision:    2,
			Platform: transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "trusty",
				Architecture: "all",
			},
			TrackingChannel: "latest/edge",
		}},
		Actions: []transport.RefreshRequestAction{{
			Action:      "refresh",
			InstanceKey: "foo-bar",
			ID:          "foo",
		}, {
			Action:      "refresh",
			InstanceKey: "foo-baz",
			ID:          "bar",
		}, {
			Action:      "install",
			InstanceKey: "foo-taz",
			ID:          "baz",
			Platform: &transport.RefreshRequestPlatform{
				OS:           "ubuntu",
				Series:       "disco",
				Architecture: "all",
			},
			Revision: &revision,
			Channel:  &channel,
		}},
	})
}

func (s *RefreshConfigSuite) TestRefreshManyEnsure(c *gc.C) {
	config1, err := RefreshOne("foo", 1, "latest/stable", "ubuntu", "focal")
	c.Assert(err, jc.ErrorIsNil)
	config1 = DefineInstanceKey(c, config1, "foo-bar")

	config2, err := RefreshOne("bar", 2, "latest/edge", "ubuntu", "trusty")
	c.Assert(err, jc.ErrorIsNil)
	config2 = DefineInstanceKey(c, config2, "foo-baz")

	config := RefreshMany(config1, config2)

	err = config.Ensure([]transport.RefreshResponse{{
		InstanceKey: "foo-bar",
	}, {
		InstanceKey: "foo-baz",
	}})
	c.Assert(err, jc.ErrorIsNil)
}
