// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"strings"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/agent"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxc"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var lxcLogger = loggo.GetLogger("juju.provisioner.lxc")

var _ environs.InstanceBroker = (*lxcBroker)(nil)

type APICalls interface {
	ContainerConfig() (params.ContainerConfig, error)
	PrepareContainerInterfaceInfo(names.MachineTag) ([]network.InterfaceInfo, error)
}

var _ APICalls = (*apiprovisioner.State)(nil)

// Override for testing.
var NewLxcBroker = newLxcBroker

func newLxcBroker(
	api APICalls, agentConfig agent.Config, managerConfig container.ManagerConfig,
	imageURLGetter container.ImageURLGetter,
) (environs.InstanceBroker, error) {
	manager, err := lxc.NewContainerManager(managerConfig, imageURLGetter)
	if err != nil {
		return nil, err
	}
	return &lxcBroker{
		manager:     manager,
		api:         api,
		agentConfig: agentConfig,
	}, nil
}

type lxcBroker struct {
	manager     container.Manager
	api         APICalls
	agentConfig agent.Config
}

// StartInstance is specified in the Broker interface.
func (broker *lxcBroker) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	if args.MachineConfig.HasNetworks() {
		return nil, errors.New("starting lxc containers with networks is not supported yet")
	}
	// TODO: refactor common code out of the container brokers.
	machineId := args.MachineConfig.MachineId
	lxcLogger.Infof("starting lxc container for machineId: %s", machineId)

	// Default to using the host network until we can configure.
	bridgeDevice := broker.agentConfig.Value(agent.LxcBridge)
	if bridgeDevice == "" {
		bridgeDevice = lxc.DefaultLxcBridge
	}
	allocatedInfo, err := maybeAllocateStaticIP(
		machineId, bridgeDevice, broker.api, args.NetworkInfo,
	)
	if err != nil {
		// It's fine, just ignore it. The effect will be that the
		// container won't have a static address configured.
		logger.Infof("not allocating static IP for container %q: %v", machineId, err)
	} else {
		args.NetworkInfo = allocatedInfo
	}
	network := container.BridgeNetworkConfig(bridgeDevice, args.NetworkInfo)

	// The provisioner worker will provide all tools it knows about
	// (after applying explicitly specified constraints), which may
	// include tools for architectures other than the host's. We
	// must constrain to the host's architecture for LXC.
	archTools, err := args.Tools.Match(tools.Filter{
		Arch: version.Current.Arch,
	})
	if err == tools.ErrNoMatches {
		return nil, errors.Errorf(
			"need tools for arch %s, only found %s",
			version.Current.Arch,
			args.Tools.Arches(),
		)
	}

	series := archTools.OneSeries()
	args.MachineConfig.MachineContainerType = instance.LXC
	args.MachineConfig.Tools = archTools[0]

	config, err := broker.api.ContainerConfig()
	if err != nil {
		lxcLogger.Errorf("failed to get container config: %v", err)
		return nil, err
	}

	// If loop mounts are to be used, check that they are allowed.
	storage := container.NewStorageConfig(args.Volumes)
	if !config.AllowLXCLoopMounts && storage.AllowMount {
		return nil, container.ErrLoopMountNotAllowed
	}

	if err := environs.PopulateMachineConfig(
		args.MachineConfig,
		config.ProviderType,
		config.AuthorizedKeys,
		config.SSLHostnameVerification,
		config.Proxy,
		config.AptProxy,
		config.AptMirror,
		config.PreferIPv6,
		config.EnableOSRefreshUpdate,
		config.EnableOSUpgrade,
	); err != nil {
		lxcLogger.Errorf("failed to populate machine config: %v", err)
		return nil, err
	}

	inst, hardware, err := broker.manager.CreateContainer(args.MachineConfig, series, network, storage)
	if err != nil {
		lxcLogger.Errorf("failed to start container: %v", err)
		return nil, err
	}
	lxcLogger.Infof("started lxc container for machineId: %s, %s, %s", machineId, inst.Id(), hardware.String())
	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: hardware,
	}, nil
}

// StopInstances shuts down the given instances.
func (broker *lxcBroker) StopInstances(ids ...instance.Id) error {
	// TODO: potentially parallelise.
	for _, id := range ids {
		lxcLogger.Infof("stopping lxc container for instance: %s", id)
		if err := broker.manager.DestroyContainer(id); err != nil {
			lxcLogger.Errorf("container did not stop: %v", err)
			return err
		}
	}
	return nil
}

// AllInstances only returns running containers.
func (broker *lxcBroker) AllInstances() (result []instance.Instance, err error) {
	return broker.manager.ListContainers()
}

type hostArchToolsFinder struct {
	f ToolsFinder
}

// FindTools is defined on the ToolsFinder interface.
func (h hostArchToolsFinder) FindTools(v version.Number, series string, arch *string) (tools.List, error) {
	// Override the arch constraint with the arch of the host.
	return h.f.FindTools(v, series, &version.Current.Arch)
}

// resolvConf is the full path to the resolv.conf file on the local
// system. Defined here so it can be overriden for testing.
var resolvConf = "/etc/resolv.conf"

