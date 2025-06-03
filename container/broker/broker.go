// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"net"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc/params"
	coretools "github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.container.broker")

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/apicalls_mock.go github.com/juju/juju/container/broker APICalls
type APICalls interface {
	ContainerConfig() (params.ContainerConfig, error)
	PrepareContainerInterfaceInfo(names.MachineTag) (corenetwork.InterfaceInfos, error)
	GetContainerProfileInfo(names.MachineTag) ([]*apiprovisioner.LXDProfileResult, error)
	ReleaseContainerAddresses(names.MachineTag) error
	SetHostMachineNetworkConfig(names.MachineTag, []params.NetworkConfig) error
	HostChangesForContainer(containerTag names.MachineTag) ([]network.DeviceToBridge, int, error)
}

// resolvConf contains the full path to common resolv.conf files on the local
// system. Defined here so it can be overridden for testing.
var resolvConfFiles = []string{"/etc/resolv.conf", "/etc/systemd/resolved.conf", "/run/systemd/resolve/resolv.conf"}

func prepareContainerInterfaceInfo(
	api APICalls, machineID string, log loggo.Logger,
) (corenetwork.InterfaceInfos, error) {
	log.Debugf("using multi-bridge networking for container %q", machineID)

	containerTag := names.NewMachineTag(machineID)
	preparedInfo, err := api.PrepareContainerInterfaceInfo(containerTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	log.Tracef("PrepareContainerInterfaceInfo returned %+v", preparedInfo)

	return preparedInfo, nil
}

// finishNetworkConfig populates the DNSServers and DNSSearchDomains fields on
// each element when they are not set. The given the DNS config is discovered
// using network.ParseResolvConf(). If interfaces has zero length,
// container.FallbackInterfaceInfo() is used as fallback.
func finishNetworkConfig(interfaces corenetwork.InterfaceInfos) (corenetwork.InterfaceInfos, error) {
	haveNameservers, haveSearchDomains := false, false

	// Note: We do not discover DNS search domain or servers with network
	// configuration, so this always results in false/false.
	// Populating it where we can agent-side, would streamline this process.
	results := make(corenetwork.InterfaceInfos, len(interfaces))
	for i, info := range interfaces {
		if len(info.DNSServers) > 0 {
			haveNameservers = true
		}

		if len(info.DNSSearchDomains) > 0 {
			haveSearchDomains = true
		}
		results[i] = info
	}

	if !haveNameservers || !haveSearchDomains {
		warnMissing := func(s string) { logger.Warningf("no %s supplied by provider, using host's %s.", s, s) }
		if !haveNameservers {
			warnMissing("name servers")
		}
		if !haveSearchDomains {
			warnMissing("search domains")
		}

		logger.Warningf("incomplete DNS config found, discovering host's DNS config")
		dnsConfig, err := findDNSServerConfig()
		if err != nil {
			return nil, errors.Trace(err)
		}

		results = associateDNSConfig(results, dnsConfig)
	}

	return results, nil
}

func associateDNSConfig(nics corenetwork.InterfaceInfos, dns *corenetwork.DNSConfig) corenetwork.InterfaceInfos {
	dnsIPs := make([]net.IP, len(dns.Nameservers))
	dnsUsed := make([]bool, len(dns.Nameservers))
	for i, a := range dns.Nameservers {
		dnsIPs[i] = net.ParseIP(a.Value)
	}

	results := make(corenetwork.InterfaceInfos, len(nics))
	for i, nic := range nics {
		// Associate the search domains with every NIC.
		logger.Infof("setting DNS domains %+v for interface %q", dns.SearchDomains, nic.InterfaceName)
		nic.DNSSearchDomains = dns.SearchDomains

		nameservers := set.NewStrings()

		// Attempt to associate the DNS addresses with NICs based on what subnet
		// they are connected to.
		for _, addr := range nic.Addresses {
			cidr := addr.AddressCIDR()
			if cidr == "" {
				continue
			}

			_, ipNet, err := net.ParseCIDR(cidr)
			if err != nil {
				logger.Warningf("invalid CIDR %q for interface %q", cidr, nic.InterfaceName)
				continue
			}

			for j, dnsIP := range dnsIPs {
				if dnsIP == nil {
					continue
				}

				if ipNet.Contains(dnsIP) {
					// Make sure we only add the nameserver to this device once.
					nsAddr := dns.Nameservers[j].Value
					if nameservers.Contains(nsAddr) {
						continue
					}

					logger.Infof("setting DNS address %q for interface %q", nsAddr, nic.InterfaceName)
					nic.DNSServers = append(nic.DNSServers, dns.Nameservers[j])
					nameservers.Add(nsAddr)
					dnsUsed[j] = true
				}
			}
		}

		results[i] = nic
	}

	// In the event that any nameservers were not associated by subnet,
	// set those against each interface. This is an inelegant fallback
	// for such examples as 1.1.1.1, 8.8.8.8 etc.
	for i, used := range dnsUsed {
		if used {
			continue
		}
		for j := range results {
			results[j].DNSServers = append(results[j].DNSServers, dns.Nameservers[i])
		}
	}

	return results
}

// findDNSServerConfig is a heuristic method to find an adequate DNS
// configuration. Currently the only rule that is implemented is that common
// configuration files are parsed until a configuration is found that is not a
// loopback address (i.e systemd/resolved stub address).
func findDNSServerConfig() (*corenetwork.DNSConfig, error) {
	for _, dnsConfigFile := range resolvConfFiles {
		dnsConfig, err := corenetwork.ParseResolvConf(dnsConfigFile)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// network.ParseResolvConf returns nil error and nil dnsConfig if the
		// file isn't found, which can lead to a panic when attempting to
		// access the dnsConfig.Nameservers. So instead, just continue and
		// exhaust the resolvConfFiles slice.
		if dnsConfig == nil {
			logger.Tracef("The DNS configuration from %s returned no dnsConfig", dnsConfigFile)
			continue
		}
		for _, nameServer := range dnsConfig.Nameservers {
			if nameServer.Scope != corenetwork.ScopeMachineLocal {
				logger.Debugf("The DNS configuration from %s has been selected for use", dnsConfigFile)
				return dnsConfig, nil
			}
		}
	}
	return nil, errors.New("A DNS configuration could not be found.")
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
			"unexpected error trying to release container %q addresses: %v",
			containerTag.Id(), err,
		)
	}
}

