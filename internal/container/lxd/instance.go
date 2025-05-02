// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"context"
	"fmt"

	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/instances"
)

type lxdInstance struct {
	id     string
	server lxd.InstanceServer
}

var _ instances.Instance = (*lxdInstance)(nil)

// Id implements instances.instance.Id.
func (lxd *lxdInstance) Id() instance.Id {
	return instance.Id(lxd.id)
}

func (*lxdInstance) Refresh() error {
	return nil
}

func (lxd *lxdInstance) Addresses(ctx context.Context) (corenetwork.ProviderAddresses, error) {
	return nil, errors.NotImplementedf("lxdInstance.Addresses")
}

// Status implements instances.Instance.Status.
func (lxd *lxdInstance) Status(ctx context.Context) instance.Status {
	instStatus, _, err := lxd.server.GetInstanceState(lxd.id)
	if err != nil {
		return instance.Status{
			Status:  status.Empty,
			Message: fmt.Sprintf("could not get status: %v", err),
		}
	}
	var jujuStatus status.Status
	switch instStatus.StatusCode {
	case api.Starting, api.Started:
		jujuStatus = status.Allocating
	case api.Running:
		jujuStatus = status.Running
	case api.Freezing, api.Frozen, api.Thawed, api.Stopping, api.Stopped:
		jujuStatus = status.Empty
	default:
		jujuStatus = status.Empty
	}
	return instance.Status{
		Status:  jujuStatus,
		Message: instStatus.Status,
	}
}

// OpenPorts implements instances.Instance.OpenPorts.
func (lxd *lxdInstance) OpenPorts(ctx context.Context, machineId string, rules firewall.IngressRules) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts implements instances.Instance.ClosePorts.
func (lxd *lxdInstance) ClosePorts(ctx context.Context, machineId string, rules firewall.IngressRules) error {
	return fmt.Errorf("not implemented")
}

// IngressRules implements instances.Instance.IngressRules.
func (lxd *lxdInstance) IngressRules(ctx context.Context, machineId string) (firewall.IngressRules, error) {
	return nil, fmt.Errorf("not implemented")
}

// Add a string representation of the id.
func (lxd *lxdInstance) String() string {
	return fmt.Sprintf("lxd:%s", lxd.id)
}
