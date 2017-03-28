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
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
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
	gatewayHandled := false
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
			gatewayHandled = true
			continue
		}

		output.WriteString("iface " + name + " inet static\n")
		output.WriteString("  address " + address + "\n")
		if !gatewayHandled && prepared.GatewayAddress != "" {
			_, network, err := net.ParseCIDR(address)
			if err != nil {
				return "", errors.Annotatef(err, "invalid gateway for interface %q with address %q", name, address)
			}

			gatewayIP := net.ParseIP(prepared.GatewayAddress)
			if network.Contains(gatewayIP) {
				output.WriteString("  gateway " + prepared.GatewayAddress + "\n")
				gatewayHandled = true // write it only once
			}
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

	if !gatewayHandled {
		logger.Infof("generated network config has no gateway")
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
	GatewayAddress   string
}

// PrepareNetworkConfigFromInterfaces collects the necessary information to
// render a persistent network config from the given slice of
// network.InterfaceInfo. The result always includes the loopback interface.
func PrepareNetworkConfigFromInterfaces(interfaces []network.InterfaceInfo) *PreparedConfig {
	dnsServers := set.NewStrings()
	dnsSearchDomains := set.NewStrings()
	gatewayAddress := ""
	namesInOrder := make([]string, 1, len(interfaces)+1)
	nameToAddress := make(map[string]string)
	nameToRoutes := make(map[string][]network.Route)

	// Always include the loopback.
	namesInOrder[0] = "lo"
	autoStarted := set.NewStrings("lo")

	for _, info := range interfaces {
		cleanIfaceName := strings.Replace(info.MACAddress, ":", "_", -1)
		// prepend eth because .format of python wont like a tag starting with numbers.
		cleanIfaceName = fmt.Sprintf("{eth%s}", cleanIfaceName)
		if !info.NoAutoStart {
			autoStarted.Add(cleanIfaceName)
		}

		if cidr := info.CIDRAddress(); cidr != "" {
			nameToAddress[cleanIfaceName] = cidr
		} else if info.ConfigType == network.ConfigDHCP {
			nameToAddress[cleanIfaceName] = string(network.ConfigDHCP)
		}
		nameToRoutes[cleanIfaceName] = info.Routes

		for _, dns := range info.DNSServers {
			dnsServers.Add(dns.Value)
		}

		dnsSearchDomains = dnsSearchDomains.Union(set.NewStrings(info.DNSSearchDomains...))

		if gatewayAddress == "" && info.GatewayAddress.Value != "" {
			gatewayAddress = info.GatewayAddress.Value
		}

		namesInOrder = append(namesInOrder, cleanIfaceName)
	}

	prepared := &PreparedConfig{
		InterfaceNames:   namesInOrder,
		NameToAddress:    nameToAddress,
		NameToRoutes:     nameToRoutes,
		AutoStarted:      autoStarted.SortedValues(),
		DNSServers:       dnsServers.SortedValues(),
		DNSSearchDomains: dnsSearchDomains.SortedValues(),
		GatewayAddress:   gatewayAddress,
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
		cloudConfig.AddBootTextFile(systemNetworkInterfacesFile, config, 0644)
		cloudConfig.AddBootTextFile(systemNetworkInterfacesFile+".py", populateNetworkInterfacesScript(), 0744)
		cloudConfig.AddBootCmd(populateNetworkInterfaces(systemNetworkInterfacesFile))
		//cloudConfig.AddRunCmd(raiseJujuNetworkInterfacesScript(systemNetworkInterfacesFile, networkInterfacesFile))
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

// TemplateUserData returns a minimal user data necessary for the template.
// This should have the authorized keys, base packages, the cloud archive if
// necessary,  initial apt proxy config, and it should do the apt-get
// update/upgrade initially.
func TemplateUserData(
	series string,
	authorizedKeys string,
	aptProxy proxy.Settings,
	aptMirror string,
	enablePackageUpdates bool,
	enableOSUpgrades bool,
	networkConfig *container.NetworkConfig,
) ([]byte, error) {
	var config cloudinit.CloudConfig
	var err error
	if networkConfig != nil {
		config, err = newCloudInitConfigWithNetworks(series, networkConfig)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		config, err = cloudinit.New(series)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	cloudconfig.SetUbuntuUser(config, authorizedKeys)
	config.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
	)
	// For LTS series which need support for the cloud-tools archive,
	// we need to enable apt-get update regardless of the environ
	// setting, otherwise provisioning will fail.
	if series == "precise" && !enablePackageUpdates {
		logger.Infof("series %q requires cloud-tools archive: enabling updates", series)
		enablePackageUpdates = true
	}

	if enablePackageUpdates && config.RequiresCloudArchiveCloudTools() {
		config.AddCloudArchiveCloudTools()
	}
	config.AddPackageCommands(aptProxy, aptMirror, enablePackageUpdates, enableOSUpgrades)

	initSystem, err := service.VersionInitSystem(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cmds, err := shutdownInitCommands(initSystem, series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	config.AddScripts(strings.Join(cmds, "\n"))

	data, err := config.RenderYAML()
	if err != nil {
		return nil, err
	}
	return data, nil
}

func shutdownInitCommands(initSystem, series string) ([]string, error) {
	shutdownCmd := "/sbin/shutdown -h now"
	name := "juju-template-restart"
	desc := "juju shutdown job"

	execStart := shutdownCmd

	conf := common.Conf{
		Desc:         desc,
		Transient:    true,
		AfterStopped: "cloud-final",
		ExecStart:    execStart,
	}
	// systemd uses targets for synchronization of services
	if initSystem == service.InitSystemSystemd {
		conf.AfterStopped = "cloud-config.target"
	}

	svc, err := service.NewService(name, conf, series)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cmds, err := svc.InstallCommands()
	if err != nil {
		return nil, errors.Trace(err)
	}

	startCommands, err := svc.StartCommands()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cmds = append(cmds, startCommands...)

	return cmds, nil
}

// raiseJujuNetworkInterfacesScript returns a cloud-init script to
// raise Juju's network interfaces supplied via cloud-init.
//
// Note: we sleep to mitigate against LP #1337873 and LP #1269921.
func raiseJujuNetworkInterfacesScript(oldInterfacesFile, newInterfacesFile string) string {
	return fmt.Sprintf(`
if [ -f %[2]s ]; then
    echo "stopping all interfaces"
    ifdown -a
    sleep 1.5
    if ifup -a --interfaces=%[2]s; then
        echo "ifup with %[2]s succeeded, renaming to %[1]s"
        cp %[1]s %[1]s-orig
        cp %[2]s %[1]s
    else
        echo "ifup with %[2]s failed, leaving old %[1]s alone"
        ifup -a
    fi
else
    echo "did not find %[2]s, not reconfiguring networking"
fi`[1:],
		oldInterfacesFile, newInterfacesFile)
}

func populateNetworkInterfaces(networkFile string) string {
	s := `
if [ -f /usr/bin/python ]; then
    python %s.py --interfaces_file %s
else
    python3 %s.py --interfaces_file %s
fi
`
	return fmt.Sprintf(s, networkFile, networkFile, networkFile, networkFile)
}

func populateNetworkInterfacesScript() string {
	s := `import subprocess, re, argparse, os
from string import Formatter
INTERFACES_FILE="/etc/network/interfaces"
IP_LINE = re.compile(r"^\d: (.*?):")
IP_HWADDR = re.compile(r".*link/ether ((\w{2}|:){11})")
COMMAND = "ip -oneline link"

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
        ip_line_str = strdecode(ip_line, 'utf-8')
        match = IP_LINE.match(ip_line_str)
        if match is None:
            continue
        nic_name = match.group(1)
        match = IP_HWADDR.match(ip_line_str)
        if match is None:
            continue
        nic_hwaddr = match.group(1)
        devices[nic_hwaddr]=nic_name
    print("Found the following devices: %s" % str(devices))
    return devices

def replace_ethernets(interfaces_file, devices):
    """check if the contents of interfaces_file contain template
    keys corresponding to hwaddresses and replace them with
    the proper device name"""
    with open(interfaces_file, "r") as intf_file:
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
            device_replacements[hwaddr] = hwaddr
    formatted = interfaces.format(**device_replacements)
    print("Used the values in: %s\nto fix the interfaces file:\n%s\ninto\n%s" %
           (str(device_replacements), str(interfaces), str(formatted)))

    with open(interfaces_file + ".tmp", "w") as intf_file:
        intf_file.write(formatted)

    os.rename(interfaces_file, interfaces_file + ".bak")
    os.rename(interfaces_file + ".tmp", interfaces_file)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--interfaces_file', dest='intf_file', default=INTERFACES_FILE)
    parser.add_argument('--command', dest='command',default=COMMAND)
    args = parser.parse_args()
    ip_output = ip_parse(subprocess.check_output(args.command.split()).splitlines())
    replace_ethernets(args.intf_file, ip_output)


if __name__ == "__main__":
    main()
`
	return s
}
