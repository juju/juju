// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/manual"
)

type manualBootstrapInstance struct {
	host string
}

func (manualBootstrapInstance) Id() instance.Id {
	return BootstrapInstanceId
}

func (manualBootstrapInstance) Status(ctx envcontext.ProviderCallContext) instance.Status {
	// We assume that if we are deploying in manual provider the
	// underlying machine is clearly running.
	return instance.Status{
		Status: status.Running,
	}
}

func (manualBootstrapInstance) Refresh(ctx envcontext.ProviderCallContext) error {
	return nil
}

func (inst manualBootstrapInstance) Addresses(ctx envcontext.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	addr, err := manual.HostAddress(inst.host)
	if err != nil {
		return nil, err
	}
	return []corenetwork.ProviderAddress{addr}, nil
}

func (manualBootstrapInstance) OpenPorts(ctx envcontext.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	return nil
}

func (manualBootstrapInstance) ClosePorts(ctx envcontext.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	return nil
}

func (manualBootstrapInstance) IngressRules(ctx envcontext.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	return nil, nil
}
