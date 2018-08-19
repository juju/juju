// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type lxdInstance struct {
	id     string
	server lxd.ContainerServer
}

var _ instance.Instance = (*lxdInstance)(nil)

// Id implements instance.Instance.Id.
func (lxd *lxdInstance) Id() instance.Id {
	return instance.Id(lxd.id)
}

func (*lxdInstance) Refresh() error {
	return nil
}

func (lxd *lxdInstance) Addresses(ctx context.ProviderCallContext) ([]network.Address, error) {
	return nil, errors.NotImplementedf("lxdInstance.Addresses")
}

// Status implements instance.Instance.Status.
func (lxd *lxdInstance) Status(ctx context.ProviderCallContext) instance.InstanceStatus {
	jujuStatus := status.Pending
	instStatus, _, err := lxd.server.GetContainerState(lxd.id)
	if err != nil {
		return instance.InstanceStatus{
			Status:  status.Empty,
			Message: fmt.Sprintf("could not get status: %v", err),
		}
	}
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
	return instance.InstanceStatus{
		Status:  jujuStatus,
		Message: instStatus.Status,
	}
}

// OpenPorts implements instance.Instance.OpenPorts.
func (lxd *lxdInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts implements instance.Instance.ClosePorts.
func (lxd *lxdInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	return fmt.Errorf("not implemented")
}

// IngressRules implements instance.Instance.IngressRules.
func (lxd *lxdInstance) IngressRules(ctx context.ProviderCallContext, machineId string) ([]network.IngressRule, error) {
	return nil, fmt.Errorf("not implemented")
}

// Add a string representation of the id.
func (lxd *lxdInstance) String() string {
	return fmt.Sprintf("lxd:%s", lxd.id)
}
