// Copyright 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package containerinit

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/network"
)

var (
	logger = loggo.GetLogger("juju.cloudconfig.containerinit")
)

// WriteUserData generates the cloud-init user-data using the
// specified machine and network config for a container, and writes
// the serialized form out to a cloud-init file in the directory
// specified.
func WriteUserData(
	instanceConfig *instancecfg.InstanceConfig,
	networkConfig *container.NetworkConfig,
	directory string,
) (string, error) {
	userData, err := CloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		logger.Errorf("failed to create user data: %v", err)
		return "", err
	}
	return WriteCloudInitFile(directory, userData)
}

// WriteCloudInitFile writes the data out to a cloud-init file in the
// directory specified, and returns the filename.
func WriteCloudInitFile(directory string, userData []byte) (string, error) {
	userDataFilename := filepath.Join(directory, "cloud-init")
	if err := ioutil.WriteFile(userDataFilename, userData, 0644); err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return "", err
	}
	return userDataFilename, nil
}

var (
	systemNetworkInterfacesFile = "/etc/network/interfaces"
	networkInterfacesFile       = systemNetworkInterfacesFile + "-juju"
)

// GenerateNetworkConfig renders a network config for one or more network
// interfaces, using the given non-nil networkConfig containing a non-empty
// Interfaces field.
func GenerateNetworkConfig(networkConfig *container.NetworkConfig) (string, error) {
	if networkConfig == nil || len(networkConfig.Interfaces) == 0 {
		return "", errors.Errorf("missing container network config")
	}
	logger.Debugf("generating network config from %#v", *networkConfig)

	prepared := PrepareNetworkConfigFromInterfaces(networkConfig.Interfaces)

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
		} else if address == string(network.ConfigDHCP) {
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
	logger.Debugf("generated network config:\n%s", generatedConfig)

	if hasV4Interface && !gateway4Handled {
		logger.Infof("generated network config has no ipv4 gateway")
	}

	if hasV6Interface && !gateway6Handled {
		logger.Infof("generated network config has no ipv6 gateway")
	}

	return generatedConfig, nil
}

// PreparedConfig holds all the necessary information to render a persistent
// network config to a file.
type PreparedConfig struct {
	InterfaceNames   []string
	AutoStarted      []string
	DNSServers       []string
	DNSSearchDomains []string
	NameToAddress    map[string]string
	NameToRoutes     map[string][]network.Route
	NameToMTU        map[string]int
	Gateway4Address  string
	Gateway6Address  string
}

