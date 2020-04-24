// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/juju/api"
	"github.com/juju/names/v4"
)

// mockAPIConnection implements just enough of the api.Connection interface
// to satisfy the methods used by the register command.
type mockAPIConnection struct {
	// This will be nil - it's just there to satisfy the api.Connection
	// interface methods not explicitly defined by mockAPIConnection.
	api.Connection

	// addr is returned by Addr.
	addr string

	// controllerTag is returned by ControllerTag.
	controllerTag names.ControllerTag

	// authTag is returned by AuthTag.
	authTag names.Tag

	// controllerAccess is returned by ControllerAccess.
	controllerAccess string
}

func (*mockAPIConnection) Close() error {
	return nil
}

func (m *mockAPIConnection) Addr() string {
	return m.addr
}

func (m *mockAPIConnection) ControllerTag() names.ControllerTag {
	return m.controllerTag
}

func (m *mockAPIConnection) AuthTag() names.Tag {
	return m.authTag
}

func (m *mockAPIConnection) ControllerAccess() string {
	return m.controllerAccess
}
