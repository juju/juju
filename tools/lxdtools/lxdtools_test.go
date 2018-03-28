// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdtools_test

import (
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/client/mocks"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools/lxdtools"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&lxdtoolsSuite{})

type lxdtoolsSuite struct {
	coretesting.BaseSuite
}

func (s *lxdtoolsSuite) TestLxdSocketPathLxdDirSet(c *gc.C) {
	os.Setenv("LXD_DIR", "foobar")
	s.PatchValue(lxdtools.OsStat, func(path string) (os.FileInfo, error) {
		return nil, nil
	})
	path := lxdtools.LxdSocketPath()
	c.Check(path, gc.Equals, "foobar/unix.socket")
}

func (s *lxdtoolsSuite) TestLxdSocketPathSnapSocketAndDebianSocketExists(c *gc.C) {
	os.Setenv("LXD_DIR", "")
	s.PatchValue(lxdtools.OsStat, func(path string) (os.FileInfo, error) {
		if path == "/var/snap/lxd/common/lxd" || path == "/var/lib/lxd/" {
			return nil, nil
		} else {
			return nil, errors.New("not found")
		}
	})
	path := lxdtools.LxdSocketPath()
	c.Check(path, gc.Equals, "/var/snap/lxd/common/lxd/unix.socket")
}

func (s *lxdtoolsSuite) TestLxdSocketPathNoSnapSocket(c *gc.C) {
	os.Setenv("LXD_DIR", "")
	s.PatchValue(lxdtools.OsStat, func(path string) (os.FileInfo, error) {
		if path == "/var/lib/lxd/" {
			return nil, nil
		} else {
			return nil, errors.New("not found")
		}
	})
	path := lxdtools.LxdSocketPath()
	c.Check(path, gc.Equals, "/var/lib/lxd/unix.socket")
}

func (s *lxdtoolsSuite) TestGetImageWithServerLocalImage(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	localImageServer := mocks.NewMockImageServer(mockCtrl)
	//	remoteImageServer := mocks.NewMockImageServer(mockCtrl)
	mockedImage := api.Image{Filename: "this-is-our-image"}

	gomock.InOrder(
		localImageServer.EXPECT().GetImageAlias("juju/xenial/amd64").Return(&api.ImageAliasesEntry{ImageAliasesEntryPut: api.ImageAliasesEntryPut{Target: "foo-target"}}, "ETAG", nil),
		localImageServer.EXPECT().GetImage("foo-target").Return(&mockedImage, "ETAG", nil),
	)
	server, image, target, err := lxdtools.GetImageWithServer(localImageServer, "xenial", "amd64", []lxdtools.RemoteServer{lxdtools.RemoteServer{}})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(server, gc.Equals, localImageServer)
	c.Check(*image, gc.DeepEquals, mockedImage)
	c.Check(target, gc.Equals, "foo-target")
}

func (s *lxdtoolsSuite) TestGetImageWithServerRemoteImageUnknownSeries(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	localImageServer := mocks.NewMockImageServer(mockCtrl)
	gomock.InOrder(
		localImageServer.EXPECT().GetImageAlias("juju/pldlinux/amd64").Return(nil, "ETAG", nil),
	)
	_, _, _, err := lxdtools.GetImageWithServer(localImageServer, "pldlinux", "amd64", []lxdtools.RemoteServer{lxdtools.RemoteServer{}})
	c.Check(err, gc.ErrorMatches, `.*series: "pldlinux".*`)
}

func (s *lxdtoolsSuite) TestGetImageWithServerRemoteImageWrongSeries(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	localImageServer := mocks.NewMockImageServer(mockCtrl)
	gomock.InOrder(
		localImageServer.EXPECT().GetImageAlias("juju/win2012hvr2/amd64").Return(nil, "ETAG", nil),
	)
	_, _, _, err := lxdtools.GetImageWithServer(localImageServer, "win2012hvr2", "amd64", []lxdtools.RemoteServer{lxdtools.RemoteServer{}})
	c.Check(err, gc.ErrorMatches, `.*series "win2012hvr2".*`)
}

func (s *lxdtoolsSuite) TestGetImageWithServerRemoteServers(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	localImageServer := mocks.NewMockImageServer(mockCtrl)
	remoteImageServer := mocks.NewMockImageServer(mockCtrl)
	mockedImage := api.Image{Filename: "this-is-our-image"}

	remoteServers := []lxdtools.RemoteServer{
		lxdtools.RemoteServer{Host: "wrong-protocol-server", Protocol: "FOOBAR"},
		lxdtools.RemoteServer{Host: "server-that-wont-work", Protocol: lxdtools.LXDProtocol},
		lxdtools.RemoteServer{Host: "server-that-has-image", Protocol: lxdtools.SimplestreamsProtocol},
		lxdtools.RemoteServer{Host: "server-that-should-not-be-touched", Protocol: lxdtools.LXDProtocol},
	}

	connectLXD := func(host string, args *lxd.ConnectionArgs) (lxd.ImageServer, error) {
		c.Assert(host, gc.Equals, "server-that-wont-work")
		return nil, errors.New("Won't work")
	}
	connectSS := func(host string, args *lxd.ConnectionArgs) (lxd.ImageServer, error) {
		c.Assert(host, gc.Equals, "server-that-has-image")
		return remoteImageServer, nil
	}
	s.PatchValue(lxdtools.LxdConnectPublicLXD, connectLXD)
	s.PatchValue(lxdtools.LxdConnectSimpleStreams, connectSS)

	gomock.InOrder(
		localImageServer.EXPECT().GetImageAlias("juju/xenial/amd64").Return(nil, "ETAG", nil),
		remoteImageServer.EXPECT().GetImageAlias("xenial/amd64").Return(&api.ImageAliasesEntry{ImageAliasesEntryPut: api.ImageAliasesEntryPut{Target: "foo-remote-target"}}, "ETAG", nil),
		remoteImageServer.EXPECT().GetImage("foo-remote-target").Return(&mockedImage, "ETAG", nil),
	)
	server, image, target, err := lxdtools.GetImageWithServer(localImageServer, "xenial", "amd64", remoteServers)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(server, gc.Equals, remoteImageServer)
	c.Check(*image, gc.DeepEquals, mockedImage)
	c.Check(target, gc.Equals, "foo-remote-target")
}

