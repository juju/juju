// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"context"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/manual"
)

type manualBootstrapInstance struct {
	host string
}

func (manualBootstrapInstance) Id() instance.Id {
	return BootstrapInstanceId
}

func (manualBootstrapInstance) Status(ctx context.Context) instance.Status {
	// We assume that if we are deploying in manual provider the
	// underlying machine is clearly running.
	return instance.Status{
		Status: status.Running,
	}
}

func (manualBootstrapInstance) Refresh(ctx context.Context) error {
	return nil
}

func (inst manualBootstrapInstance) Addresses(ctx context.Context) (corenetwork.ProviderAddresses, error) {
	addr, err := manual.HostAddress(inst.host)
	if err != nil {
		return nil, err
	}
	return []corenetwork.ProviderAddress{addr}, nil
}

func (manualBootstrapInstance) OpenPorts(ctx context.Context, machineId string, rules firewall.IngressRules) error {
	return nil
}

func (manualBootstrapInstance) ClosePorts(ctx context.Context, machineId string, rules firewall.IngressRules) error {
	return nil
}

func (manualBootstrapInstance) IngressRules(ctx context.Context, machineId string) (firewall.IngressRules, error) {
	return nil, nil
}
