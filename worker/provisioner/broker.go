// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"bufio"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
)

type APICalls interface {
	ContainerConfig() (params.ContainerConfig, error)
	PrepareContainerInterfaceInfo(names.MachineTag) ([]network.InterfaceInfo, error)
	GetContainerInterfaceInfo(names.MachineTag) ([]network.InterfaceInfo, error)
	ReleaseContainerAddresses(names.MachineTag) error
}

type hostArchToolsFinder struct {
	f ToolsFinder
}

// FindTools is defined on the ToolsFinder interface.
func (h hostArchToolsFinder) FindTools(v version.Number, series, _ string) (tools.List, error) {
	// Override the arch constraint with the arch of the host.
	return h.f.FindTools(v, series, arch.HostArch())
}

// resolvConf is the full path to the resolv.conf file on the local
// system. Defined here so it can be overriden for testing.
var resolvConf = "/etc/resolv.conf"

// localDNSServers parses the /etc/resolv.conf file (if available) and
// extracts all nameservers addresses, and the default search domain
// and returns them.
func localDNSServers() ([]network.Address, string, error) {
	file, err := os.Open(resolvConf)
	if os.IsNotExist(err) {
		return nil, "", nil
	} else if err != nil {
		return nil, "", errors.Annotatef(err, "cannot open %q", resolvConf)
	}
	defer file.Close()

	var addresses []network.Address
	var searchDomain string
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
		if strings.HasPrefix(line, "search") {
			searchDomain = strings.TrimPrefix(line, "search")
			// Drop comments after the domain, if any.
			if strings.Contains(searchDomain, "#") {
				searchDomain = searchDomain[:strings.Index(searchDomain, "#")]
			}
			searchDomain = strings.TrimSpace(searchDomain)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, "", errors.Annotatef(err, "cannot read DNS servers from %q", resolvConf)
	}
	return addresses, searchDomain, nil
}

func prepareOrGetContainerInterfaceInfo(
	api APICalls,
	machineID string,
	bridgeDevice string,
	allocateOrMaintain bool,
	startingNetworkInfo []network.InterfaceInfo,
	log loggo.Logger,
) ([]network.InterfaceInfo, error) {
	maintain := !allocateOrMaintain

	if maintain {
		log.Debugf("not running maintenance for machine %q", machineID)
		return nil, nil
	}

	log.Debugf("using multi-bridge networking for container %q", machineID)

	containerTag := names.NewMachineTag(machineID)
	preparedInfo, err := api.PrepareContainerInterfaceInfo(containerTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	log.Tracef("PrepareContainerInterfaceInfo returned %+v", preparedInfo)

	return preparedInfo, nil
}

// finishNetworkConfig populates the ParentInterfaceName, DNSServers, and
// DNSSearchDomains fields on each element, when they are not set. The given
// bridgeDevice is used for ParentInterfaceName, while the DNS config is
// discovered using localDNSServers. If interfaces has zero length,
// container.FallbackInterfaceInfo() is used as fallback.
func finishNetworkConfig(bridgeDevice string, interfaces []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	haveDNSConfig := false
	if len(interfaces) == 0 {
		// Use the fallback network config as a last resort.
		interfaces = container.FallbackInterfaceInfo()
	}

	results := make([]network.InterfaceInfo, len(interfaces))
	for i, info := range interfaces {
		if info.ParentInterfaceName == "" {
			info.ParentInterfaceName = bridgeDevice
		}
		if len(info.DNSServers) > 0 {
			haveDNSConfig = true
		}
		results[i] = info
	}

	if !haveDNSConfig {
		logger.Warningf("no DNS settings found, discovering the host settings")
		dnsServers, searchDomain, err := localDNSServers()
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Since the result is sorted, the first entry is the primary NIC. Also,
		// results always contains at least one element.
		results[0].DNSServers = dnsServers
		results[0].DNSSearchDomains = []string{searchDomain}
		logger.Debugf(
			"setting DNS servers %+v and domains %+v on container interface %q",
			results[0].DNSServers, results[0].DNSSearchDomains, results[0].InterfaceName,
		)
	}

	return results, nil
}

func releaseContainerAddresses(
	api APICalls,
	instanceID instance.Id,
	namespace instance.Namespace,
	log loggo.Logger,
) {
	containerTag, err := namespace.MachineTag(string(instanceID))
	if err != nil {
		// Not a reason to cause StopInstances to fail though..
		log.Warningf("unexpected container tag %q: %v", instanceID, err)
		return
	}
	err = api.ReleaseContainerAddresses(containerTag)
	switch {
	case err == nil:
		log.Infof("released all addresses for container %q", containerTag.Id())
	case errors.IsNotSupported(err):
		log.Warningf("not releasing all addresses for container %q: %v", containerTag.Id(), err)
	default:
		log.Warningf(
			"unexpected error trying to release container %q addreses: %v",
			containerTag.Id(), err,
		)
	}
}

// matchHostArchTools filters the given list of tools to the host architecture.
func matchHostArchTools(allTools tools.List) (tools.List, error) {
	arch := arch.HostArch()
	archTools, err := allTools.Match(tools.Filter{Arch: arch})
	if err == tools.ErrNoMatches {
		return nil, errors.Errorf(
			"need tools for arch %s, only found %s",
			arch, allTools.Arches(),
		)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return archTools, nil
}
