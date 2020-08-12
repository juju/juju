// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

type kvmInstance struct {
	container Container
	id        string
}

var _ instances.Instance = (*kvmInstance)(nil)

// Id implements instances.instance.Id.
func (kvm *kvmInstance) Id() instance.Id {
	return instance.Id(kvm.id)
}

// Status implements instances.Instance.Status.
func (kvm *kvmInstance) Status(ctx context.ProviderCallContext) instance.Status {
	if kvm.container.IsRunning() {
		return instance.Status{
			Status:  status.Running,
			Message: "running",
		}
	}
	return instance.Status{
		Status:  status.Stopped,
		Message: "stopped",
	}
}

func (*kvmInstance) Refresh() error {
	return nil
}

func (kvm *kvmInstance) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	logger.Errorf("kvmInstance.Addresses not implemented")
	return nil, nil
}

// OpenPorts implements instances.Instance.OpenPorts.
func (kvm *kvmInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts implements instances.Instance.ClosePorts.
func (kvm *kvmInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	return fmt.Errorf("not implemented")
}

// IngressRules implements instances.Instance.IngressRules.
func (kvm *kvmInstance) IngressRules(ctx context.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	return nil, fmt.Errorf("not implemented")
}

// Add a string representation of the id.
func (kvm *kvmInstance) String() string {
	return fmt.Sprintf("kvm:%s", kvm.id)
}
