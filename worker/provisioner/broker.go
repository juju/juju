// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/common"
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
	SetHostMachineNetworkConfig(string, []params.NetworkConfig) error
	HostChangesForContainer(containerTag names.MachineTag) ([]network.DeviceToBridge, error)
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

var getObservedNetworkConfig = common.GetObservedNetworkConfig

func prepareHost(bridger network.Bridger, hostMachineID string, containerTag names.MachineTag, api APICalls, log loggo.Logger) error {
	devicesToBridge, err := api.HostChangesForContainer(containerTag)

	if err != nil {
		return errors.Annotate(err, "unable to setup network")
	}

	if len(devicesToBridge) == 0 {
		log.Tracef("container %q requires no additional bridges", containerTag)
		return nil
	}

	deviceNamesToBridge := make([]string, len(devicesToBridge))

	for i, v := range devicesToBridge {
		deviceNamesToBridge[i] = v.DeviceName
	}

	log.Tracef("Bridging %q devices on host %q", deviceNamesToBridge, hostMachineID)

	err = bridger.Bridge(deviceNamesToBridge)

	if err != nil {
		return errors.Annotate(err, "failed to bridge devices")
	}

	// We just changed the hosts' network setup so discover new
	// interfaces/devices and propagate to state.

	observedConfig, err := getObservedNetworkConfig(common.DefaultNetworkConfigSource())

	if err != nil {
		return errors.Annotate(err, "cannot discover observed network config")
	}

	if len(observedConfig) > 0 {
		err := api.SetHostMachineNetworkConfig(hostMachineID, observedConfig)
		if err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("observed network config updated")
	}

	return nil
}

func prepareOrGetContainerInterfaceInfo(
	api APICalls,
	machineID string,
	bridgeDevice string,
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
		logger.Warningf("incomplete DNS config found, discovering host's DNS config")
		dnsConfig, err := network.ParseResolvConf(resolvConf)
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
