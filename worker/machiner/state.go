// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package machiner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v2/api/agent/machiner"
	"github.com/juju/juju/v2/core/life"
	"github.com/juju/juju/v2/core/network"
	"github.com/juju/juju/v2/core/status"
	"github.com/juju/juju/v2/core/watcher"
	"github.com/juju/juju/v2/rpc/params"
)

type MachineAccessor interface {
	Machine(names.MachineTag) (Machine, error)
}

type Machine interface {
	Refresh() error
	Life() life.Value
	EnsureDead() error
	SetMachineAddresses(addresses []network.MachineAddress) error
	SetStatus(machineStatus status.Status, info string, data map[string]interface{}) error
	Watch() (watcher.NotifyWatcher, error)
	SetObservedNetworkConfig(netConfig []params.NetworkConfig) error
}

type APIMachineAccessor struct {
	State *machiner.State
}

func (a APIMachineAccessor) Machine(tag names.MachineTag) (Machine, error) {
	m, err := a.State.Machine(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}
