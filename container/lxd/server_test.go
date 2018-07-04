// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	client "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
)

type serverSuite struct {
	lxdtesting.BaseSuite
}

var _ = gc.Suite(&serverSuite{})

func (s *serverSuite) TestUpdateServerConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	updateReq := api.ServerPut{Config: map[string]interface{}{"key1": "val1"}}
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(&api.Server{}, lxdtesting.ETag, nil).Times(2),
		cSvr.EXPECT().UpdateServer(updateReq, lxdtesting.ETag).Return(nil),
	)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.UpdateServerConfig(map[string]string{"key1": "val1"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestUpdateContainerConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := lxdtesting.NewMockContainerServer(ctrl)

	cName := "juju-lxd-1"
	newConfig := map[string]string{"key1": "val1"}
	updateReq := api.ContainerPut{Config: newConfig}
	op := lxdtesting.NewMockOperation(ctrl)
	gomock.InOrder(
		cSvr.EXPECT().GetServer().Return(&api.Server{}, lxdtesting.ETag, nil),
		cSvr.EXPECT().GetContainer(cName).Return(&api.Container{}, lxdtesting.ETag, nil),
		cSvr.EXPECT().UpdateContainer(cName, updateReq, lxdtesting.ETag).Return(op, nil),
		op.EXPECT().Wait().Return(nil),
	)
	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	err = jujuSvr.UpdateContainerConfig("juju-lxd-1", newConfig)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestHasProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	cSvr.EXPECT().GetProfileNames().Return([]string{"default", "custom"}, nil).Times(2)

	jujuSvr, err := lxd.NewServer(cSvr)
	c.Assert(err, jc.ErrorIsNil)

	has, err := jujuSvr.HasProfile("default")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(has, jc.IsTrue)

	has, err = jujuSvr.HasProfile("unknown")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(has, jc.IsFalse)
}

func (s *serverSuite) TestCreateProfileWithConfig(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	cSvr := s.NewMockServer(ctrl)

	req := api.ProfilesPost{
		Name: "custom",
		ProfilePut: api.ProfilePut{
			Config: map[string]string{
				"boot.autostart": "false",
			},
		},
	}
	cSvr.EXPECT().CreateProfile(req).Return(nil)

	jujuSvr, err := lxd.NewServer(cSvr)
	err = jujuSvr.CreateProfileWithConfig("custom", map[string]string{"boot.autostart": "false"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serverSuite) TestIsSimpleStreamsServerWithMockServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxdtesting.NewMockContainerServer(ctrl)
	res := lxd.IsSimpleStreamsServer(svr)
	c.Assert(res, gc.Equals, false)
}

func (s *serverSuite) TestIsSimpleStreamsServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := &client.ProtocolSimpleStreams{}
	res := lxd.IsSimpleStreamsServer(svr)
	c.Assert(res, gc.Equals, true)
}

func (s *serverSuite) TestIsSupportedAPIVersion(c *gc.C) {
	for _, t := range []struct {
		input    string
		expected bool
		output   string
	}{
		{
			input:    "foo",
			expected: false,
			output:   `LXD API version "foo": expected format <major>\.<minor>`,
		},
		{
			input:    "a.b",
			expected: false,
			output:   `LXD API version "a.b": unexpected major number: strconv.(ParseInt|Atoi): parsing "a": invalid syntax`,
		},
		{
			input:    "0.9",
			expected: false,
			output:   `LXD API version "0.9": expected major version 1 or later`,
		},
		{
			input:    "1.0",
			expected: true,
			output:   "",
		},
		{
			input:    "2.0",
			expected: true,
			output:   "",
		},
		{
			input:    "2.1",
			expected: true,
			output:   "",
		},
	} {
		msg, ok := lxd.IsSupportedAPIVersion(t.input)
		c.Assert(ok, gc.Equals, t.expected)
		c.Assert(msg, gc.Matches, t.output)
	}
}
