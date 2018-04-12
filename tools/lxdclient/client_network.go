// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/network"
)

type rawNetworkClient interface {
	NetworkCreate(name string, config map[string]string) error
	NetworkGet(name string) (api.Network, error)
}

type networkClient struct {
	raw       rawNetworkClient
	supported bool
}

// NetworkCreate creates the specified network.
func (c *networkClient) NetworkCreate(name string, config map[string]string) error {
	if !c.supported {
		return errors.NotSupportedf("network API not supported on this remote")
	}

	return c.raw.NetworkCreate(name, config)
}

// NetworkGet returns the specified network's configuration.
func (c *networkClient) NetworkGet(name string) (api.Network, error) {
	if !c.supported {
		return api.Network{}, errors.NotSupportedf("network API not supported on this remote")
	}

	return c.raw.NetworkGet(name)
}

type creator interface {
	rawNetworkClient
	ProfileDeviceAdd(profile, devname, devtype string, props []string) (*api.Response, error)
	ProfileConfig(profile string) (*api.Profile, error)
}

func checkBridgeConfig(client rawNetworkClient, bridge string) error {
	n, err := client.NetworkGet(bridge)
	if err != nil {
		return errors.Annotatef(err, "LXD %s network config", bridge)
	}
	ipv6AddressConfig := n.Config["ipv6.address"]
	if n.Managed && ipv6AddressConfig != "none" && ipv6AddressConfig != "" {
		return errors.Errorf(`juju doesn't support ipv6. Please disable LXD's IPV6:

	$ lxc network set %s ipv6.address none

and rebootstrap`, bridge)
	}

	return nil
}

// CreateDefaultBridgeInDefaultProfile creates a default bridge if it doesn't
// exist and (if necessary) inserts it into the default profile.
func CreateDefaultBridgeInDefaultProfile(client creator) error {
	/* create the default bridge if it doesn't exist */
	n, err := client.NetworkGet(network.DefaultLXDBridge)
	if err != nil {
		err := client.NetworkCreate(network.DefaultLXDBridge, map[string]string{
			"ipv6.address": "none",
			"ipv6.nat":     "false",
		})
		if err != nil {
			return err
		}

		n, err = client.NetworkGet(network.DefaultLXDBridge)
		if err != nil {
			return err
		}
	} else {
		if err := checkBridgeConfig(client, network.DefaultLXDBridge); err != nil {
			return err
		}
	}

	nicType := "macvlan"
	if n.Type == "bridge" {
		nicType = "bridged"
	}

	props := []string{fmt.Sprintf("nictype=%s", nicType), fmt.Sprintf("parent=%s", network.DefaultLXDBridge)}

	config, err := client.ProfileConfig("default")
	if err != nil {
		return err
	}

	_, ok := config.Devices["eth0"]
	if ok {
		/* don't configure an eth0 if it already exists */
		return nil
	}

	_, err = client.ProfileDeviceAdd("default", "eth0", "nic", props)
	if err != nil {
		return err
	}

	return nil
}
