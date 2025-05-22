// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	containerLXD "github.com/juju/juju/internal/container/lxd"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package lxd -destination package_mock_test.go github.com/juju/juju/internal/provider/lxd Server,ServerFactory,InterfaceAddress,CertificateReadWriter,CertificateGenerator,LXCConfigReader
//go:generate go run go.uber.org/mock/mockgen -typed -package lxd -destination environs_mock_test.go github.com/juju/juju/environs CredentialInvalidator


// NewLocalServerFactory creates a factory with a local server method that
// returns a mock server.
// The factory, server and mocked local address are returned.
func NewLocalServerFactory(ctrl *gomock.Controller) (ServerFactory, *MockServer, *MockInterfaceAddress) {
	server := NewMockServer(ctrl)
	interfaceAddr := NewMockInterfaceAddress(ctrl)

	factory := NewServerFactoryWithMocks(
		func() (Server, error) { return server, nil },
		DefaultRemoteServerFunc(ctrl),
		interfaceAddr,
		&MockClock{},
	)

	return factory, server, interfaceAddr
}

// NewRemoteServerFactory creates a factory with a remote server method that
// returns a mock server. The factory and server are returned.
func NewRemoteServerFactory(ctrl *gomock.Controller) (ServerFactory, *MockServer) {
	server := NewMockServer(ctrl)
	interfaceAddr := NewMockInterfaceAddress(ctrl)

	return NewServerFactoryWithMocks(
		defaultLocalServerFunc(ctrl),
		func(spec containerLXD.ServerSpec) (Server, error) { return server, nil },
		interfaceAddr,
		&MockClock{},
	), server
}

// DefaultRemoteServerFunc returns a factory method that returns a new mock
// LXD server. Since the server is not assigned, no expectations can be set
// against it, meaning any usage of the server is an error condition.
func DefaultRemoteServerFunc(ctrl *gomock.Controller) func(containerLXD.ServerSpec) (Server, error) {
	return func(containerLXD.ServerSpec) (Server, error) {
		return NewMockServer(ctrl), nil
	}
}

func defaultLocalServerFunc(ctrl *gomock.Controller) func() (Server, error) {
	return func() (Server, error) {
		return NewMockServer(ctrl), nil
	}
}
