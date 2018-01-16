// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools/lxdclient"
)

type environInstance struct {
	raw *lxdclient.Instance
	env *environ
}

var _ instance.Instance = (*environInstance)(nil)

func newInstance(raw *lxdclient.Instance, env *environ) *environInstance {
	return &environInstance{
		raw: raw,
		env: env,
	}
}

// Id implements instance.Instance.
func (inst *environInstance) Id() instance.Id {
	return instance.Id(inst.raw.Name)
}

// Status implements instance.Instance.
func (inst *environInstance) Status() instance.InstanceStatus {
	jujuStatus := status.Pending
	instStatus := inst.raw.Status()
	switch instStatus {
	case lxdclient.StatusStarting, lxdclient.StatusStarted:
		jujuStatus = status.Allocating
	case lxdclient.StatusRunning:
		jujuStatus = status.Running
	case lxdclient.StatusFreezing, lxdclient.StatusFrozen, lxdclient.StatusThawed, lxdclient.StatusStopping, lxdclient.StatusStopped:
		jujuStatus = status.Empty
	default:
		jujuStatus = status.Empty
	}
	return instance.InstanceStatus{
		Status:  jujuStatus,
		Message: instStatus,
	}

}

// Addresses implements instance.Instance.
func (inst *environInstance) Addresses() ([]network.Address, error) {
	return inst.env.raw.Addresses(inst.raw.Name)
}