func (s *lxdtoolsSuite) TestGetImageWithServerRemoteServersOtherSeries(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	localImageServer := mocks.NewMockImageServer(mockCtrl)
	remoteImageServer := mocks.NewMockImageServer(mockCtrl)
	mockedImage := api.Image{Filename: "this-is-our-image"}

	remoteServers := []lxdtools.RemoteServer{
		lxdtools.RemoteServer{Host: "server-that-has-image", Protocol: lxdtools.SimplestreamsProtocol},
	}

	connectLXD := func(host string, args *lxd.ConnectionArgs) (lxd.ImageServer, error) {
		c.Error("There's no remote LXD server")
		return nil, nil
	}
	connectSS := func(host string, args *lxd.ConnectionArgs) (lxd.ImageServer, error) {
		c.Assert(host, gc.Equals, "server-that-has-image")
		return remoteImageServer, nil
	}
	s.PatchValue(lxdtools.LxdConnectPublicLXD, connectLXD)
	s.PatchValue(lxdtools.LxdConnectSimpleStreams, connectSS)

	gomock.InOrder(
		localImageServer.EXPECT().GetImageAlias("juju/centos7/amd64").Return(nil, "ETAG", nil),
		remoteImageServer.EXPECT().GetImageAlias("centos/7/amd64").Return(&api.ImageAliasesEntry{ImageAliasesEntryPut: api.ImageAliasesEntryPut{Target: "foo-remote-target"}}, "ETAG", nil),
		remoteImageServer.EXPECT().GetImage("foo-remote-target").Return(&mockedImage, "ETAG", nil),
		localImageServer.EXPECT().GetImageAlias("juju/opensuseleap/amd64").Return(nil, "ETAG", nil),
		remoteImageServer.EXPECT().GetImageAlias("opensuse/42.2/amd64").Return(&api.ImageAliasesEntry{ImageAliasesEntryPut: api.ImageAliasesEntryPut{Target: "foo-remote-target"}}, "ETAG", nil),
		remoteImageServer.EXPECT().GetImage("foo-remote-target").Return(&mockedImage, "ETAG", nil),
	)
	server, image, target, err := lxdtools.GetImageWithServer(localImageServer, "centos7", "amd64", remoteServers)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(server, gc.Equals, remoteImageServer)
	c.Check(*image, gc.DeepEquals, mockedImage)
	c.Check(target, gc.Equals, "foo-remote-target")

	server, image, target, err = lxdtools.GetImageWithServer(localImageServer, "opensuseleap", "amd64", remoteServers)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(server, gc.Equals, remoteImageServer)
	c.Check(*image, gc.DeepEquals, mockedImage)
	c.Check(target, gc.Equals, "foo-remote-target")
}
func (s *lxdtoolsSuite) TestGetImageWithServerRemoteServersFailure(c *gc.C) {
	mockCtrl := gomock.NewController(c)
	defer mockCtrl.Finish()
	localImageServer := mocks.NewMockImageServer(mockCtrl)
	remoteImageServer := mocks.NewMockImageServer(mockCtrl)

	remoteServers := []lxdtools.RemoteServer{
		lxdtools.RemoteServer{Host: "server-that-has-image", Protocol: lxdtools.SimplestreamsProtocol},
	}

	connectLXD := func(host string, args *lxd.ConnectionArgs) (lxd.ImageServer, error) {
		c.Error("There's no remote LXD server")
		return nil, nil
	}
	connectSS := func(host string, args *lxd.ConnectionArgs) (lxd.ImageServer, error) {
		c.Assert(host, gc.Equals, "server-that-has-image")
		return remoteImageServer, nil
	}
	s.PatchValue(lxdtools.LxdConnectPublicLXD, connectLXD)
	s.PatchValue(lxdtools.LxdConnectSimpleStreams, connectSS)

	gomock.InOrder(
		localImageServer.EXPECT().GetImageAlias("juju/centos7/amd64").Return(nil, "ETAG", nil),
		remoteImageServer.EXPECT().GetImageAlias("centos/7/amd64").Return(&api.ImageAliasesEntry{ImageAliasesEntryPut: api.ImageAliasesEntryPut{Target: "foo-remote-target"}}, "ETAG", nil),
		remoteImageServer.EXPECT().GetImage("foo-remote-target").Return(nil, "ETAG", errors.New("Failed to retrieve image")),
	)
	_, _, _, err := lxdtools.GetImageWithServer(localImageServer, "centos7", "amd64", remoteServers)
	c.Assert(err, gc.ErrorMatches, ".*Failed to retrieve image.*")
}
