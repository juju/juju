package juju_test

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/network"
)

type mockAPIState struct {
	close func(juju.APIState) error

	apiHostPorts [][]network.HostPort
	environTag   string
}

func (s *mockAPIState) Close() error {
	if s.close != nil {
		return s.close(s)
	}
	return nil
}

func (s *mockAPIState) APIHostPorts() [][]network.HostPort {
	return s.apiHostPorts
}

func (s *mockAPIState) EnvironTag() string {
	return s.environTag
}

func panicAPIOpen(apiInfo *api.Info, opts api.DialOpts) (juju.APIState, error) {
	panic("api.Open called unexpectedly")
}
