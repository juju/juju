// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	client "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/lxd"
)

var (
	_ = gc.Suite(&serverSuite{})
)

type serverSuite struct{}

func (s *serverSuite) TestLocalServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := &api.Profile{}
	etag := "etag"
	bridgeName := "lxdbr0"
	hostAddress := "192.168.0.1"
	connectionInfo := &client.ConnectionInfo{
		Addresses: []string{
			"https://192.168.0.1:8443",
		},
	}

	server := lxd.NewMockServer(ctrl)
	interfaceAddr := lxd.NewMockInterfaceAddress(ctrl)

	gomock.InOrder(
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil),
		server.EXPECT().EnableHTTPSListener().Return(nil),
		server.EXPECT().LocalBridgeName().Return(bridgeName),
		interfaceAddr.EXPECT().InterfaceAddress(bridgeName).Return(hostAddress, nil),
		server.EXPECT().GetConnectionInfo().Return(connectionInfo, nil),
	)

	factory := lxd.NewServerFactory(func() (lxd.Server, error) {
		return server, nil
	}, interfaceAddr, &lxd.MockClock{})

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestLocalServerRetrySemantics(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := &api.Profile{}
	etag := "etag"
	bridgeName := "lxdbr0"
	hostAddress := "192.168.0.1"
	emptyConnectionInfo := &client.ConnectionInfo{
		Addresses: []string{},
	}
	connectionInfo := &client.ConnectionInfo{
		Addresses: []string{
			"https://192.168.0.1:8443",
		},
	}

	server := lxd.NewMockServer(ctrl)
	interfaceAddr := lxd.NewMockInterfaceAddress(ctrl)

	gomock.InOrder(
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil),
		server.EXPECT().EnableHTTPSListener().Return(nil),
		server.EXPECT().LocalBridgeName().Return(bridgeName),
		interfaceAddr.EXPECT().InterfaceAddress(bridgeName).Return(hostAddress, nil),
		server.EXPECT().GetConnectionInfo().Return(emptyConnectionInfo, nil),
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil),
		server.EXPECT().EnableHTTPSListener().Return(nil),
		server.EXPECT().GetConnectionInfo().Return(connectionInfo, nil),
	)

	factory := lxd.NewServerFactory(func() (lxd.Server, error) {
		return server, nil
	}, interfaceAddr, &lxd.MockClock{})

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestLocalServerRetrySemanticsFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := &api.Profile{}
	etag := "etag"
	bridgeName := "lxdbr0"
	hostAddress := "192.168.0.1"
	emptyConnectionInfo := &client.ConnectionInfo{
		Addresses: []string{},
	}

	server := lxd.NewMockServer(ctrl)
	interfaceAddr := lxd.NewMockInterfaceAddress(ctrl)

	server.EXPECT().GetProfile("default").Return(profile, etag, nil).Times(31)
	server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil).Times(31)
	server.EXPECT().EnableHTTPSListener().Return(nil).Times(31)
	server.EXPECT().LocalBridgeName().Return(bridgeName)
	interfaceAddr.EXPECT().InterfaceAddress(bridgeName).Return(hostAddress, nil)
	server.EXPECT().GetConnectionInfo().Return(emptyConnectionInfo, nil).Times(30)

	factory := lxd.NewServerFactory(func() (lxd.Server, error) {
		return server, nil
	}, interfaceAddr, &lxd.MockClock{})

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.IsNil)
	c.Assert(err.Error(), gc.Equals, "LXD is not listening on address https://192.168.0.1 (reported addresses: [])")
}
