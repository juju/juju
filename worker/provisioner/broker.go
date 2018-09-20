// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/tools"
)

//go:generate mockgen -package mocks -destination mocks/apicalls_mock.go github.com/juju/juju/worker/provisioner APICalls
type APICalls interface {
	ContainerConfig() (params.ContainerConfig, error)
	PrepareContainerInterfaceInfo(names.MachineTag) ([]network.InterfaceInfo, error)
	GetContainerInterfaceInfo(names.MachineTag) ([]network.InterfaceInfo, error)
	GetContainerProfileInfo(names.MachineTag) ([]apiprovisioner.LXDProfileResult, error)
	ReleaseContainerAddresses(names.MachineTag) error
	SetHostMachineNetworkConfig(names.MachineTag, []params.NetworkConfig) error
	HostChangesForContainer(containerTag names.MachineTag) ([]network.DeviceToBridge, int, error)
}

type hostArchToolsFinder struct {
	f ToolsFinder
}

// FindTools is defined on the ToolsFinder interface.
func (h hostArchToolsFinder) FindTools(v version.Number, series, _ string) (tools.List, error) {
	// Override the arch constraint with the arch of the host.
	return h.f.FindTools(v, series, arch.HostArch())
}

// resolvConf contains the full path to common resolv.conf files on the local
// system. Defined here so it can be overridden for testing.
var resolvConfFiles = []string{"/etc/resolv.conf", "/etc/systemd/resolved.conf", "/run/systemd/resolve/resolv.conf"}

func prepareOrGetContainerInterfaceInfo(
	api APICalls,
	machineID string,
	allocateOrMaintain bool,
	log loggo.Logger,
) ([]network.InterfaceInfo, error) {
	maintain := !allocateOrMaintain

	if maintain {
		// TODO(jam): 2016-12-14 The function is called
		// 'prepareOrGet', but the only time we would handle the 'Get'
		// side, we explicitly abort. Something seems wrong.
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
// discovered using network.ParseResolvConf(). If interfaces has zero length,
// container.FallbackInterfaceInfo() is used as fallback.
func finishNetworkConfig(bridgeDevice string, interfaces []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	haveNameservers, haveSearchDomains := false, false
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

		// Since the result is sorted, the first entry is the primary NIC. Also,
		// results always contains at least one element.
		results[0].DNSServers = dnsConfig.Nameservers
		results[0].DNSSearchDomains = dnsConfig.SearchDomains
		logger.Debugf(
			"setting DNS servers %+v and domains %+v on container interface %q",
			results[0].DNSServers, results[0].DNSSearchDomains, results[0].InterfaceName,
		)
	}

	return results, nil
}

// findDNSServerConfig is a heuristic method to find an adequate DNS
// configuration. Currently the only rule that is implemented is that common
// configuration files are parsed until a configuration is found that is not a
// loopback address (i.e systemd/resolved stub address).
func findDNSServerConfig() (*network.DNSConfig, error) {
	for _, dnsConfigFile := range resolvConfFiles {
		dnsConfig, err := network.ParseResolvConf(dnsConfigFile)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, nameServer := range dnsConfig.Nameservers {
			if nameServer.Scope != network.ScopeMachineLocal {
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
			"need agent binaries for arch %s, only found %s",
			arch, allTools.Arches(),
		)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return archTools, nil
}

// GetMachineCloudInitData is for testing purposes.
var GetMachineCloudInitData = cloudconfig.GetMachineCloudInitData

// combinedCloudInitData returns a combined map of the given cloudInitData
// and instance cloud init properties provided.
func combinedCloudInitData(
	cloudInitData map[string]interface{},
	containerInheritProperties, series string,
	log loggo.Logger,
) (map[string]interface{}, error) {
	if containerInheritProperties == "" {
		return cloudInitData, nil
	}
	machineData, err := GetMachineCloudInitData(series)
	if err != nil {
		return nil, err
	}
	if machineData == nil {
		return cloudInitData, nil
	}

	if cloudInitData == nil {
		cloudInitData = make(map[string]interface{})
	}

	resultsMap := cloudconfig.CloudConfigByVersionFunc(series)(containerInheritProperties, machineData, log)
	for k, v := range resultsMap {
		cloudInitData[k] = v
	}

	return cloudInitData, nil
}
