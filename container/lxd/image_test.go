// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"errors"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	lxdclient "github.com/lxc/lxd/client"
	lxdapi "github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&imageSuite{})

type imageSuite struct {
	coretesting.BaseSuite
}

func (t *imageSuite) patch(svr lxdclient.ImageServer) {
	lxd.PatchConnectRemote(t, svr)
}

func (s *imageSuite) TestFindImageLocalServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockImageServer(ctrl)

	alias := &lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}
	image := lxdapi.Image{Filename: "this-is-our-image"}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/xenial/amd64").Return(alias, "ETAG", nil),
		iSvr.EXPECT().GetImage("foo-target").Return(&image, "ETAG", nil),
	)

	jSvr := lxd.JujuImageServer{iSvr}
	found, err := jSvr.FindImage("xenial", "amd64", []lxd.RemoteServer{{}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.LXDServer, gc.Equals, iSvr)
	c.Check(*found.Image, gc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageLocalServerUnknownSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockImageServer(ctrl)

	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/pldlinux/amd64").Return(nil, "ETAG", nil),
	)

	jSvr := lxd.JujuImageServer{iSvr}
	_, err := jSvr.FindImage("pldlinux", "amd64", []lxd.RemoteServer{{}})
	c.Check(err, gc.ErrorMatches, `.*series: "pldlinux".*`)
}

func (s *imageSuite) TestFindImageRemoteServers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockImageServer(ctrl)
	s.patch(iSvr)

	image := lxdapi.Image{Filename: "this-is-our-image"}
	alias := lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-remote-target"}}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/xenial/amd64").Return(nil, "ETAG", nil),
		iSvr.EXPECT().GetImageAlias("xenial/amd64").Return(nil, "ETAG", nil),
		iSvr.EXPECT().GetImageAlias("xenial/amd64").Return(&alias, "ETAG", nil),
		iSvr.EXPECT().GetImage("foo-remote-target").Return(&image, "ETAG", nil),
	)

	jSvr := lxd.JujuImageServer{iSvr}
	remotes := []lxd.RemoteServer{
		{Host: "server-that-wont-work", Protocol: lxd.LXDProtocol},
		{Host: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol},
		{Host: "server-that-should-not-be-touched", Protocol: lxd.LXDProtocol},
	}
	found, err := jSvr.FindImage("xenial", "amd64", remotes)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.LXDServer, gc.Equals, iSvr)
	c.Check(*found.Image, gc.DeepEquals, image)
	c.Check(*found.Remote, gc.DeepEquals, remotes[1])
}

func (s *imageSuite) TestFindImageRemoteServersNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockImageServer(ctrl)
	s.patch(iSvr)

	image := lxdapi.Image{Filename: "this-is-our-image"}
	alias := lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-remote-target"}}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/centos7/amd64").Return(nil, "ETAG", nil),
		iSvr.EXPECT().GetImageAlias("centos/7/amd64").Return(&alias, "ETAG", nil),
		iSvr.EXPECT().GetImage("foo-remote-target").Return(&image, "ETAG", errors.New("failed to retrieve image")),
	)

	jSvr := lxd.JujuImageServer{iSvr}
	remotes := []lxd.RemoteServer{{Host: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol}}
	_, err := jSvr.FindImage("centos7", "amd64", remotes)
	c.Assert(err, gc.ErrorMatches, ".*failed to retrieve image.*")

}
