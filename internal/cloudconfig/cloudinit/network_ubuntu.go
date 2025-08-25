// Copyright 2013, 2015, 2018 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	corenetwork "github.com/juju/juju/core/network"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/network/netplan"
)

var logger = internallogger.GetLogger("juju.cloudconfig.cloudinit")

var (
	jujuNetplanFile = "/etc/netplan/99-juju.yaml"
)

// GenerateNetplan renders a netplan file for the input non-empty collection
// of interfaces.
// The matchHWAddr argument indicates whether to add a match stanza for the
// MAC address to each device.
func GenerateNetplan(interfaces corenetwork.InterfaceInfos, matchHWAddr bool) (string, error) {
	if len(interfaces) == 0 {
		return "", errors.Errorf("missing container network config")
	}
	logger.Debugf(context.TODO(), "generating netplan from %#v", interfaces)
	var netPlan netplan.Netplan
	netPlan.Network.Ethernets = make(map[string]netplan.Ethernet)
	netPlan.Network.Version = 2
	for _, info := range interfaces {
		var iface netplan.Ethernet
		cidr, err := info.PrimaryAddress().ValueWithMask()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return "", errors.Trace(err)
		}
		if cidr != "" {
			iface.Addresses = append(iface.Addresses, cidr)
		} else if info.ConfigType == corenetwork.ConfigDHCP {
			t := true
			iface.DHCP4 = &t
		}

		for _, dns := range info.DNSServers {
			// Netplan doesn't support IPv6 link-local addresses, so skip them.
			if strings.HasPrefix(dns, "fe80:") {
				continue
			}

			iface.Nameservers.Addresses = append(iface.Nameservers.Addresses, dns)
		}
		iface.Nameservers.Search = append(iface.Nameservers.Search, info.DNSSearchDomains...)

		if info.GatewayAddress.Value != "" {
			switch {
			case info.GatewayAddress.Type == corenetwork.IPv4Address:
				iface.Gateway4 = info.GatewayAddress.Value
			case info.GatewayAddress.Type == corenetwork.IPv6Address:
				iface.Gateway6 = info.GatewayAddress.Value
			}
		}

		if info.MTU != 0 && info.MTU != 1500 {
			iface.MTU = info.MTU
		}

		if matchHWAddr && info.MACAddress != "" {
			iface.Match = map[string]string{"macaddress": info.MACAddress}
		}

		for _, route := range info.Routes {
			route := netplan.Route{
				To:     route.DestinationCIDR,
				Via:    route.GatewayIP,
				Metric: &route.Metric,
			}
			iface.Routes = append(iface.Routes, route)
		}
		netPlan.Network.Ethernets[info.InterfaceName] = iface
	}
	out, err := netplan.Marshal(&netPlan)
	if err != nil {
		return "", errors.Trace(err)
	}

	return string(out), nil
}

// PreparedConfig holds all the necessary information to render a persistent
// network config to a file.
type PreparedConfig struct {
	InterfaceNames   []string
	AutoStarted      []string
	DNSServers       []string
	DNSSearchDomains []string
	NameToAddress    map[string]string
	NameToRoutes     map[string][]corenetwork.Route
	NameToMTU        map[string]int
	Gateway4Address  string
	Gateway6Address  string
}

// PrepareNetworkConfigFromInterfaces collects the necessary information to
// render a persistent network config from the given slice of
// network.InterfaceInfo. The result always includes the loopback interface.
func PrepareNetworkConfigFromInterfaces(interfaces corenetwork.InterfaceInfos) (*PreparedConfig, error) {
	dnsServers := set.NewStrings()
	dnsSearchDomains := set.NewStrings()
	gateway4Address := ""
	gateway6Address := ""
	namesInOrder := make([]string, 1, len(interfaces)+1)
	nameToAddress := make(map[string]string)
	nameToRoutes := make(map[string][]corenetwork.Route)
	nameToMTU := make(map[string]int)

	// Always include the loopback.
	namesInOrder[0] = "lo"
	autoStarted := set.NewStrings("lo")

	// We need to check if we have a host-provided default GW and use it.
	// Otherwise, we'll use the first device with a gateway address,
	// it'll be filled in the second loop.
	for _, info := range interfaces {
		if info.IsDefaultGateway {
			switch info.GatewayAddress.Type {
			case corenetwork.IPv4Address:
				gateway4Address = info.GatewayAddress.Value
			case corenetwork.IPv6Address:
				gateway6Address = info.GatewayAddress.Value
			}
		}
	}

	for _, info := range interfaces {
		ifaceName := strings.Replace(info.MACAddress, ":", "_", -1)
		// prepend eth because .format of python wont like a tag starting with numbers.
		ifaceName = fmt.Sprintf("{eth%s}", ifaceName)

		if !info.NoAutoStart {
			autoStarted.Add(ifaceName)
		}

		cidr, err := info.PrimaryAddress().ValueWithMask()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}
		if cidr != "" {
			nameToAddress[ifaceName] = cidr
		} else if info.ConfigType == corenetwork.ConfigDHCP {
			nameToAddress[ifaceName] = string(corenetwork.ConfigDHCP)
		}
		nameToRoutes[ifaceName] = info.Routes

		dnsServers = dnsServers.Union(set.NewStrings(info.DNSServers...))
		dnsSearchDomains = dnsSearchDomains.Union(set.NewStrings(info.DNSSearchDomains...))

		if info.GatewayAddress.Value != "" {
			switch {
			case gateway4Address == "" && info.GatewayAddress.Type == corenetwork.IPv4Address:
				gateway4Address = info.GatewayAddress.Value

			case gateway6Address == "" && info.GatewayAddress.Type == corenetwork.IPv6Address:
				gateway6Address = info.GatewayAddress.Value
			}
		}

		if info.MTU != 0 && info.MTU != 1500 {
			nameToMTU[ifaceName] = info.MTU
		}

		namesInOrder = append(namesInOrder, ifaceName)
	}

	prepared := &PreparedConfig{
		InterfaceNames:   namesInOrder,
		NameToAddress:    nameToAddress,
		NameToRoutes:     nameToRoutes,
		NameToMTU:        nameToMTU,
		AutoStarted:      autoStarted.SortedValues(),
		DNSServers:       dnsServers.SortedValues(),
		DNSSearchDomains: dnsSearchDomains.SortedValues(),
		Gateway4Address:  gateway4Address,
		Gateway6Address:  gateway6Address,
	}

	logger.Debugf(context.TODO(), "prepared network config for rendering: %+v", prepared)
	return prepared, nil
}

// AddNetworkConfig adds configuration scripts for specified interfaces
// to cloudconfig - using boot text files and boot commands.
func (cfg *ubuntuCloudConfig) AddNetworkConfig(interfaces corenetwork.InterfaceInfos) error {
	if len(interfaces) != 0 {
		netPlan, err := GenerateNetplan(interfaces, cfg.useNetplanHWAddrMatch)
		if err != nil {
			return errors.Trace(err)
		}
		cfg.AddBootTextFile(jujuNetplanFile, netPlan, 0644)
		cfg.AddBootCmd(populateNetworkInterfaces())
	}
	return nil
}

func populateNetworkInterfaces() string {
	return `
echo "Applying netplan configuration."
netplan generate
netplan apply
for i in {1..5}; do
  hostip=$(hostname -I)
  if [ -z "$hostip" ]; then
    sleep 1
  else
    echo "Got IP addresses $hostip"
    break
  fi
done
`[1:]
}

const CloudInitNetworkConfigDisabled = `config: "disabled"
`
