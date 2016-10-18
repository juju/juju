// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"net"

	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
	"github.com/juju/version"
)

type mockAPIState struct {
	api.Connection

	// If non-nil, close is called when the Close method is called.
	close func(api.Connection) error

	addr          string
	apiHostPorts  [][]network.HostPort
	modelTag      string
	controllerTag string
}

type mockedStateFlags int

const (
	noFlags        mockedStateFlags = 0x0000
	mockedHostPort mockedStateFlags = 0x0001
	mockedModelTag mockedStateFlags = 0x0002
)

// mockedAPIState returns a mocked-up implementation
// of api.Connection. The logical OR of the flags specifies
// whether to include a fake host port and model tag
// in the result.
func mockedAPIState(flags mockedStateFlags) *mockAPIState {
	hasHostPort := flags&mockedHostPort == mockedHostPort
	hasModelTag := flags&mockedModelTag == mockedModelTag
	addr := ""

	apiHostPorts := [][]network.HostPort{}
	if hasHostPort {
		var apiAddrs []network.Address
		ipv4Address := network.NewAddress("0.1.2.3")
		ipv6Address := network.NewAddress("2001:db8::1")
		addr = net.JoinHostPort(ipv4Address.Value, "1234")
		apiAddrs = append(apiAddrs, ipv4Address, ipv6Address)
		apiHostPorts = [][]network.HostPort{
			network.AddressesWithPort(apiAddrs, 1234),
		}
	}
	modelTag := ""
	if hasModelTag {
		modelTag = "model-df136476-12e9-11e4-8a70-b2227cce2b54"
	}
	return &mockAPIState{
		apiHostPorts:  apiHostPorts,
		modelTag:      modelTag,
		controllerTag: testing.ControllerTag.Id(),
		addr:          addr,
	}
}

func (s *mockAPIState) Close() error {
	if s.close != nil {
		return s.close(s)
	}
	return nil
}

func (s *mockAPIState) ServerVersion() (version.Number, bool) {
	return version.MustParse("1.2.3"), true
}

func (s *mockAPIState) Addr() string {
	return s.addr
}

func (s *mockAPIState) APIHostPorts() [][]network.HostPort {
	return s.apiHostPorts
}

func (s *mockAPIState) ModelTag() (names.ModelTag, bool) {
	if s.modelTag == "" {
		return names.ModelTag{}, false
	}
	t, err := names.ParseModelTag(s.modelTag)
	if err != nil {
		panic("bad model tag")
	}
	return t, true
}

func (s *mockAPIState) ControllerTag() names.ControllerTag {
	t, err := names.ParseControllerTag(s.controllerTag)
	if err != nil {
		panic("bad controller tag")
	}
	return t
}

func (s *mockAPIState) AuthTag() names.Tag {
	return names.NewUserTag("admin")
}

func (s *mockAPIState) ControllerAccess() string {
	return "superuser"
}

func panicAPIOpen(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
	panic("api.Open called unexpectedly")
}
