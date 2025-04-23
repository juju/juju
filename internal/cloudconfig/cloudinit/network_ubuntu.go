// Copyright 2013, 2015, 2018 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	corenetwork "github.com/juju/juju/core/network"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/network/netplan"
)

var logger = internallogger.GetLogger("juju.cloudconfig.cloudinit")

var (
	systemNetworkInterfacesFile = "/etc/network/interfaces"
	networkInterfacesFile       = systemNetworkInterfacesFile + "-juju"
	jujuNetplanFile             = "/etc/netplan/99-juju.yaml"
)

// GenerateENITemplate renders an e/n/i template config for one or more network
// interfaces, using the given non-empty interfaces list.
func GenerateENITemplate(interfaces corenetwork.InterfaceInfos) (string, error) {
	if len(interfaces) == 0 {
		return "", errors.Errorf("missing container network config")
	}
	logger.Debugf(context.TODO(), "generating /e/n/i template from %#v", interfaces)

	prepared, err := PrepareNetworkConfigFromInterfaces(interfaces)
	if err != nil {
		return "", errors.Trace(err)
	}

	var output bytes.Buffer
	gateway4Handled := false
	gateway6Handled := false
	hasV4Interface := false
	hasV6Interface := false
	for _, name := range prepared.InterfaceNames {
		output.WriteString("\n")
		if name == "lo" {
			output.WriteString("auto ")
			autoStarted := strings.Join(prepared.AutoStarted, " ")
			output.WriteString(autoStarted + "\n\n")
			output.WriteString("iface lo inet loopback\n")

			dnsServers := strings.Join(prepared.DNSServers, " ")
			if dnsServers != "" {
				output.WriteString("  dns-nameservers ")
				output.WriteString(dnsServers + "\n")
			}

			dnsSearchDomains := strings.Join(prepared.DNSSearchDomains, " ")
			if dnsSearchDomains != "" {
				output.WriteString("  dns-search ")
				output.WriteString(dnsSearchDomains + "\n")
			}
			continue
		}

		address, hasAddress := prepared.NameToAddress[name]
		if !hasAddress {
			output.WriteString("iface " + name + " inet manual\n")
			continue
		} else if address == string(corenetwork.ConfigDHCP) {
			output.WriteString("iface " + name + " inet dhcp\n")
			// We're expecting to get a default gateway
			// from the DHCP lease.
			gateway4Handled = true
			continue
		}

		_, network, err := net.ParseCIDR(address)
		if err != nil {
			return "", errors.Annotatef(err, "invalid address for interface %q: %q", name, address)
		}

		isIpv4 := network.IP.To4() != nil

		if isIpv4 {
			output.WriteString("iface " + name + " inet static\n")
			hasV4Interface = true
		} else {
			output.WriteString("iface " + name + " inet6 static\n")
			hasV6Interface = true
		}
		output.WriteString("  address " + address + "\n")

		if isIpv4 {
			if !gateway4Handled && prepared.Gateway4Address != "" {
				gatewayIP := net.ParseIP(prepared.Gateway4Address)
				if network.Contains(gatewayIP) {
					output.WriteString("  gateway " + prepared.Gateway4Address + "\n")
					gateway4Handled = true // write it only once
				}
			}
		} else {
			if !gateway6Handled && prepared.Gateway6Address != "" {
				gatewayIP := net.ParseIP(prepared.Gateway6Address)
				if network.Contains(gatewayIP) {
					output.WriteString("  gateway " + prepared.Gateway6Address + "\n")
					gateway4Handled = true // write it only once
				}
			}
		}

		if mtu, ok := prepared.NameToMTU[name]; ok {
			output.WriteString(fmt.Sprintf("  mtu %d\n", mtu))
		}

		for _, route := range prepared.NameToRoutes[name] {
			output.WriteString(fmt.Sprintf("  post-up ip route add %s via %s metric %d\n",
				route.DestinationCIDR, route.GatewayIP, route.Metric))
			output.WriteString(fmt.Sprintf("  pre-down ip route del %s via %s metric %d\n",
				route.DestinationCIDR, route.GatewayIP, route.Metric))
		}
	}

	generatedConfig := output.String()
	logger.Debugf(context.TODO(), "generated network config:\n%s", generatedConfig)

	if hasV4Interface && !gateway4Handled {
		logger.Infof(context.TODO(), "generated network config has no ipv4 gateway")
	}

	if hasV6Interface && !gateway6Handled {
		logger.Infof(context.TODO(), "generated network config has no ipv6 gateway")
	}

	return generatedConfig, nil
}

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
	// Otherwise we'll use the first device with a gateway address,
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
// to cloudconfig - using boot text files and boot commands. It currently
// supports e/n/i and netplan.
func (cfg *ubuntuCloudConfig) AddNetworkConfig(interfaces corenetwork.InterfaceInfos) error {
	if len(interfaces) != 0 {
		eni, err := GenerateENITemplate(interfaces)
		if err != nil {
			return errors.Trace(err)
		}
		netPlan, err := GenerateNetplan(interfaces, cfg.useNetplanHWAddrMatch)
		if err != nil {
			return errors.Trace(err)
		}
		cfg.AddBootTextFile(jujuNetplanFile, netPlan, 0644)
		cfg.AddBootTextFile(systemNetworkInterfacesFile+".templ", eni, 0644)
		cfg.AddBootTextFile(systemNetworkInterfacesFile+".py", NetworkInterfacesScript, 0744)
		cfg.AddBootCmd(populateNetworkInterfaces(systemNetworkInterfacesFile))
	}
	return nil
}