// matchHostArchTools filters the given list of tools to the host architecture.
func matchHostArchTools(allTools coretools.List) (coretools.List, error) {
	arch := arch.HostArch()
	archTools, err := allTools.Match(coretools.Filter{Arch: arch})
	if err == coretools.ErrNoMatches {
		agentArch, _ := allTools.OneArch()
		return nil, errors.Errorf(
			"need agent binaries for arch %s, only found %s",
			arch, agentArch,
		)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return archTools, nil
}

var newMachineInitReader = cloudconfig.NewMachineInitReader

// combinedCloudInitData returns a combined map of the given cloudInitData
// and instance cloud init properties provided.
func combinedCloudInitData(
	cloudInitData map[string]interface{},
	containerInheritProperties string, base corebase.Base,
	log loggo.Logger,
) (map[string]interface{}, error) {
	if containerInheritProperties == "" {
		return cloudInitData, nil
	}

	reader, err := newMachineInitReader(base)
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineData, err := reader.GetInitConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if machineData == nil {
		return cloudInitData, nil
	}

	if cloudInitData == nil {
		cloudInitData = make(map[string]interface{})
	}

	props := strings.Split(containerInheritProperties, ",")
	for i, p := range props {
		props[i] = strings.TrimSpace(p)
	}

	// MAAS versions 2.5 and later no longer write repository settings as apt
	// config in cloud-init data.
	// These settings are now represented in curtin data and are a single key,
	// "sources_list" with a value equal to what the content of
	// /etc/apt/sources.list will be.
	// If apt-sources is being inherited, automatically search for the new
	// setting, so new MAAS versions keep working with inherited apt sources.
	if set.NewStrings(props...).Contains("apt-sources") {
		props = append(props, "apt-sources_list")
	}

	resultsMap := reader.ExtractPropertiesFromConfig(props, machineData, log)
	for k, v := range resultsMap {
		cloudInitData[k] = v
	}

	return cloudInitData, nil
}

// proxyConfigurationFromContainerCfg populates a ProxyConfiguration object
// from an ContainerConfig API response.
func proxyConfigurationFromContainerCfg(cfg params.ContainerConfig) instancecfg.ProxyConfiguration {
	return instancecfg.ProxyConfiguration{
		Legacy:              cfg.LegacyProxy,
		Juju:                cfg.JujuProxy,
		Apt:                 cfg.AptProxy,
		AptMirror:           cfg.AptMirror,
		Snap:                cfg.SnapProxy,
		SnapStoreAssertions: cfg.SnapStoreAssertions,
		SnapStoreProxyID:    cfg.SnapStoreProxyID,
		SnapStoreProxyURL:   cfg.SnapStoreProxyURL,
	}
}
