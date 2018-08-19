// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"fmt"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type kvmInstance struct {
	container Container
	id        string
}

var _ instance.Instance = (*kvmInstance)(nil)

// Id implements instance.Instance.Id.
func (kvm *kvmInstance) Id() instance.Id {
	return instance.Id(kvm.id)
}

// Status implements instance.Instance.Status.
func (kvm *kvmInstance) Status(ctx context.ProviderCallContext) instance.InstanceStatus {
	if kvm.container.IsRunning() {
		return instance.InstanceStatus{
			Status:  status.Running,
			Message: "running",
		}
	}
	return instance.InstanceStatus{
		Status:  status.Stopped,
		Message: "stopped",
	}
}

func (*kvmInstance) Refresh() error {
	return nil
}

func (kvm *kvmInstance) Addresses(ctx context.ProviderCallContext) ([]network.Address, error) {
	logger.Errorf("kvmInstance.Addresses not implemented")
	return nil, nil
}

// OpenPorts implements instance.Instance.OpenPorts.
func (kvm *kvmInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts implements instance.Instance.ClosePorts.
func (kvm *kvmInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	return fmt.Errorf("not implemented")
}

// IngressRules implements instance.Instance.IngressRules.
func (kvm *kvmInstance) IngressRules(ctx context.ProviderCallContext, machineId string) ([]network.IngressRule, error) {
	return nil, fmt.Errorf("not implemented")
}

// Add a string representation of the id.
func (kvm *kvmInstance) String() string {
	return fmt.Sprintf("kvm:%s", kvm.id)
}
