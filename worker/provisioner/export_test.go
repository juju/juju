// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"reflect"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
)

func SetObserver(p Provisioner, observer chan<- *config.Config) {
	var configObserver *configObserver
	if ep, ok := p.(*environProvisioner); ok {
		configObserver = &ep.configObserver
	} else {
		cp := p.(*containerProvisioner)
		configObserver = &cp.configObserver
	}
	configObserver.Lock()
	configObserver.observer = observer
	configObserver.Unlock()
}

func GetRetryWatcher(p Provisioner) (watcher.NotifyWatcher, error) {
	return p.getRetryWatcher()
}

var (
	ContainerManagerConfig     = containerManagerConfig
	GetToolsFinder             = &getToolsFinder
	SysctlConfig               = &sysctlConfig
	ResolvConf                 = &resolvConf
	LocalDNSServers            = localDNSServers
	MustParseTemplate          = mustParseTemplate
	RunTemplateCommand         = runTemplateCommand
	IptablesRules              = &iptablesRules
	NetInterfaces              = &netInterfaces
	InterfaceAddrs             = &interfaceAddrs
	DiscoverPrimaryNIC         = discoverPrimaryNIC
	ConfigureContainerNetwork  = configureContainerNetwork
	MaybeOverrideDefaultLXCNet = maybeOverrideDefaultLXCNet
	EtcDefaultLXCNetPath       = &etcDefaultLXCNetPath
	EtcDefaultLXCNet           = etcDefaultLXCNet
)

const (
	IPForwardSysctlKey = ipForwardSysctlKey
	ARPProxySysctlKey  = arpProxySysctlKey
)

// SetIPAndARPForwarding calls the internal setIPAndARPForwarding and
// then restores the mocked one.
var SetIPAndARPForwarding func(bool) error

// SetupRoutesAndIPTables calls the internal setupRoutesAndIPTables
// and the restores the mocked one.
var SetupRoutesAndIPTables func(string, network.Address, string, []network.InterfaceInfo, bool) error

func init() {
	// In order to isolate the host machine from the running tests,
	// but also allow calling the original setIPAndARPForwarding and
	// setupRoutesAndIPTables funcs to test them, we need a litte bit
	// of reflect magic, mostly borrowed from the juju/testing
	// pacakge.
	newSetIPAndARPForwardingValue := reflect.ValueOf(&setIPAndARPForwarding).Elem()
	newSetupRoutesAndIPTablesValue := reflect.ValueOf(&setupRoutesAndIPTables).Elem()
	oldSetIPAndARPForwardingValue := reflect.New(newSetIPAndARPForwardingValue.Type()).Elem()
	oldSetupRoutesAndIPTablesValue := reflect.New(newSetupRoutesAndIPTablesValue.Type()).Elem()
	oldSetIPAndARPForwardingValue.Set(newSetIPAndARPForwardingValue)
	oldSetupRoutesAndIPTablesValue.Set(newSetupRoutesAndIPTablesValue)
	mockSetIPAndARPForwardingValue := reflect.ValueOf(
		func(bool) error { return nil },
	)
	mockSetupRoutesAndIPTablesValue := reflect.ValueOf(
		func(string, network.Address, string, []network.InterfaceInfo, bool) error { return nil },
	)
	switchValues := func(newValue, oldValue reflect.Value) {
		newValue.Set(oldValue)
	}
	switchValues(newSetIPAndARPForwardingValue, mockSetIPAndARPForwardingValue)
	switchValues(newSetupRoutesAndIPTablesValue, mockSetupRoutesAndIPTablesValue)

	SetIPAndARPForwarding = func(v bool) error {
		switchValues(newSetIPAndARPForwardingValue, oldSetIPAndARPForwardingValue)
		defer switchValues(newSetIPAndARPForwardingValue, mockSetIPAndARPForwardingValue)
		return setIPAndARPForwarding(v)
	}
	SetupRoutesAndIPTables = func(nic string, addr network.Address, bridge string, ifinfo []network.InterfaceInfo, enableNAT bool) error {
		switchValues(newSetupRoutesAndIPTablesValue, oldSetupRoutesAndIPTablesValue)
		defer switchValues(newSetupRoutesAndIPTablesValue, mockSetupRoutesAndIPTablesValue)
		return setupRoutesAndIPTables(nic, addr, bridge, ifinfo, enableNAT)
	}
}

var ClassifyMachine = classifyMachine
