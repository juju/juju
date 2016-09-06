// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/network"
	"gopkg.in/juju/names.v2"
)

type mockAPIConnection struct {
	api.Connection
	info          *api.Info
	opts          api.DialOpts
	addr          string
	apiHostPorts  [][]network.HostPort
	controllerTag names.ControllerTag
	username      string
	password      string
}

func (*mockAPIConnection) Close() error {
	return nil
}

func (m *mockAPIConnection) Addr() string {
	return m.addr
}

func (m *mockAPIConnection) APIHostPorts() [][]network.HostPort {
	return m.apiHostPorts
}

func (m *mockAPIConnection) ControllerTag() names.ControllerTag {
	return m.controllerTag
}

func (m *mockAPIConnection) SetPassword(username, password string) error {
	m.username = username
	m.password = password
	return nil
}
