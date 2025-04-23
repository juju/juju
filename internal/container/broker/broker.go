// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package broker

import (
	"context"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	corelogger "github.com/juju/juju/core/logger"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/network"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

var logger = internallogger.GetLogger("juju.container.broker")

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/apicalls_mock.go github.com/juju/juju/internal/container/broker APICalls
type APICalls interface {
	ContainerConfig(context.Context) (params.ContainerConfig, error)
	PrepareContainerInterfaceInfo(context.Context, names.MachineTag) (corenetwork.InterfaceInfos, error)
	GetContainerProfileInfo(context.Context, names.MachineTag) ([]*apiprovisioner.LXDProfileResult, error)
	ReleaseContainerAddresses(context.Context, names.MachineTag) error
	SetHostMachineNetworkConfig(context.Context, names.MachineTag, []params.NetworkConfig) error
	HostChangesForContainer(context.Context, names.MachineTag) ([]network.DeviceToBridge, error)
}

// resolvConf contains the full path to common resolv.conf files on the local
// system. Defined here so it can be overridden for testing.
var resolvConfFiles = []string{"/etc/resolv.conf", "/etc/systemd/resolved.conf", "/run/systemd/resolve/resolv.conf"}

func prepareContainerInterfaceInfo(
	ctx context.Context,
	api APICalls, machineID string, log corelogger.Logger,
) (corenetwork.InterfaceInfos, error) {
	log.Debugf(context.TODO(), "using multi-bridge networking for container %q", machineID)

	containerTag := names.NewMachineTag(machineID)
	preparedInfo, err := api.PrepareContainerInterfaceInfo(ctx, containerTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	log.Tracef(context.TODO(), "PrepareContainerInterfaceInfo returned %+v", preparedInfo)

	return preparedInfo, nil
}

// finishNetworkConfig populates the DNSServers and DNSSearchDomains fields on
// each element when they are not set. The given the DNS config is discovered
// using network.ParseResolvConf(). If interfaces has zero length,
// container.FallbackInterfaceInfo() is used as fallback.
func finishNetworkConfig(interfaces corenetwork.InterfaceInfos) (corenetwork.InterfaceInfos, error) {
	haveNameservers, haveSearchDomains := false, false

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
		warnMissing := func(s string) { logger.Warningf(context.TODO(), "no %s supplied by provider, using host's %s.", s, s) }
		if !haveNameservers {
			warnMissing("name servers")
		}
		if !haveSearchDomains {
			warnMissing("search domains")
		}

		logger.Warningf(context.TODO(), "incomplete DNS config found, discovering host's DNS config")
		dnsConfig, err := findDNSServerConfig()
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Since the result is sorted, the first entry is the primary NIC. Also,
		// results always contains at least one element.
		results[0].DNSServers = dnsConfig.Nameservers
		results[0].DNSSearchDomains = dnsConfig.SearchDomains
		logger.Debugf(context.TODO(),
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
			logger.Tracef(context.TODO(), "The DNS configuration from %s returned no dnsConfig", dnsConfigFile)
			continue
		}
		for _, nameServer := range dnsConfig.Nameservers {
			if corenetwork.NewMachineAddress(nameServer).Scope != corenetwork.ScopeMachineLocal {
				logger.Debugf(context.TODO(), "The DNS configuration from %s has been selected for use", dnsConfigFile)
				return dnsConfig, nil
			}
		}
	}
	return nil, errors.New("A DNS configuration could not be found.")
}

func releaseContainerAddresses(
	ctx context.Context,
	api APICalls,
	instanceID instance.Id,
	namespace instance.Namespace,
	log corelogger.Logger,
) {
	containerTag, err := namespace.MachineTag(instanceID.String())
	if err != nil {
		// Not a reason to cause StopInstances to fail though..
		log.Warningf(context.TODO(), "unexpected container tag %q: %v", instanceID, err)
		return
	}
	err = api.ReleaseContainerAddresses(ctx, containerTag)
	switch {
	case err == nil:
		log.Infof(context.TODO(), "released all addresses for container %q", containerTag.Id())
	case errors.Is(err, errors.NotSupported):
		log.Warningf(context.TODO(), "not releasing all addresses for container %q: %v", containerTag.Id(), err)
	default:
		log.Warningf(
			context.TODO(),
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
	log corelogger.Logger,
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