// localDNSServers parses the /etc/resolv.conf file (if available) and
// extracts all nameservers addresses, returning them.
func localDNSServers() ([]network.Address, error) {
	file, err := os.Open(resolvConf)
	if os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot open %q", resolvConf)
	}
	defer file.Close()

	var addresses []network.Address
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			// Skip comments.
			continue
		}
		if strings.HasPrefix(line, "nameserver") {
			address := strings.TrimPrefix(line, "nameserver")
			// Drop comments after the address, if any.
			if strings.Contains(address, "#") {
				address = address[:strings.Index(address, "#")]
			}
			address = strings.TrimSpace(address)
			addresses = append(addresses, network.NewAddress(address))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, errors.Annotatef(err, "cannot read DNS servers from %q", resolvConf)
	}
	return addresses, nil
}

// ipRouteAdd is the command template to add a static route for
// .ContainerIP using the .HostBridge device (usually lxcbr0 or virbr0).
var ipRouteAdd = mustParseTemplate("ipRouteAdd", `
ip route add {{.ContainerIP}} dev {{.HostBridge}}`[1:])

type IptablesRule struct {
	Table string
	Chain string
	Rule  string
}

var iptablesRules = map[string]IptablesRule{
	// iptablesCheckSNAT is the command template to verify if a SNAT
	// rule already exists for the host NIC named .HostIF (usually
	// eth0) and source address .HostIP (usually eth0's address). We
	// need to check whether the rule exists because we only want to
	// add it once. Exit code 0 means the rule exists, 1 means it
	// doesn't
	"iptablesSNAT": {
		"nat",
		"POSTROUTING",
		"-o {{.HostIF}} -j SNAT --to-source {{.HostIP}}",
	}, "iptablesForwardOut": {
		// Ensure that we have ACCEPT rules that apply to the containers that
		// we are creating so any DROP rules added by libvirt while setting
		// up virbr0 further down the chain don't disrupt wanted traffic.
		"filter",
		"FORWARD",
		"-d {{.ContainerCIDR}} -o {{.HostBridge}} -j ACCEPT",
	}, "iptablesForwardIn": {
		"filter",
		"FORWARD",
		"-s {{.ContainerCIDR}} -i {{.HostBridge}} -j ACCEPT",
	}}

// mustParseTemplate works like template.Parse, but panics on error.
func mustParseTemplate(name, source string) *template.Template {
	templ, err := template.New(name).Parse(source)
	if err != nil {
		panic(err.Error())
	}
	return templ
}

// mustExecTemplate works like template.Parse followed by template.Execute,
// but panics on error.
func mustExecTemplate(name, tmpl string, data interface{}) string {
	t := mustParseTemplate(name, tmpl)
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(err.Error())
	}
	return buf.String()
}

// runTemplateCommand executes the given template with the given data,
// which generates a command to execute. If exitNonZeroOK is true, no
// error is returned if the exit code is not 0, otherwise an error is
// returned.
func runTemplateCommand(t *template.Template, exitNonZeroOK bool, data interface{}) (
	exitCode int, err error,
) {
	// Clone the template to ensure the original won't be changed.
	cloned, err := t.Clone()
	if err != nil {
		return -1, errors.Annotatef(err, "cannot clone command template %q", t.Name())
	}
	var buf bytes.Buffer
	if err := cloned.Execute(&buf, data); err != nil {
		return -1, errors.Annotatef(err, "cannot execute command template %q", t.Name())
	}
	command := buf.String()
	logger.Debugf("running command %q", command)
	result, err := exec.RunCommands(exec.RunParams{Commands: command})
	if err != nil {
		return -1, errors.Annotatef(err, "cannot run command %q", command)
	}
	exitCode = result.Code
	stdout := string(result.Stdout)
	stderr := string(result.Stderr)
	logger.Debugf(
		"command %q returned code=%d, stdout=%q, stderr=%q",
		command, exitCode, stdout, stderr,
	)
	if exitCode != 0 {
		if exitNonZeroOK {
			return exitCode, nil
		}
		return exitCode, errors.Errorf(
			"command %q failed with exit code %d",
			command, exitCode,
		)
	}
	return 0, nil
}

