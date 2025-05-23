// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"errors"
	"testing"
	"time"

	lxdclient "github.com/canonical/lxd/client"
	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/internal/container/lxd"
	"github.com/juju/juju/internal/container/lxd/mocks"
	lxdtesting "github.com/juju/juju/internal/container/lxd/testing"
)

func TestImageSuite(t *testing.T) {
	tc.Run(t, &imageSuite{})
}

type imageSuite struct {
	lxdtesting.BaseSuite
}

func (s *imageSuite) patch(remotes map[string]lxdclient.ImageServer) {
	lxd.PatchConnectRemote(s, remotes)
}

func (s *imageSuite) TestCopyImageUsesPassedCallback(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	copyOp := lxdtesting.NewMockRemoteOperation(ctrl)
	copyOp.EXPECT().Wait().Return(nil).AnyTimes()
	copyOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Success}, nil)
	copyOp.EXPECT().AddHandler(gomock.Any()).Return(nil, nil)

	image := lxdapi.Image{Filename: "this-is-our-image", Fingerprint: "fingerprint"}
	aliases := []lxdapi.ImageAlias{{Name: "local/image/alias"}}
	req := &lxdclient.ImageCopyArgs{Aliases: aliases, AutoUpdate: true}
	iSvr.EXPECT().CopyImage(iSvr, image, req).Return(copyOp, nil)
	iSvr.EXPECT().GetImageAliases().Return(nil, nil)

	s.expectAlias(iSvr, "local/image/alias", "fingerprint")

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, tc.ErrorIsNil)

	sourced := lxd.SourcedImage{
		Image:       &image,
		LXDServer:   iSvr,
		Fingerprint: image.Fingerprint,
	}
	err = jujuSvr.CopyRemoteImage(c.Context(), sourced, []string{"local/image/alias"}, lxdtesting.NoOpCallback)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *imageSuite) TestCopyImageRetries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	clock := mocks.NewMockClock(ctrl)
	after := make(chan time.Time, 2)
	after <- time.Time{}
	after <- time.Time{}
	clock.EXPECT().After(gomock.Any()).Return(after).AnyTimes()
	clock.EXPECT().Now().Return(time.Now()).AnyTimes()

	iSvr := s.NewMockServer(ctrl)
	image := lxdapi.Image{Filename: "this-is-our-image", Fingerprint: "fingerprint"}
	aliases := []lxdapi.ImageAlias{{Name: "local/image/alias"}}
	req := &lxdclient.ImageCopyArgs{Aliases: aliases, AutoUpdate: true}

	copyOp := lxdtesting.NewMockRemoteOperation(ctrl)
	copyOp.EXPECT().AddHandler(gomock.Any()).Return(nil, nil).AnyTimes()
	copyOp.EXPECT().Wait().Return(nil).Return(errors.New("Failed remote image download: boom"))
	copyOp.EXPECT().Wait().Return(nil).Return(errors.New("Failed remote image download: boom"))
	copyOp.EXPECT().Wait().Return(nil).Return(nil)
	copyOp.EXPECT().GetTarget().Return(&lxdapi.Operation{StatusCode: lxdapi.Success}, nil)

	iSvr.EXPECT().CopyImage(iSvr, image, req).Return(copyOp, nil).Times(3)
	iSvr.EXPECT().GetImageAliases().Return(nil, nil)

	s.expectAlias(iSvr, "local/image/alias", "fingerprint")

	jujuSvr, err := lxd.NewTestingServer(iSvr, clock)
	c.Assert(err, tc.ErrorIsNil)

	sourced := lxd.SourcedImage{
		Image:       &image,
		LXDServer:   iSvr,
		Fingerprint: image.Fingerprint,
	}
	err = jujuSvr.CopyRemoteImage(c.Context(), sourced, []string{"local/image/alias"}, lxdtesting.NoOpCallback)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *imageSuite) TestFindImageLocalServer(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	alias := &lxdapi.ImageAliasesEntry{Target: "foo-target"}
	image := lxdapi.Image{Filename: "this-is-our-image"}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/ubuntu@16.04/"+s.Arch()).Return(alias, lxdtesting.ETag, nil),
		iSvr.EXPECT().GetImage("foo-target").Return(&image, lxdtesting.ETag, nil),
	)

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, tc.ErrorIsNil)

	found, err := jujuSvr.FindImage(c.Context(), corebase.MakeDefaultBase("ubuntu", "16.04"), s.Arch(), instance.InstanceTypeContainer, []lxd.ServerSpec{{}}, false, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found.LXDServer, tc.Equals, iSvr)
	c.Check(*found.Image, tc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageLocalServerUnknownSeries(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)
	iSvr.EXPECT().GetImageAlias("juju/pldlinux@18.04/"+s.Arch()).Return(nil, lxdtesting.ETag, errors.New("not found"))

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, tc.ErrorIsNil)

	_, err = jujuSvr.FindImage(c.Context(), corebase.MakeDefaultBase("pldlinux", "18.04"), s.Arch(), instance.InstanceTypeContainer, []lxd.ServerSpec{{}}, false, nil)
	c.Check(err, tc.ErrorMatches, `base.*pldlinux.*`)
}

