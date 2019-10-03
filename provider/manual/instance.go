// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/network"
)

type manualBootstrapInstance struct {
	host string
}

func (manualBootstrapInstance) Id() instance.Id {
	return BootstrapInstanceId
}

func (manualBootstrapInstance) Status(ctx context.ProviderCallContext) instance.Status {
	// We assume that if we are deploying in manual provider the
	// underlying machine is clearly running.
	return instance.Status{
		Status: status.Running,
	}
}

func (manualBootstrapInstance) Refresh(ctx context.ProviderCallContext) error {
	return nil
}

func (inst manualBootstrapInstance) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	addr, err := manual.HostAddress(inst.host)
	if err != nil {
		return nil, err
	}
	return []corenetwork.ProviderAddress{addr}, nil
}

func (manualBootstrapInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	return nil
}

func (manualBootstrapInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules []network.IngressRule) error {
	return nil
}

func (manualBootstrapInstance) IngressRules(ctx context.ProviderCallContext, machineId string) ([]network.IngressRule, error) {
	return nil, nil
}