// Note: we sleep to mitigate against LP #1337873 and LP #1269921.
// Note2: wait with anything that's hard to revert for as long as possible,
// we've seen weird failure modes and IMHO it's impossible to avoid them all,
// but we could do as much as we can to 1. avoid them 2. make the machine boot
// if we mess up
func populateNetworkInterfaces(networkFile string) string {
	s := `
if [ ! -f /sbin/ifup ]; then
  echo "No /sbin/ifup, applying netplan configuration."
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
else
  if [ -f /usr/bin/python ]; then
    python %[1]s.py --interfaces-file %[1]s --output-file %[1]s.out
  else
    python3 %[1]s.py --interfaces-file %[1]s --output-file %[1]s.out
  fi
  ifdown -a
  sleep 1.5
  mv %[1]s.out %[1]s
  ifup -a
fi
`
	return fmt.Sprintf(s, networkFile)
}

const NetworkInterfacesScript = `from __future__ import print_function, unicode_literals
import subprocess, re, argparse, os, time, shutil
from string import Formatter

INTERFACES_FILE="/etc/network/interfaces"
IP_LINE = re.compile(r"^\d+: (.*?):")
IP_HWADDR = re.compile(r".*link/ether ((\w{2}|:){11})")
COMMAND = "ip -oneline link"
RETRIES = 3
WAIT = 5

# Python3 vs Python2
try:
    strdecode = str.decode
except AttributeError:
    strdecode = str

def ip_parse(ip_output):
    """parses the output of the ip command
    and returns a hwaddr->nic-name dict"""
    devices = dict()
    print("Parsing ip command output {}".format(ip_output))
    for ip_line in ip_output:
        ip_line_str = strdecode(ip_line, "utf-8")
        match = IP_LINE.match(ip_line_str)
        if match is None:
            continue
        nic_name = match.group(1).split('@')[0]
        match = IP_HWADDR.match(ip_line_str)
        if match is None:
            continue
        nic_hwaddr = match.group(1)
        devices[nic_hwaddr] = nic_name
    print("Found the following devices: {}".format(devices))
    return devices

def replace_ethernets(interfaces_file, output_file, devices, fail_on_missing):
    """check if the contents of interfaces_file contain template
    keys corresponding to hwaddresses and replace them with
    the proper device name"""
    with open(interfaces_file + ".templ", "r") as templ_file:
        interfaces = templ_file.read()

    formatter = Formatter()
    hwaddrs = [v[1] for v in formatter.parse(interfaces) if v[1]]
    print("Found the following hwaddrs: {}".format(hwaddrs))
    device_replacements = dict()
    for hwaddr in hwaddrs:
        hwaddr_clean = hwaddr[3:].replace("_", ":")
        if devices.get(hwaddr_clean, None):
            device_replacements[hwaddr] = devices[hwaddr_clean]
        else:
            if fail_on_missing:
                print("Can not find device with MAC {}, will retry".format(hwaddr_clean))
                return False
            else:
                print("WARNING: Can not find device with MAC {} when expected".format(hwaddr_clean))
                device_replacements[hwaddr] = hwaddr
    formatted = interfaces.format(**device_replacements)
    print("Used the values in: {}".format(device_replacements))
    print("to generate new interfaces file:")
    print(formatted)

    with open(output_file, "w") as intf_out_file:
        intf_out_file.write(formatted)

    if not os.path.exists(interfaces_file + ".bak"):
        try:
            shutil.copyfile(interfaces_file, interfaces_file + ".bak")
        except OSError:  # silently ignore if the file is missing
            pass
    return True

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--interfaces-file", dest="intf_file", default=INTERFACES_FILE)
    parser.add_argument("--output-file", dest="out_file", default=INTERFACES_FILE+".out")
    parser.add_argument("--command", default=COMMAND)
    parser.add_argument("--retries", default=RETRIES)
    parser.add_argument("--wait", default=WAIT)
    args = parser.parse_args()
    retries = int(args.retries)
    for tries in range(retries):
        ip_output = ip_parse(subprocess.check_output(args.command.split()).splitlines())
        if replace_ethernets(args.intf_file, args.out_file, ip_output, (tries != retries - 1)):
            break
        else:
            time.sleep(float(args.wait))

if __name__ == "__main__":
    main()
`

const CloudInitNetworkConfigDisabled = `config: "disabled"
`
