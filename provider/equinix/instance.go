// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/packethost/packngo"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common"
)

type equinixDevice struct {
	e *environ

	*packngo.Device
	newInstanceConfigurator func(string) common.InstanceConfigurator
}

var (
	_ instances.Instance           = (*equinixDevice)(nil)
	_ instances.InstanceFirewaller = (*equinixDevice)(nil)
)

// newInstance returns a new equinixDevice
func newInstance(raw *packngo.Device, env *environ) *equinixDevice {
	return &equinixDevice{
		Device:                  raw,
		e:                       env,
		newInstanceConfigurator: common.NewSshInstanceConfigurator,
	}
}

func (device *equinixDevice) String() string {
	return device.ID
}

func (device *equinixDevice) Id() instance.Id {
	return instance.Id(device.ID)
}

func (device *equinixDevice) Status(ctx envcontext.ProviderCallContext) instance.Status {
	var jujuStatus status.Status

	switch device.State {
	case Active:
		jujuStatus = status.Running
	case Provisioning:
		jujuStatus = status.Pending
	case ShuttingDown, Stopped, Stopping, Terminated:
		jujuStatus = status.Empty
	default:
		jujuStatus = status.Empty
	}

	return instance.Status{
		Status:  jujuStatus,
		Message: device.State,
	}
}

// Addresses implements network.Addresses() returning generic address
// details for the instance, and requerying the equinix api if required.
func (device *equinixDevice) Addresses(ctx envcontext.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	var addresses []corenetwork.ProviderAddress

	for _, netw := range device.Network {
		address := corenetwork.ProviderAddress{}
		address.Value = netw.Address
		address.CIDR = fmt.Sprintf("%s/%d", netw.Network, netw.CIDR)

		if netw.Public {
			address.Scope = corenetwork.ScopePublic
		} else {
			address.Scope = corenetwork.ScopeCloudLocal
		}

		if netw.AddressFamily == 4 {
			address.Type = network.IPv4Address
		} else {
			address.Type = network.IPv6Address
			logger.Infof("skipping IPv6 Address %s", netw.Address)

			continue
		}

		addresses = append(addresses, address)
	}

	return addresses, nil
}

// OpenPorts (InstanceFirewaller) ensures that the input ingress rule is
// permitted for machine with the input ID.
func (device *equinixDevice) OpenPorts(ctx envcontext.ProviderCallContext, _ string, rules firewall.IngressRules) error {
	client, err := device.getInstanceConfigurator(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(client.ChangeIngressRules("", true, rules))
}

// OpenPorts (InstanceFirewaller) ensures that the input ingress rule is
// restricted for machine with the input ID.
func (device *equinixDevice) ClosePorts(ctx envcontext.ProviderCallContext, _ string, rules firewall.IngressRules) error {
	client, err := device.getInstanceConfigurator(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(client.ChangeIngressRules("", false, rules))
}

// IngressRules (InstanceFirewaller) returns the ingress rules that have been
// applied to the input machine ID.
func (device *equinixDevice) IngressRules(ctx envcontext.ProviderCallContext, _ string) (firewall.IngressRules, error) {
	client, err := device.getInstanceConfigurator(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	rules, err := client.FindIngressRules()
	return rules, errors.Trace(err)
}

func (device *equinixDevice) getInstanceConfigurator(
	ctx envcontext.ProviderCallContext,
) (common.InstanceConfigurator, error) {
	addresses, err := device.Addresses(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Try to find a public address.
	// Different models use different VCNs (and therefore subnets),
	// so the cloud-local IPs are no good if a controller is trying to
	// configure an instance in another model.
	for _, addr := range addresses {
		if addr.Scope == corenetwork.ScopePublic {
			return device.newInstanceConfigurator(addr.Value), nil
		}
	}

	return nil, errors.NotFoundf("public address for instance %q", device.Id())
}
