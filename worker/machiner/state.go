// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package machiner

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/machiner"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/names"
)

type MachineAccessor interface {
	Machine(names.MachineTag) (Machine, error)
}

type Machine interface {
	Refresh() error
	Life() params.Life
	EnsureDead() error
	SetMachineAddresses(addresses []network.Address) error
	SetStatus(status params.Status, info string, data map[string]interface{}) error
	Watch() (watcher.NotifyWatcher, error)
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
