// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"errors"

	lxdclient "github.com/canonical/lxd/client"
	lxdapi "github.com/canonical/lxd/shared/api"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
	corebase "github.com/juju/juju/core/base"
)

var _ = gc.Suite(&imageSuite{})

type imageSuite struct {
	lxdtesting.BaseSuite
}

func (s *imageSuite) patch(remotes map[string]lxdclient.ImageServer) {
	lxd.PatchConnectRemote(s, remotes)
}

func (s *imageSuite) TestCopyImageUsesPassedCallback(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	copyOp := lxdtesting.NewMockRemoteOperation(ctrl)
	copyOp.EXPECT().Wait().Return(nil).AnyTimes()
	copyOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Success}, nil)
	copyOp.EXPECT().AddHandler(gomock.Any()).Return(nil, nil)

	image := lxdapi.Image{Filename: "this-is-our-image"}
	aliases := []lxdapi.ImageAlias{{Name: "local/image/alias"}}
	req := &lxdclient.ImageCopyArgs{Aliases: aliases}
	iSvr.EXPECT().CopyImage(iSvr, image, req).Return(copyOp, nil)

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, jc.ErrorIsNil)

	sourced := lxd.SourcedImage{
		Image:     &image,
		LXDServer: iSvr,
	}
	err = jujuSvr.CopyRemoteImage(sourced, []string{"local/image/alias"}, lxdtesting.NoOpCallback)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *imageSuite) TestFindImageLocalServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	alias := &lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-target"}}
	image := lxdapi.Image{Filename: "this-is-our-image"}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/ubuntu@16.04/"+s.Arch()).Return(alias, lxdtesting.ETag, nil),
		iSvr.EXPECT().GetImage("foo-target").Return(&image, lxdtesting.ETag, nil),
	)

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, jc.ErrorIsNil)

	found, err := jujuSvr.FindImage(corebase.MakeDefaultBase("ubuntu", "16.04"), s.Arch(), lxdapi.InstanceTypeContainer, []lxd.ServerSpec{{}}, false, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.LXDServer, gc.Equals, iSvr)
	c.Check(*found.Image, gc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageLocalServerUnknownSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)
	iSvr.EXPECT().GetImageAlias("juju/pldlinux@18.04/"+s.Arch()).Return(nil, lxdtesting.ETag, errors.New("not found"))

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, jc.ErrorIsNil)

	_, err = jujuSvr.FindImage(corebase.MakeDefaultBase("pldlinux", "18.04"), s.Arch(), lxdapi.InstanceTypeContainer, []lxd.ServerSpec{{}}, false, nil)
	c.Check(err, gc.ErrorMatches, `base.*pldlinux.*`)
}