// setupRoutesAndIPTables sets up on the host machine the needed
// iptables rules and static routes for an addressable container.
var setupRoutesAndIPTables = func(
	primaryNIC string,
	primaryAddr network.Address,
	bridgeName string,
	ifaceInfo []network.InterfaceInfo,
) error {

	if primaryNIC == "" || primaryAddr.Value == "" || bridgeName == "" || len(ifaceInfo) == 0 {
		return errors.Errorf("primaryNIC, primaryAddr, bridgeName, and ifaceInfo must be all set")
	}

	for _, iface := range ifaceInfo {
		containerIP := iface.Address.Value
		if containerIP == "" {
			return errors.Errorf("container IP %q must be set", containerIP)
		}
		data := struct {
			HostIF        string
			HostIP        string
			HostBridge    string
			ContainerIP   string
			ContainerCIDR string
		}{primaryNIC, primaryAddr.Value, bridgeName, containerIP, iface.CIDR}

		for name, rule := range iptablesRules {
			check := mustExecTemplate("rule", "iptables -t {{.Table}} -C {{.Chain}} {{.Rule}}", rule)
			t := mustParseTemplate(name+"Check", check)

			code, err := runTemplateCommand(t, true, data)
			if err != nil {
				return errors.Trace(err)
			}
			switch code {
			case 0:
			// Rule does exist. Do nothing
			case 1:
				// Rule does not exist, add it. We insert the rule at the top of the list so it precedes any
				// REJECT rules.
				action := mustExecTemplate("action", "iptables -t {{.Table}} -I {{.Chain}} 1 {{.Rule}}", rule)
				t = mustParseTemplate(name+"Add", action)
				_, err = runTemplateCommand(t, false, data)
				if err != nil {
					return errors.Trace(err)
				}
			default:
				// Unexpected code - better report it.
				return errors.Errorf("iptables failed with unexpected exit code %d", code)
			}
		}

		_, err := runTemplateCommand(ipRouteAdd, false, data)
		if err != nil {
			return errors.Trace(err)
		}
	}
	logger.Infof("successfully configured iptables and routes for container interfaces")

	return nil
}

var (
	netInterfaces  = net.Interfaces
	interfaceAddrs = (*net.Interface).Addrs
)

// discoverPrimaryNIC returns the name of the first network interface
// on the machine which is up and has address, along with the first
// address it has.
func discoverPrimaryNIC() (string, network.Address, error) {
	interfaces, err := netInterfaces()
	if err != nil {
		return "", network.Address{}, errors.Annotatef(err, "cannot get network interfaces")
	}
	logger.Tracef("trying to discover primary network interface")
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			// Skip the loopback.
			logger.Tracef("not using loopback interface %q", iface.Name)
			continue
		}
		if iface.Flags&net.FlagUp != 0 {
			// Possibly the primary, but ensure it has an address as
			// well.
			logger.Tracef("verifying interface %q has addresses", iface.Name)
			addrs, err := interfaceAddrs(&iface)
			if err != nil {
				return "", network.Address{}, errors.Annotatef(err, "cannot get %q addresses", iface.Name)
			}
			if len(addrs) > 0 {
				// We found it.
				// Check if it's an IP or a CIDR.
				addr := addrs[0].String()
				ip := net.ParseIP(addr)
				if ip == nil {
					// Try a CIDR.
					ip, _, err = net.ParseCIDR(addr)
					if err != nil {
						return "", network.Address{}, errors.Annotatef(err, "cannot parse address %q", addr)
					}
				}
				addr = ip.String()

				logger.Tracef("primary network interface is %q, address %q", iface.Name, addr)
				return iface.Name, network.NewAddress(addr), nil
			}
		}
	}
	return "", network.Address{}, errors.Errorf("cannot detect the primary network interface")
}

// MACAddressTemplate is used to generate a unique MAC address for a
// container. Every 'x' is replaced by a random hexadecimal digit,
// while the rest is kept as-is.
const MACAddressTemplate = "00:16:3e:xx:xx:xx"

// maybeAllocateStaticIP tries to allocate a static IP address for the
// given containerId using the provisioner API. If it fails, it's not
// critical - just a warning, and it won't cause StartInstance to
// fail.
func maybeAllocateStaticIP(
	containerId, bridgeDevice string,
	apiFacade APICalls,
	ifaceInfo []network.InterfaceInfo,
) (finalIfaceInfo []network.InterfaceInfo, err error) {
	defer func() {
		if err != nil {
			logger.Warningf(
				"failed allocating a static IP for container %q: %v",
				containerId, err,
			)
		}
	}()

	if len(ifaceInfo) != 0 {
		// When we already have interface info, don't overwrite it.
		return nil, nil
	}
	logger.Debugf("trying to allocate a static IP for container %q", containerId)

	var primaryNIC string
	var primaryAddr network.Address
	primaryNIC, primaryAddr, err = discoverPrimaryNIC()
	if err != nil {
		return nil, errors.Trace(err)
	}

	finalIfaceInfo, err = apiFacade.PrepareContainerInterfaceInfo(names.NewMachineTag(containerId))
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("PrepareContainerInterfaceInfo returned %#v", finalIfaceInfo)

	// Populate ConfigType and DNSServers as needed.
	var dnsServers []network.Address
	dnsServers, err = localDNSServers()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Generate the final configuration for each container interface.
	for i, _ := range finalIfaceInfo {
		// Always start at the first device index and generate the
		// interface name based on that. We need to do this otherwise
		// the container will inherit the host's device index and
		// interface name.
		finalIfaceInfo[i].DeviceIndex = i
		finalIfaceInfo[i].InterfaceName = fmt.Sprintf("eth%d", i)
		finalIfaceInfo[i].MACAddress = MACAddressTemplate
		finalIfaceInfo[i].ConfigType = network.ConfigStatic
		finalIfaceInfo[i].DNSServers = dnsServers
		finalIfaceInfo[i].GatewayAddress = primaryAddr
	}
	err = setupRoutesAndIPTables(primaryNIC, primaryAddr, bridgeDevice, finalIfaceInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return finalIfaceInfo, nil
}
