// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	client "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	containerLXD "github.com/juju/juju/container/lxd"
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

	factory, server, interfaceAddr := s.newLocalServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil),
		server.EXPECT().EnableHTTPSListener().Return(nil),
		server.EXPECT().LocalBridgeName().Return(bridgeName),
		interfaceAddr.EXPECT().InterfaceAddress(bridgeName).Return(hostAddress, nil),
		server.EXPECT().GetConnectionInfo().Return(connectionInfo, nil),
	)

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
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

	factory, server, interfaceAddr := s.newLocalServerFactory(ctrl)

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

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
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

	factory, server, interfaceAddr := s.newLocalServerFactory(ctrl)

	server.EXPECT().GetProfile("default").Return(profile, etag, nil).Times(31)
	server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil).Times(31)
	server.EXPECT().EnableHTTPSListener().Return(nil).Times(31)
	server.EXPECT().LocalBridgeName().Return(bridgeName)
	interfaceAddr.EXPECT().InterfaceAddress(bridgeName).Return(hostAddress, nil)
	server.EXPECT().GetConnectionInfo().Return(emptyConnectionInfo, nil).Times(30)

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.IsNil)
	c.Assert(err.Error(), gc.Equals, "LXD is not listening on address https://192.168.0.1 (reported addresses: [])")
}

func (s *serverSuite) TestRemoteServerWithEmptyEndpointYieldsLocalServer(c *gc.C) {
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

	factory, server, interfaceAddr := s.newLocalServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil),
		server.EXPECT().EnableHTTPSListener().Return(nil),
		server.EXPECT().LocalBridgeName().Return(bridgeName),
		interfaceAddr.EXPECT().InterfaceAddress(bridgeName).Return(hostAddress, nil),
		server.EXPECT().GetConnectionInfo().Return(connectionInfo, nil),
	)

	svr, err := factory.RemoteServer(environs.CloudSpec{
		Endpoint: "",
	})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestRemoteServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory, server := s.newRemoteServerFactory(ctrl)

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	svr, err := factory.RemoteServer(environs.CloudSpec{
		Endpoint:   "https://10.0.0.9:8443",
		Credential: &creds,
	})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestRemoteServerMissingCertificates(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory, _ := s.newRemoteServerFactory(ctrl)

	creds := cloud.NewCredential("any", map[string]string{})
	svr, err := factory.RemoteServer(environs.CloudSpec{
		Endpoint:   "https://10.0.0.9:8443",
		Credential: &creds,
	})
	c.Assert(svr, gc.IsNil)
	c.Assert(errors.Cause(err).Error(), gc.Equals, "credentials not valid")
}

func (s *serverSuite) newLocalServerFactory(ctrl *gomock.Controller) (lxd.ServerFactory, *lxd.MockServer, *lxd.MockInterfaceAddress) {
	server := lxd.NewMockServer(ctrl)
	interfaceAddr := lxd.NewMockInterfaceAddress(ctrl)

	factory := lxd.NewServerFactoryWithMocks(
		func() (lxd.Server, error) {
			return server, nil
		},
		defaultRemoteServerFunc(ctrl),
		interfaceAddr,
		&lxd.MockClock{},
	)

	return factory, server, interfaceAddr
}

func (s *serverSuite) newRemoteServerFactory(ctrl *gomock.Controller) (lxd.ServerFactory, lxd.Server) {
	server := lxd.NewMockServer(ctrl)
	interfaceAddr := lxd.NewMockInterfaceAddress(ctrl)

	return lxd.NewServerFactoryWithMocks(
		defaultLocalServerFunc(ctrl),
		func(spec containerLXD.ServerSpec) (lxd.Server, error) {
			return server, nil
		},
		interfaceAddr,
		&lxd.MockClock{},
	), server
}

func defaultLocalServerFunc(ctrl *gomock.Controller) func() (lxd.Server, error) {
	return func() (lxd.Server, error) {
		return lxd.NewMockServer(ctrl), nil
	}
}

func defaultRemoteServerFunc(ctrl *gomock.Controller) func(containerLXD.ServerSpec) (lxd.Server, error) {
	return func(containerLXD.ServerSpec) (lxd.Server, error) {
		return lxd.NewMockServer(ctrl), nil
	}
}
