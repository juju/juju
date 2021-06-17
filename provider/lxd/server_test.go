// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"net"
	"net/url"
	"os"
	"syscall"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	client "github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	containerLXD "github.com/juju/juju/container/lxd"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/provider/lxd/mocks"
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
	serverInfo := &api.Server{
		ServerUntrusted: api.ServerUntrusted{
			APIVersion: "1.1",
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
		server.EXPECT().StorageSupported().Return(true),
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().EnsureDefaultStorage(profile, etag).Return(nil),
		server.EXPECT().GetServer().Return(serverInfo, etag, nil),
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
	serverInfo := &api.Server{
		ServerUntrusted: api.ServerUntrusted{
			APIVersion: "1.1",
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
		server.EXPECT().StorageSupported().Return(true),
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().EnsureDefaultStorage(profile, etag).Return(nil),
		server.EXPECT().GetServer().Return(serverInfo, etag, nil),
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

func (s *serverSuite) TestLocalServerWithInvalidAPIVersion(c *gc.C) {
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
	serverInfo := &api.Server{
		ServerUntrusted: api.ServerUntrusted{
			APIVersion: "a.b",
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
		server.EXPECT().StorageSupported().Return(true),
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().EnsureDefaultStorage(profile, etag).Return(nil),
		server.EXPECT().GetServer().Return(serverInfo, etag, nil),
	)

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestLocalServerErrorMessageShowsInstallMessage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := lxd.NewServerFactoryWithMocks(
		func() (lxd.Server, error) {
			return nil, errors.New("bad")
		},
		defaultRemoteServerFunc(ctrl),
		nil,
		&lxd.MockClock{},
	)

	_, err := factory.LocalServer()
	c.Assert(errors.Cause(err).Error(), gc.Equals, `bad

Please install LXD by running:
	$ sudo snap install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`)
}

func (s *serverSuite) TestLocalServerErrorMessageShowsConfigureMessage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := lxd.NewServerFactoryWithMocks(
		func() (lxd.Server, error) {
			return nil, errors.Annotatef(&url.Error{
				Err: &net.OpError{
					Op:  "dial",
					Net: "unix",
					Err: &os.SyscallError{
						Err: syscall.ECONNREFUSED,
					},
				},
			}, "bad")
		},
		defaultRemoteServerFunc(ctrl),
		nil,
		&lxd.MockClock{},
	)

	_, err := factory.LocalServer()
	c.Assert(errors.Cause(err).Error(), gc.Equals, `LXD refused connections; is LXD running?

Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`)
}

func (s *serverSuite) TestLocalServerErrorMessageShowsConfigureMessageWhenEACCES(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := lxd.NewServerFactoryWithMocks(
		func() (lxd.Server, error) {
			return nil, errors.Annotatef(&url.Error{
				Err: &net.OpError{
					Op:  "dial",
					Net: "unix",
					Err: &os.SyscallError{
						Err: syscall.EACCES,
					},
				},
			}, "bad")
		},
		defaultRemoteServerFunc(ctrl),
		nil,
		&lxd.MockClock{},
	)

	_, err := factory.LocalServer()
	c.Assert(errors.Cause(err).Error(), gc.Equals, `Permission denied, are you in the lxd group?

Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`)
}

func (s *serverSuite) TestLocalServerErrorMessageShowsInstallMessageWhenENOENT(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := lxd.NewServerFactoryWithMocks(
		func() (lxd.Server, error) {
			return nil, errors.Annotatef(&url.Error{
				Err: &net.OpError{
					Op:  "dial",
					Net: "unix",
					Err: &os.SyscallError{
						Err: syscall.ENOENT,
					},
				},
			}, "bad")
		},
		defaultRemoteServerFunc(ctrl),
		nil,
		&lxd.MockClock{},
	)

	_, err := factory.LocalServer()
	c.Assert(errors.Cause(err).Error(), gc.Equals, `LXD socket not found; is LXD installed & running?

Please install LXD by running:
	$ sudo snap install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`)
}

func (s *serverSuite) TestLocalServerWithStorageNotSupported(c *gc.C) {
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
	serverInfo := &api.Server{
		ServerUntrusted: api.ServerUntrusted{
			APIVersion: "2.2",
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
		server.EXPECT().StorageSupported().Return(false),
		server.EXPECT().GetServer().Return(serverInfo, etag, nil),
	)

	svr, err := factory.RemoteServer(environscloudspec.CloudSpec{
		Endpoint: "",
	})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(err, gc.IsNil)
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
	serverInfo := &api.Server{
		ServerUntrusted: api.ServerUntrusted{
			APIVersion: "1.1",
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
		server.EXPECT().StorageSupported().Return(true),
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().EnsureDefaultStorage(profile, etag).Return(nil),
		server.EXPECT().GetServer().Return(serverInfo, etag, nil),
	)

	svr, err := factory.RemoteServer(environscloudspec.CloudSpec{
		Endpoint: "",
	})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestRemoteServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := &api.Profile{}
	etag := "etag"
	serverInfo := &api.Server{
		ServerUntrusted: api.ServerUntrusted{
			APIVersion: "1.1",
		},
	}

	factory, server := s.newRemoteServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().StorageSupported().Return(true),
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().EnsureDefaultStorage(profile, etag).Return(nil),
		server.EXPECT().GetServer().Return(serverInfo, etag, nil),
	)

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	svr, err := factory.RemoteServer(environscloudspec.CloudSpec{
		Endpoint:   "https://10.0.0.9:8443",
		Credential: &creds,
	})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverSuite) TestRemoteServerWithNoStorage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	etag := "etag"
	serverInfo := &api.Server{
		ServerUntrusted: api.ServerUntrusted{
			APIVersion: "1.1",
		},
	}

	factory, server := s.newRemoteServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().StorageSupported().Return(false),
		server.EXPECT().GetServer().Return(serverInfo, etag, nil),
	)

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	svr, err := factory.RemoteServer(environscloudspec.CloudSpec{
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
	svr, err := factory.RemoteServer(environscloudspec.CloudSpec{
		Endpoint:   "https://10.0.0.9:8443",
		Credential: &creds,
	})
	c.Assert(svr, gc.IsNil)
	c.Assert(errors.Cause(err).Error(), gc.Equals, "credentials not valid")
}

func (s *serverSuite) TestRemoteServerWithGetServerError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory, server := s.newRemoteServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().StorageSupported().Return(false),
		server.EXPECT().GetServer().Return(nil, "", errors.New("bad")),
	)

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	_, err := factory.RemoteServer(environscloudspec.CloudSpec{
		Endpoint:   "https://10.0.0.9:8443",
		Credential: &creds,
	})
	c.Assert(errors.Cause(err).Error(), gc.Equals, "bad")
}

func (s *serverSuite) newLocalServerFactory(ctrl *gomock.Controller) (lxd.ServerFactory, *mocks.MockServer, *mocks.MockInterfaceAddress) {
	server := mocks.NewMockServer(ctrl)
	interfaceAddr := mocks.NewMockInterfaceAddress(ctrl)

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

func (s *serverSuite) newRemoteServerFactory(ctrl *gomock.Controller) (lxd.ServerFactory, *mocks.MockServer) {
	server := mocks.NewMockServer(ctrl)
	interfaceAddr := mocks.NewMockInterfaceAddress(ctrl)

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
		return mocks.NewMockServer(ctrl), nil
	}
}

func defaultRemoteServerFunc(ctrl *gomock.Controller) func(containerLXD.ServerSpec) (lxd.Server, error) {
	return func(containerLXD.ServerSpec) (lxd.Server, error) {
		return mocks.NewMockServer(ctrl), nil
	}
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