func (s *imageSuite) TestFindImageRemoteServers(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	rSvr1 := lxdtesting.NewMockImageServer(ctrl)
	rSvr2 := lxdtesting.NewMockImageServer(ctrl)
	s.patch(map[string]lxdclient.ImageServer{
		"server-that-wont-work": rSvr1,
		"server-that-has-image": rSvr2,
	})

	image := lxdapi.Image{
		AutoUpdate:  true,
		Filename:    "this-is-our-image",
		Fingerprint: "fingerprint",
	}

	const imageType = "container"
	alias := lxdapi.ImageAliasesEntry{Target: "foo-remote-target"}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/ubuntu@16.04/"+s.Arch()).Return(nil, lxdtesting.ETag, errors.New("not found")),
		rSvr1.EXPECT().GetImageAliasType(imageType, "16.04/"+s.Arch()).Return(nil, lxdtesting.ETag, errors.New("not found")),
		rSvr2.EXPECT().GetImageAliasType(imageType, "16.04/"+s.Arch()).Return(&alias, lxdtesting.ETag, nil),
		rSvr2.EXPECT().GetImage("foo-remote-target").Return(&image, lxdtesting.ETag, nil),
		iSvr.EXPECT().DeleteImageAlias("16.04/"+s.Arch()).Return(nil),
	)

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, tc.ErrorIsNil)

	remotes := []lxd.ServerSpec{
		{Name: "server-that-wont-work", Protocol: lxd.LXDProtocol},
		{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol},
		{Name: "server-that-should-not-be-touched", Protocol: lxd.LXDProtocol},
	}
	found, err := jujuSvr.FindImage(c.Context(), corebase.MakeDefaultBase("ubuntu", "16.04"), s.Arch(), instance.InstanceTypeContainer, remotes, false, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found.LXDServer, tc.Equals, rSvr2)
	c.Check(*found.Image, tc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageRemoteServersCopyLocalNoCallback(c *tc.C) {
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

	image := lxdapi.Image{
		AutoUpdate:  true,
		Filename:    "this-is-our-image",
		Fingerprint: "fingerprint",
	}

	localAlias := "juju/ubuntu@16.04/" + s.Arch()
	copyReq := &lxdclient.ImageCopyArgs{
		AutoUpdate: true,
		Aliases: []lxdapi.ImageAlias{
			{Name: "16.04/" + s.Arch()},
			{Name: localAlias},
		},
	}

	alias := lxdapi.ImageAliasesEntry{Target: "foo-remote-target"}

	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias(localAlias).Return(nil, lxdtesting.ETag, nil),
		rSvr.EXPECT().GetImageAliasType("container", "16.04/"+s.Arch()).Return(&alias, lxdtesting.ETag, nil),
		rSvr.EXPECT().GetImage("foo-remote-target").Return(&image, lxdtesting.ETag, nil),
		iSvr.EXPECT().CopyImage(rSvr, image, copyReq).Return(copyOp, nil),
		iSvr.EXPECT().GetImageAliases().Return(nil, nil),
		iSvr.EXPECT().DeleteImageAlias("16.04/"+s.Arch()).Return(nil),
	)

	s.expectAlias(iSvr, "16.04/"+s.Arch(), "fingerprint")
	s.expectAlias(iSvr, "juju/ubuntu@16.04/"+s.Arch(), "fingerprint")

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, tc.ErrorIsNil)

	remotes := []lxd.ServerSpec{
		{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol},
	}
	found, err := jujuSvr.FindImage(c.Context(), corebase.MakeDefaultBase("ubuntu", "16.04"), s.Arch(), instance.InstanceTypeContainer, remotes, true, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(found.LXDServer, tc.Equals, iSvr)
	c.Check(*found.Image, tc.DeepEquals, image)
}

func (s *imageSuite) TestFindImageRemoteServersNotFound(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	iSvr := s.NewMockServer(ctrl)

	rSvr := lxdtesting.NewMockImageServer(ctrl)
	s.patch(map[string]lxdclient.ImageServer{
		"server-that-has-image": rSvr,
	})

	alias := lxdapi.ImageAliasesEntry{Target: "foo-remote-target"}
	gomock.InOrder(
		iSvr.EXPECT().GetImageAlias("juju/ubuntu@18.04/"+s.Arch()).Return(nil, lxdtesting.ETag, errors.New("not found")),
		rSvr.EXPECT().GetImageAliasType("container", "18.04/"+s.Arch()).Return(&alias, lxdtesting.ETag, nil),
		rSvr.EXPECT().GetImage("foo-remote-target").Return(
			nil, lxdtesting.ETag, errors.New("failed to retrieve image")),
	)

	jujuSvr, err := lxd.NewServer(iSvr)
	c.Assert(err, tc.ErrorIsNil)

	remotes := []lxd.ServerSpec{{Name: "server-that-has-image", Protocol: lxd.SimpleStreamsProtocol}}
	_, err = jujuSvr.FindImage(c.Context(), corebase.MakeDefaultBase("ubuntu", "18.04"), s.Arch(), instance.InstanceTypeContainer, remotes, false, nil)
	c.Assert(err, tc.ErrorMatches, ".*failed to retrieve image.*")
}

func (s *imageSuite) TestConstructBaseRemoteAliasNotSupported(c *tc.C) {
	_, err := lxd.ConstructBaseRemoteAlias(corebase.MakeDefaultBase("centos", "7"), "arm64")
	c.Assert(err, tc.ErrorMatches, `base "centos@7" not supported`)

	_, err = lxd.ConstructBaseRemoteAlias(corebase.MakeDefaultBase("centos", "8"), "arm64")
	c.Assert(err, tc.ErrorMatches, `base "centos@8" not supported`)

	_, err = lxd.ConstructBaseRemoteAlias(corebase.MakeDefaultBase("opensuse", "opensuse42"), "s390x")
	c.Assert(err, tc.ErrorMatches, `base "opensuse@opensuse42" not supported`)
}

func (s *imageSuite) expectAlias(iSvr *lxdtesting.MockInstanceServer, name, target string) {
	var aliasPost lxdapi.ImageAliasesPost
	aliasPost.Name = name
	aliasPost.Target = target
	iSvr.EXPECT().CreateImageAlias(aliasPost).Return(nil)
}
