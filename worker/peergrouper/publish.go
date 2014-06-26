// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

type publisher struct {
	st *state.State
}

func newPublisher(st *state.State) *publisher {
	return &publisher{
		st: st,
	}
}

func (pub *publisher) publishAPIServers(apiServers [][]network.HostPort, instanceIds []instance.Id) error {
	if len(apiServers) == 0 {
		return errors.Errorf("no api servers specified")
	}
	// TODO(rog) publish instanceIds in environment storage.
	return pub.st.SetAPIHostPorts(apiServers)
}
