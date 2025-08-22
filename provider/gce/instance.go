// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/errors"
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/gce/internal/google"
)

type environInstance struct {
	base *compute.Instance
	env  *environ
}

var _ instances.Instance = (*environInstance)(nil)

func newInstance(base *compute.Instance, env *environ) *environInstance {
	return &environInstance{
		base: base,
		env:  env,
	}
}

// Id implements instances.Instance.
func (inst *environInstance) Id() instance.Id {
	return instance.Id(inst.base.Name)
}

// Status implements instances.Instance.
func (inst *environInstance) Status(ctx context.ProviderCallContext) instance.Status {
	instStatus := inst.base.Status
	var jujuStatus status.Status
	switch instStatus {
	case google.StatusProvisioning, google.StatusStaging:
		jujuStatus = status.Provisioning
	case google.StatusRunning:
		jujuStatus = status.Running
	case google.StatusStopped, google.StatusTerminated:
		jujuStatus = status.Empty
	default:
		jujuStatus = status.Empty
	}
	return instance.Status{
		Status:  jujuStatus,
		Message: instStatus,
	}
}

func extractAddresses(interfaces ...*compute.NetworkInterface) []corenetwork.ProviderAddress {
	var addresses []corenetwork.ProviderAddress

	for _, netif := range interfaces {
		// Add public addresses.
		for _, accessConfig := range netif.AccessConfigs {
			if accessConfig.NatIP == "" {
				continue
			}
			address := corenetwork.ProviderAddress{
				MachineAddress: corenetwork.MachineAddress{
					Value: accessConfig.NatIP,
					Type:  corenetwork.IPv4Address,
					Scope: corenetwork.ScopePublic,
				},
			}
			addresses = append(addresses, address)
		}

		// Add private address.
		if netif.NetworkIP == "" {
			continue
		}
		address := corenetwork.ProviderAddress{
			MachineAddress: corenetwork.MachineAddress{
				Value: netif.NetworkIP,
				Type:  corenetwork.IPv4Address,
				Scope: corenetwork.ScopeCloudLocal,
			},
		}
		addresses = append(addresses, address)
	}

	return addresses
}

// Addresses implements instances.Instance.
func (inst *environInstance) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	return extractAddresses(inst.base.NetworkInterfaces...), nil
}

func findInst(id instance.Id, instances []instances.Instance) instances.Instance {
	for _, inst := range instances {
		if id == inst.Id() {
			return inst
		}
	}
	return nil
}

// firewall stuff

// OpenPorts opens the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) OpenPorts(ctx context.ProviderCallContext, machineID string, rules firewall.IngressRules) error {
	// TODO(ericsnow) Make sure machineId matches inst.Id()?
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return errors.Trace(err)
	}
	err = inst.env.OpenPorts(ctx, name, rules)
	return google.HandleCredentialError(errors.Trace(err), ctx)
}

// ClosePorts closes the given ports on the instance, which
// should have been started with the given machine id.
func (inst *environInstance) ClosePorts(ctx context.ProviderCallContext, machineID string, rules firewall.IngressRules) error {
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return errors.Trace(err)
	}
	err = inst.env.ClosePorts(ctx, name, rules)
	return google.HandleCredentialError(errors.Trace(err), ctx)
}

// IngressRules returns the set of ingress rules applicable to the instance, which
// should have been started with the given machine id.
// The rules are returned as sorted by SortIngressRules.
func (inst *environInstance) IngressRules(ctx context.ProviderCallContext, machineID string) (firewall.IngressRules, error) {
	name, err := inst.env.namespace.Hostname(machineID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ports, err := inst.env.IngressRules(ctx, name)
	return ports, google.HandleCredentialError(errors.Trace(err), ctx)
}