// PrepareNetworkConfigFromInterfaces collects the necessary information to
// render a persistent network config from the given slice of
// network.InterfaceInfo. The result always includes the loopback interface.
func PrepareNetworkConfigFromInterfaces(interfaces []network.InterfaceInfo) *PreparedConfig {
	dnsServers := set.NewStrings()
	dnsSearchDomains := set.NewStrings()
	gateway4Address := ""
	gateway6Address := ""
	namesInOrder := make([]string, 1, len(interfaces)+1)
	nameToAddress := make(map[string]string)
	nameToRoutes := make(map[string][]network.Route)
	nameToMTU := make(map[string]int)

	// Always include the loopback.
	namesInOrder[0] = "lo"
	autoStarted := set.NewStrings("lo")

	for _, info := range interfaces {
		ifaceName := strings.Replace(info.MACAddress, ":", "_", -1)
		// prepend eth because .format of python wont like a tag starting with numbers.
		ifaceName = fmt.Sprintf("{eth%s}", ifaceName)

		if !info.NoAutoStart {
			autoStarted.Add(ifaceName)
		}

		if cidr := info.CIDRAddress(); cidr != "" {
			nameToAddress[ifaceName] = cidr
		} else if info.ConfigType == network.ConfigDHCP {
			nameToAddress[ifaceName] = string(network.ConfigDHCP)
		}
		nameToRoutes[ifaceName] = info.Routes

		for _, dns := range info.DNSServers {
			dnsServers.Add(dns.Value)
		}

		dnsSearchDomains = dnsSearchDomains.Union(set.NewStrings(info.DNSSearchDomains...))

		if info.GatewayAddress.Value != "" {
			switch {
			case gateway4Address == "" && info.GatewayAddress.Type == network.IPv4Address:
				gateway4Address = info.GatewayAddress.Value

			case gateway6Address == "" && info.GatewayAddress.Type == network.IPv6Address:
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

	logger.Debugf("prepared network config for rendering: %+v", prepared)
	return prepared
}

// newCloudInitConfigWithNetworks creates a cloud-init config which
// might include per-interface networking config if both networkConfig
// is not nil and its Interfaces field is not empty.
func newCloudInitConfigWithNetworks(series string, networkConfig *container.NetworkConfig) (cloudinit.CloudConfig, error) {
	cloudConfig, err := cloudinit.New(series)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if networkConfig != nil {
		config, err := GenerateNetworkConfig(networkConfig)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cloudConfig.AddBootTextFile(systemNetworkInterfacesFile+".templ", config, 0644)
		cloudConfig.AddBootTextFile(systemNetworkInterfacesFile+".py", NetworkInterfacesScript, 0744)
		cloudConfig.AddBootCmd(populateNetworkInterfaces(systemNetworkInterfacesFile))
	}

	return cloudConfig, nil
}

func CloudInitUserData(
	instanceConfig *instancecfg.InstanceConfig,
	networkConfig *container.NetworkConfig,
) ([]byte, error) {
	cloudConfig, err := newCloudInitConfigWithNetworks(instanceConfig.Series, networkConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	udata, err := cloudconfig.NewUserdataConfig(instanceConfig, cloudConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = udata.Configure(); err != nil {
		return nil, errors.Trace(err)
	}
	// Run ifconfig to get the addresses of the internal container at least
	// logged in the host.
	cloudConfig.AddRunCmd("ifconfig")

	if instanceConfig.MachineContainerHostname != "" {
		logger.Debugf("Cloud-init configured to set hostname")
		cloudConfig.SetAttr("hostname", instanceConfig.MachineContainerHostname)
	}

	data, err := cloudConfig.RenderYAML()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return data, nil
}

// Note: we sleep to mitigate against LP #1337873 and LP #1269921.
func populateNetworkInterfaces(networkFile string) string {
	s := `
ifdown -a
sleep 1.5
if [ -f /usr/bin/python ]; then
    python %[1]s.py --interfaces-file %[1]s
else
    python3 %[1]s.py --interfaces-file %[1]s
fi
ifup -a
`
	return fmt.Sprintf(s, networkFile)
}

const NetworkInterfacesScript = `from __future__ import print_function, unicode_literals
import subprocess, re, argparse, os, time
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
    print("Parsing ip command output %s" % ip_output)
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
    print("Found the following devices: %s" % str(devices))
    return devices

def replace_ethernets(interfaces_file, devices, fail_on_missing):
    """check if the contents of interfaces_file contain template
    keys corresponding to hwaddresses and replace them with
    the proper device name"""
    with open(interfaces_file + ".templ", "r") as intf_file:
        interfaces = intf_file.read()

    formatter = Formatter()
    hwaddrs = [v[1] for v in formatter.parse(interfaces) if v[1]]
    print("Found the following hwaddrs: %s" % str(hwaddrs))
    device_replacements = dict()
    for hwaddr in hwaddrs:
        hwaddr_clean = hwaddr[3:].replace("_", ":")
        if devices.get(hwaddr_clean, None):
            device_replacements[hwaddr] = devices[hwaddr_clean]
        else:
            if fail_on_missing:
                print("Can't find device with MAC %s, will retry" % hwaddr_clean)
                return False
            else:
                print("WARNING: Can't find device with MAC %s when expected" % hwaddr_clean)
                device_replacements[hwaddr] = hwaddr
    formatted = interfaces.format(**device_replacements)
    print("Used the values in: %s\nto fix the interfaces file:\n%s\ninto\n%s" %
           (str(device_replacements), str(interfaces), str(formatted)))

    with open(interfaces_file + ".tmp", "w") as intf_file:
        intf_file.write(formatted)

    if not os.path.exists(interfaces_file + ".bak"):
        try:
            os.rename(interfaces_file, interfaces_file + ".bak")
        except OSError: #silently ignore if the file is missing
            pass
    os.rename(interfaces_file + ".tmp", interfaces_file)
    return True

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--interfaces-file", dest="intf_file", default=INTERFACES_FILE)
    parser.add_argument("--command", default=COMMAND)
    parser.add_argument("--retries", default=RETRIES)
    parser.add_argument("--wait", default=WAIT)
    args = parser.parse_args()
    retries = int(args.retries)
    for tries in range(retries):
        ip_output = ip_parse(subprocess.check_output(args.command.split()).splitlines())
        if replace_ethernets(args.intf_file, ip_output, (tries != retries - 1)):
             break
        else:
             time.sleep(float(args.wait))

if __name__ == "__main__":
    main()
`

const CloudInitNetworkConfigDisabled = `network:
  config: "disabled"
`
