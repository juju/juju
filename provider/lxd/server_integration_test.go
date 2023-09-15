// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"net"
	"net/url"
	"os"
	"syscall"

	client "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/provider/lxd"
)

var (
	_ = gc.Suite(&serverIntegrationSuite{})
)

// serverIntegrationSuite tests server module functionality from outside the
// lxd package. See server_test.go for package-local unit tests.
type serverIntegrationSuite struct {
	testing.IsolationSuite
}

func (s *serverIntegrationSuite) TestLocalServer(c *gc.C) {
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

	factory, server, interfaceAddr := lxd.NewLocalServerFactory(ctrl)

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
		server.EXPECT().ServerVersion().Return("5.2"),
	)

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverIntegrationSuite) TestLocalServerRetrySemantics(c *gc.C) {
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

	factory, server, interfaceAddr := lxd.NewLocalServerFactory(ctrl)

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
		server.EXPECT().ServerVersion().Return("5.2"),
	)

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverIntegrationSuite) TestLocalServerRetrySemanticsFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := &api.Profile{}
	etag := "etag"
	bridgeName := "lxdbr0"
	hostAddress := "192.168.0.1"
	emptyConnectionInfo := &client.ConnectionInfo{
		Addresses: []string{},
	}

	factory, server, interfaceAddr := lxd.NewLocalServerFactory(ctrl)

	server.EXPECT().GetProfile("default").Return(profile, etag, nil).Times(31)
	server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil).Times(31)
	server.EXPECT().EnableHTTPSListener().Return(nil).Times(31)
	server.EXPECT().LocalBridgeName().Return(bridgeName)
	interfaceAddr.EXPECT().InterfaceAddress(bridgeName).Return(hostAddress, nil)
	server.EXPECT().GetConnectionInfo().Return(emptyConnectionInfo, nil).Times(30)

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.IsNil)
	c.Assert(err, gc.NotNil)
	c.Assert(err.Error(), gc.Equals, "LXD is not listening on address https://192.168.0.1 (reported addresses: [])")
}

func (s *serverIntegrationSuite) TestLocalServerWithInvalidAPIVersion(c *gc.C) {
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

	factory, server, interfaceAddr := lxd.NewLocalServerFactory(ctrl)

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
		server.EXPECT().ServerVersion().Return("a.b"),
	)

	svr, err := factory.LocalServer()
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverIntegrationSuite) TestLocalServerErrorMessageShowsInstallMessage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory := lxd.NewServerFactoryWithMocks(
		func() (lxd.Server, error) {
			return nil, errors.New("bad")
		},
		lxd.DefaultRemoteServerFunc(ctrl),
		nil,
		&lxd.MockClock{},
	)

	_, err := factory.LocalServer()
	c.Assert(err.Error(), gc.Equals, `bad

Please install LXD by running:
	$ sudo snap install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`)
}

func (s *serverIntegrationSuite) TestLocalServerErrorMessageShowsConfigureMessage(c *gc.C) {
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
		lxd.DefaultRemoteServerFunc(ctrl),
		nil,
		&lxd.MockClock{},
	)

	_, err := factory.LocalServer()
	c.Assert(err.Error(), gc.Equals, `LXD refused connections; is LXD running?

Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`)
}

func (s *serverIntegrationSuite) TestLocalServerErrorMessageShowsConfigureMessageWhenEACCES(c *gc.C) {
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
		lxd.DefaultRemoteServerFunc(ctrl),
		nil,
		&lxd.MockClock{},
	)

	_, err := factory.LocalServer()
	c.Assert(err.Error(), gc.Equals, `Permission denied, are you in the lxd group?

Please configure LXD by running:
	$ newgrp lxd
	$ lxd init
`)
}

func (s *serverIntegrationSuite) TestLocalServerErrorMessageShowsInstallMessageWhenENOENT(c *gc.C) {
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
		lxd.DefaultRemoteServerFunc(ctrl),
		nil,
		&lxd.MockClock{},
	)

	_, err := factory.LocalServer()
	c.Assert(err.Error(), gc.Equals, `LXD socket not found; is LXD installed & running?

Please install LXD by running:
	$ sudo snap install lxd
and then configure it with:
	$ newgrp lxd
	$ lxd init
`)
}

func (s *serverIntegrationSuite) TestLocalServerWithStorageNotSupported(c *gc.C) {
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

	factory, server, interfaceAddr := lxd.NewLocalServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().VerifyNetworkDevice(profile, etag).Return(nil),
		server.EXPECT().EnableHTTPSListener().Return(nil),
		server.EXPECT().LocalBridgeName().Return(bridgeName),
		interfaceAddr.EXPECT().InterfaceAddress(bridgeName).Return(hostAddress, nil),
		server.EXPECT().GetConnectionInfo().Return(connectionInfo, nil),
		server.EXPECT().StorageSupported().Return(false),
		server.EXPECT().ServerVersion().Return("5.2"),
	)

	svr, err := factory.RemoteServer(lxd.CloudSpec{})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(err, gc.IsNil)
}

