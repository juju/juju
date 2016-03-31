// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/network"
	"github.com/juju/names"
)

type mockAPIConnection struct {
	api.Connection
	info          *api.Info
	opts          api.DialOpts
	addr          string
	apiHostPorts  [][]network.HostPort
	controllerTag names.ModelTag
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

func (m *mockAPIConnection) ControllerTag() (names.ModelTag, error) {
	if m.controllerTag.Id() == "" {
		return m.controllerTag, errors.New("no server tag")
	}
	return m.controllerTag, nil
}

func (m *mockAPIConnection) SetPassword(username, password string) error {
	m.username = username
	m.password = password
	return nil
}
