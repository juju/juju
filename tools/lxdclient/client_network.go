// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/network"
)

type rawNetworkClient interface {
	CreateNetwork(post api.NetworksPost) error
	GetNetwork(name string) (network *api.Network, ETag string, err error)
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

	req := api.NetworksPost{
		Name:       name,
		NetworkPut: api.NetworkPut{Config: config},
	}
	return errors.Trace(c.raw.CreateNetwork(req))
}

// NetworkGet returns the specified network's configuration.
func (c *networkClient) NetworkGet(name string) (api.Network, error) {
	if !c.supported {
		return api.Network{}, errors.NotSupportedf("network API not supported on this remote")
	}

	n, _, err := c.raw.GetNetwork(name)
	return *n, errors.Trace(err)
}

type creator interface {
	rawNetworkClient
	GetProfile(name string) (profile *api.Profile, ETag string, err error)
	CreateProfile(profile api.ProfilesPost) (err error)
}

func checkBridgeConfig(client rawNetworkClient, bridge string) error {
	n, _, err := client.GetNetwork(bridge)
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
	n, _, err := client.GetNetwork(network.DefaultLXDBridge)
	if err != nil {
		req := api.NetworksPost{
			Name: network.DefaultLXDBridge,
			NetworkPut: api.NetworkPut{Config: map[string]string{
				"ipv6.address": "none",
				"ipv6.nat":     "false",
			}},
		}
		err := client.CreateNetwork(req)
		if err != nil {
			return errors.Trace(err)
		}

		n, _, err = client.GetNetwork(network.DefaultLXDBridge)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		if err := checkBridgeConfig(client, network.DefaultLXDBridge); err != nil {
			return errors.Trace(err)
		}
	}

	nicType := "macvlan"
	if n.Type == "bridge" {
		nicType = "bridged"
	}

	config, _, err := client.GetProfile("default")
	if err != nil {
		return errors.Trace(err)
	}
	_, ok := config.Devices["eth0"]
	if ok {
		/* don't configure an eth0 if it already exists */
		return errors.Trace(err)
	}

	req := api.ProfilesPost{
		Name: "default",
		ProfilePut: api.ProfilePut{
			Devices: map[string]map[string]string{
				"eth0": {
					"nictype": nicType,
					"parent":  network.DefaultLXDBridge,
				},
			},
		},
	}
	err = client.CreateProfile(req)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}
