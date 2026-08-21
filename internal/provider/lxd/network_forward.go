// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"net"
	"time"

	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
)

const controllerAddressTimeout = 5 * time.Minute

func (env *environ) ensureControllerNetworkForward(
	ctx context.ProviderCallContext,
	container *lxd.Container,
	instanceConfig *instancecfg.InstanceConfig,
	controllerUUID string,
) error {
	network, ok, err := env.ovnNetwork()
	if err != nil || !ok {
		return errors.Trace(err)
	}

	targetAddress, err := env.waitForControllerAddress(ctx, container.Name, network)
	if err != nil {
		return errors.Trace(err)
	}

	controllerConfig := instanceConfig.ControllerConfig
	ports := []int{
		controllerConfig.SSHServerPort(),
		controllerConfig.APIPort(),
		controllerConfig.ControllerAPIPort(),
	}
	_, err = env.server().EnsureControllerNetworkForward(
		network.Name, controllerUUID, container.Name, targetAddress, ports,
	)
	return errors.Trace(err)
}

func (env *environ) controllerNetworkForwardAddress(
	controllerUUID, instanceID string,
) (string, bool, error) {
	network, ok, err := env.ovnNetwork()
	if err != nil || !ok {
		return "", false, errors.Trace(err)
	}
	return env.server().ControllerNetworkForwardAddress(
		network.Name, controllerUUID, instanceID,
	)
}

func (env *environ) deleteControllerNetworkForwards(
	controllerUUID, instanceID string,
) error {
	network, ok, err := env.ovnNetwork()
	if err != nil || !ok {
		return errors.Trace(err)
	}
	return errors.Trace(env.server().DeleteControllerNetworkForwards(
		network.Name, controllerUUID, instanceID,
	))
}

func (env *environ) ovnNetwork() (*lxdapi.Network, bool, error) {
	network, err := env.server().DefaultNetwork()
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return network, network.Type == "ovn", nil
}

func (env *environ) waitForControllerAddress(
	ctx context.ProviderCallContext,
	instanceID string,
	network *lxdapi.Network,
) (string, error) {
	_, subnet, err := net.ParseCIDR(network.Config["ipv4.address"])
	if err != nil {
		return "", errors.Annotatef(
			err, "parsing IPv4 address for OVN network %q", network.Name)
	}

	var targetAddress string
	err = retry.Call(retry.CallArgs{
		Func: func() error {
			addresses, err := env.server().ContainerAddresses(instanceID)
			if err != nil {
				return errors.Trace(err)
			}
			targetAddress = addressInSubnet(addresses, subnet)
			if targetAddress == "" {
				return errors.NotFoundf(
					"controller address in OVN network %q", network.Name)
			}
			return nil
		},
		Delay:       time.Second,
		Attempts:    retry.UnlimitedAttempts,
		MaxDuration: controllerAddressTimeout,
		Clock:       clock.WallClock,
		Stop:        ctx.Done(),
	})
	if err != nil {
		if ctx.Err() != nil {
			return "", errors.Trace(ctx.Err())
		}
		return "", errors.Trace(retry.LastError(err))
	}
	return targetAddress, nil
}

func addressInSubnet(addresses network.ProviderAddresses, subnet *net.IPNet) string {
	for _, address := range addresses {
		ip := net.ParseIP(address.Value)
		if ip != nil && ip.To4() != nil && subnet.Contains(ip) {
			return ip.String()
		}
	}
	return ""
}

func controllerForwardAddress(address string) network.ProviderAddress {
	return network.NewMachineAddress(
		address,
		network.WithScope(network.ScopePublic),
	).AsProviderAddress()
}

func isController(container *lxd.Container) (string, bool) {
	controllerUUID := container.Metadata(tags.JujuController)
	return controllerUUID, container.Metadata(tags.JujuIsController) == "true"
}