func (s *serverIntegrationSuite) TestRemoteServerWithEmptyEndpointYieldsLocalServer(c *gc.C) {
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

	factory, server, interfaceAddr := lxd.NewLocalServerFactory(ctrl)

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
		server.EXPECT().ServerVersion().Return("5.2"),
	)

	svr, err := factory.RemoteServer(lxd.CloudSpec{})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(err, gc.IsNil)
}

func (s *serverIntegrationSuite) TestRemoteServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	profile := &api.Profile{}
	etag := "etag"

	factory, server := lxd.NewRemoteServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().StorageSupported().Return(true),
		server.EXPECT().GetProfile("default").Return(profile, etag, nil),
		server.EXPECT().EnsureDefaultStorage(profile, etag).Return(nil),
		server.EXPECT().ServerVersion().Return("5.2"),
	)

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	svr, err := factory.RemoteServer(
		lxd.CloudSpec{
			CloudSpec: environscloudspec.CloudSpec{
				Endpoint:   "https://10.0.0.9:8443",
				Credential: &creds,
			},
		})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverIntegrationSuite) TestRemoteServerWithNoStorage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory, server := lxd.NewRemoteServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().StorageSupported().Return(false),
		server.EXPECT().ServerVersion().Return("5.2"),
	)

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	svr, err := factory.RemoteServer(
		lxd.CloudSpec{
			CloudSpec: environscloudspec.CloudSpec{
				Endpoint:   "https://10.0.0.9:8443",
				Credential: &creds,
			},
		})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverIntegrationSuite) TestInsecureRemoteServerDoesNotCallGetServer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory, server := lxd.NewRemoteServerFactory(ctrl)

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	svr, err := factory.InsecureRemoteServer(
		lxd.CloudSpec{
			CloudSpec: environscloudspec.CloudSpec{
				Endpoint:   "https://10.0.0.9:8443",
				Credential: &creds,
			},
		})
	c.Assert(svr, gc.Not(gc.IsNil))
	c.Assert(svr, gc.Equals, server)
	c.Assert(err, gc.IsNil)
}

func (s *serverIntegrationSuite) TestRemoteServerMissingCertificates(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory, _ := lxd.NewRemoteServerFactory(ctrl)

	creds := cloud.NewCredential("any", map[string]string{})
	svr, err := factory.RemoteServer(
		lxd.CloudSpec{
			CloudSpec: environscloudspec.CloudSpec{
				Endpoint:   "https://10.0.0.9:8443",
				Credential: &creds,
			},
		})
	c.Assert(svr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "credentials not valid")
}

func (s *serverIntegrationSuite) TestRemoteServerBadServerFuncError(c *gc.C) {
	factory := lxd.NewServerFactoryWithError()

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	svr, err := factory.RemoteServer(
		lxd.CloudSpec{
			CloudSpec: environscloudspec.CloudSpec{
				Endpoint:   "https://10.0.0.9:8443",
				Credential: &creds,
			},
		})
	c.Assert(svr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "oops")
}

func (s *serverIntegrationSuite) TestRemoteServerWithUnSupportedAPIVersion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	factory, server := lxd.NewRemoteServerFactory(ctrl)

	gomock.InOrder(
		server.EXPECT().StorageSupported().Return(false),
		server.EXPECT().ServerVersion().Return("4.0"),
	)

	creds := cloud.NewCredential("any", map[string]string{
		"client-cert": "client-cert",
		"client-key":  "client-key",
		"server-cert": "server-cert",
	})
	_, err := factory.RemoteServer(
		lxd.CloudSpec{
			CloudSpec: environscloudspec.CloudSpec{
				Endpoint:   "https://10.0.0.9:8443",
				Credential: &creds,
			},
		})
	c.Assert(err, gc.ErrorMatches, `LXD version has to be at least "5.0.0", but current version is only "4.0.0"`)
}

func (s *serverIntegrationSuite) TestIsSupportedAPIVersion(c *gc.C) {
	for _, t := range []struct {
		input  string
		output string
	}{
		{
			input:  "foo",
			output: `LXD API version "foo": expected format <major>\.<minor>`,
		},
		{
			input:  "a.b",
			output: `major version number  a not valid`,
		},
		{
			input:  "4.0",
			output: `LXD version has to be at least "5.0.0", but current version is only "4.0.0"`,
		},
		{
			input:  "5.0",
			output: "",
		},
	} {
		err := lxd.ValidateAPIVersion(t.input)
		if t.output == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, t.output)
		}
	}
}
