package juju_test

import (
	"github.com/juju/names"

	"github.com/juju/juju/api"
	"github.com/juju/juju/network"
)

type mockAPIState struct {
	api.Connection
	close func(api.Connection) error

	addr          string
	apiHostPorts  [][]network.HostPort
	modelTag      string
	controllerTag string
}

func (s *mockAPIState) Close() error {
	if s.close != nil {
		return s.close(s)
	}
	return nil
}

func (s *mockAPIState) Addr() string {
	return s.addr
}

func (s *mockAPIState) APIHostPorts() [][]network.HostPort {
	return s.apiHostPorts
}

func (s *mockAPIState) ModelTag() (names.ModelTag, error) {
	return names.ParseModelTag(s.modelTag)
}

func (s *mockAPIState) ControllerTag() (names.ModelTag, error) {
	return names.ParseModelTag(s.controllerTag)
}

func panicAPIOpen(apiInfo *api.Info, opts api.DialOpts) (api.Connection, error) {
	panic("api.Open called unexpectedly")
}
