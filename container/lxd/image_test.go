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

func (t *imageSuite) patch(remotes map[string]lxdclient.ImageServer) {
	lxd.PatchConnectRemote(t, remotes)
}

func (s *imageSuite) TestCopyImageUsesPassedCallback(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockContainerServer(ctrl)

	copyOp := lxdtesting.NewMockRemoteOperation(ctrl)
	copyOp.EXPECT().Wait().Return(nil).AnyTimes()
	copyOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Success}, nil)
	copyOp.EXPECT().AddHandler(gomock.Any()).Return(nil, nil)

	image := lxdapi.Image{Filename: "this-is-our-image"}
	aliases := []lxdapi.ImageAlias{{Name: "local/image/alias"}}
	req := &lxdclient.ImageCopyArgs{Aliases: aliases}
	iSvr.EXPECT().CopyImage(iSvr, image, req).Return(copyOp, nil)

	jSvr := lxd.JujuImageServer{iSvr}
	sourced := lxd.SourcedImage{
		Image:     &image,
		LXDServer: iSvr,
	}
	err := jSvr.CopyRemoteImage(sourced, []string{"local/image/alias"}, noOpCallback)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *imageSuite) TestFindImageLocalServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockContainerServer(ctrl)

	alias := &lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}
	image := lxdapi.Image{Filename: "this-is-our-image"}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/xenial/amd64").Return(alias, "ETAG", nil),
		iSvr.EXPECT().GetImage("foo-target").Return(&image, "ETAG", nil),
	)

	jSvr := lxd.JujuImageServer{iSvr}
	found, err := jSvr.FindImage("xenial", "amd64", []lxd.RemoteServer{{}}, false, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.LXDServer, gc.Equals, iSvr)
	c.Check(*found.Image, gc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageLocalServerUnknownSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockContainerServer(ctrl)
	iSvr.EXPECT().GetImageAlias("juju/pldlinux/amd64").Return(nil, "ETAG", nil)

	jSvr := lxd.JujuImageServer{iSvr}
	_, err := jSvr.FindImage("pldlinux", "amd64", []lxd.RemoteServer{{}}, false, nil)
	c.Check(err, gc.ErrorMatches, `.*series: "pldlinux".*`)
}

func (s *imageSuite) TestFindImageRemoteServers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockContainerServer(ctrl)

	rSvr1 := lxdtesting.NewMockImageServer(ctrl)
	rSvr2 := lxdtesting.NewMockImageServer(ctrl)
	s.patch(map[string]lxdclient.ImageServer{
		"server-that-wont-work": rSvr1,
		"server-that-has-image": rSvr2,
	})

	image := lxdapi.Image{Filename: "this-is-our-image"}
	alias := lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-remote-target"}}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/xenial/amd64").Return(nil, "ETAG", nil),
		rSvr1.EXPECT().GetImageAlias("xenial/amd64").Return(nil, "ETAG", nil),
		rSvr2.EXPECT().GetImageAlias("xenial/amd64").Return(&alias, "ETAG", nil),
		rSvr2.EXPECT().GetImage("foo-remote-target").Return(&image, "ETAG", nil),
	)

	jSvr := lxd.JujuImageServer{iSvr}
	remotes := []lxd.RemoteServer{
		{Name: "server-that-wont-work", Protocol: lxd.LXDProtocol},
		{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol},
		{Name: "server-that-should-not-be-touched", Protocol: lxd.LXDProtocol},
	}
	found, err := jSvr.FindImage("xenial", "amd64", remotes, false, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.LXDServer, gc.Equals, rSvr2)
	c.Check(*found.Image, gc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageRemoteServersCopyLocalNoCallback(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockContainerServer(ctrl)

	rSvr := lxdtesting.NewMockImageServer(ctrl)
	s.patch(map[string]lxdclient.ImageServer{
		"server-that-has-image": rSvr,
	})

	copyOp := lxdtesting.NewMockRemoteOperation(ctrl)
	copyOp.EXPECT().Wait().Return(nil).AnyTimes()
	copyOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Success}, nil)

	localAlias := "juju/xenial/amd64"
	image := lxdapi.Image{Filename: "this-is-our-image"}
	alias := lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-remote-target"}}
	copyReq := &lxdclient.ImageCopyArgs{Aliases: []lxdapi.ImageAlias{{Name: localAlias}}}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias(localAlias).Return(nil, "ETAG", nil),
		rSvr.EXPECT().GetImageAlias("xenial/amd64").Return(&alias, "ETAG", nil),
		rSvr.EXPECT().GetImage("foo-remote-target").Return(&image, "ETAG", nil),
		iSvr.EXPECT().CopyImage(rSvr, image, copyReq).Return(copyOp, nil),
	)

	jSvr := lxd.JujuImageServer{iSvr}
	remotes := []lxd.RemoteServer{
		{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol},
	}
	found, err := jSvr.FindImage("xenial", "amd64", remotes, true, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.LXDServer, gc.Equals, iSvr)
	c.Check(*found.Image, gc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageRemoteServersNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := lxdtesting.NewMockContainerServer(ctrl)

	rSvr := lxdtesting.NewMockImageServer(ctrl)
	s.patch(map[string]lxdclient.ImageServer{
		"server-that-has-image": rSvr,
	})

	alias := lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-remote-target"}}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/centos7/amd64").Return(nil, "ETAG", nil),
		rSvr.EXPECT().GetImageAlias("centos/7/amd64").Return(&alias, "ETAG", nil),
		rSvr.EXPECT().GetImage("foo-remote-target").Return(nil, "ETAG", errors.New("failed to retrieve image")),
	)

	jSvr := lxd.JujuImageServer{iSvr}
	remotes := []lxd.RemoteServer{{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol}}
	_, err := jSvr.FindImage("centos7", "amd64", remotes, false, nil)
	c.Assert(err, gc.ErrorMatches, ".*failed to retrieve image.*")
}