func (s *imageSuite) TestFindImageRemoteServers(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	rSvr1 := lxdtesting.NewMockImageServer(ctrl)
	rSvr2 := lxdtesting.NewMockImageServer(ctrl)
	s.patch(map[string]lxdclient.ImageServer{
		"server-that-wont-work": rSvr1,
		"server-that-has-image": rSvr2,
	})

	const imageType = "container"
	image := lxdapi.Image{Filename: "this-is-our-image"}
	alias := lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-remote-target"}}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/ubuntu@16.04/"+s.Arch()).Return(nil, lxdtesting.ETag, errors.New("not found")),
		rSvr1.EXPECT().GetImageAliasType(imageType, "16.04/"+s.Arch()).Return(nil, lxdtesting.ETag, errors.New("not found")),
		rSvr2.EXPECT().GetImageAliasType(imageType, "16.04/"+s.Arch()).Return(&alias, lxdtesting.ETag, nil),
		rSvr2.EXPECT().GetImage("foo-remote-target").Return(&image, lxdtesting.ETag, nil),
	)

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, jc.ErrorIsNil)

	remotes := []lxd.ServerSpec{
		{Name: "server-that-wont-work", Protocol: lxd.LXDProtocol},
		{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol},
		{Name: "server-that-should-not-be-touched", Protocol: lxd.LXDProtocol},
	}
	found, err := jujuSvr.FindImage(corebase.MakeDefaultBase("ubuntu", "16.04"), s.Arch(), lxdapi.InstanceTypeContainer, remotes, false, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.LXDServer, gc.Equals, rSvr2)
	c.Check(*found.Image, gc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageRemoteServersCopyLocalNoCallback(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	rSvr := lxdtesting.NewMockImageServer(ctrl)
	s.patch(map[string]lxdclient.ImageServer{
		"server-that-has-image": rSvr,
	})

	copyOp := lxdtesting.NewMockRemoteOperation(ctrl)
	copyOp.EXPECT().Wait().Return(nil).AnyTimes()
	copyOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Success}, nil)

	localAlias := "juju/ubuntu@16.04/" + s.Arch()
	image := lxdapi.Image{Filename: "this-is-our-image"}
	alias := lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-remote-target"}}
	copyReq := &lxdclient.ImageCopyArgs{Aliases: []lxdapi.ImageAlias{{Name: localAlias}}}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias(localAlias).Return(nil, lxdtesting.ETag, nil),
		rSvr.EXPECT().GetImageAliasType("container", "16.04/"+s.Arch()).Return(&alias, lxdtesting.ETag, nil),
		rSvr.EXPECT().GetImage("foo-remote-target").Return(&image, lxdtesting.ETag, nil),
		iSvr.EXPECT().CopyImage(rSvr, image, copyReq).Return(copyOp, nil),
	)

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, jc.ErrorIsNil)

	remotes := []lxd.ServerSpec{
		{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol},
	}
	found, err := jujuSvr.FindImage(corebase.MakeDefaultBase("ubuntu", "16.04"), s.Arch(), lxdapi.InstanceTypeContainer, remotes, true, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(found.LXDServer, gc.Equals, iSvr)
	c.Check(*found.Image, gc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageRemoteServersNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	rSvr := lxdtesting.NewMockImageServer(ctrl)
	s.patch(map[string]lxdclient.ImageServer{
		"server-that-has-image": rSvr,
	})

	alias := lxdapi.ImageAliasesEntry{ImageAliasesEntryPut: lxdapi.ImageAliasesEntryPut{Target: "foo-remote-target"}}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/ubuntu@18.04/"+s.Arch()).Return(nil, lxdtesting.ETag, errors.New("not found")),
		rSvr.EXPECT().GetImageAliasType("container", "18.04/"+s.Arch()).Return(&alias, lxdtesting.ETag, nil),
		rSvr.EXPECT().GetImage("foo-remote-target").Return(
			nil, lxdtesting.ETag, errors.New("failed to retrieve image")),
	)

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, jc.ErrorIsNil)

	remotes := []lxd.ServerSpec{{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol}}
	_, err = jujuSvr.FindImage(corebase.MakeDefaultBase("ubuntu", "18.04"), s.Arch(), lxdapi.InstanceTypeContainer, remotes, false, nil)
	c.Assert(err, gc.ErrorMatches, ".*failed to retrieve image.*")
}

func (s *imageSuite) TestBaseRemoteAliasesNotSupported(c *gc.C) {
	_, err := lxd.BaseRemoteAliases(corebase.MakeDefaultBase("centos", "7"), "arm64")
	c.Assert(err, gc.ErrorMatches, `base "centos@7" not supported`)

	_, err = lxd.BaseRemoteAliases(corebase.MakeDefaultBase("centos", "8"), "arm64")
	c.Assert(err, gc.ErrorMatches, `base "centos@8" not supported`)

	_, err = lxd.BaseRemoteAliases(corebase.MakeDefaultBase("opensuse", "opensuse42"), "s390x")
	c.Assert(err, gc.ErrorMatches, `base "opensuse@opensuse42" not supported`)
}
