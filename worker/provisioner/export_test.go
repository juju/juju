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
	ep := p.(*environProvisioner)
	ep.Lock()
	ep.observer = observer
	ep.Unlock()
}

func GetRetryWatcher(p Provisioner) (watcher.NotifyWatcher, error) {
	return p.getRetryWatcher()
}

var (
	ContainerManagerConfig = containerManagerConfig
	GetToolsFinder         = &getToolsFinder
	SysctlConfig           = &sysctlConfig
	ResolvConf             = &resolvConf
	LocalDNSServers        = localDNSServers
	MustParseTemplate      = mustParseTemplate
	RunTemplateCommand     = runTemplateCommand
	IPTablesCheckSNAT      = &iptablesCheckSNAT
	IPTablesAddSNAT        = &iptablesAddSNAT
	NetInterfaces          = &netInterfaces
	InterfaceAddrs         = &interfaceAddrs
	DiscoverPrimaryNIC     = discoverPrimaryNIC
	MaybeAllocateStaticIP  = maybeAllocateStaticIP
)

const IPForwardSysctlKey = ipForwardSysctlKey

// SetIPForwarding calls the internal setIPForwarding and then
// restores the mocked one.
var SetIPForwarding func(bool) error

// SetupRoutesAndIPTables calls the internal setupRoutesAndIPTables
// and the restores the mocked one.
var SetupRoutesAndIPTables func(string, network.Address, string, []network.InterfaceInfo) error

func init() {
	// In order to isolate the host machine from the running tests,
	// but also allow calling the original setIPForwarding and
	// setupRoutesAndIPTables funcs to test them, we need a litte bit
	// of reflect magic, mostly borrowed from the juju/testing
	// pacakge.
	newSetIPForwardingValue := reflect.ValueOf(&setIPForwarding).Elem()
	newSetupRoutesAndIPTablesValue := reflect.ValueOf(&setupRoutesAndIPTables).Elem()
	oldSetIPForwardingValue := reflect.New(newSetIPForwardingValue.Type()).Elem()
	oldSetupRoutesAndIPTablesValue := reflect.New(newSetupRoutesAndIPTablesValue.Type()).Elem()
	oldSetIPForwardingValue.Set(newSetIPForwardingValue)
	oldSetupRoutesAndIPTablesValue.Set(newSetupRoutesAndIPTablesValue)
	mockSetIPForwardingValue := reflect.ValueOf(
		func(bool) error { return nil },
	)
	mockSetupRoutesAndIPTablesValue := reflect.ValueOf(
		func(string, network.Address, string, []network.InterfaceInfo) error { return nil },
	)
	switchValues := func(newValue, oldValue reflect.Value) {
		newValue.Set(oldValue)
	}
	switchValues(newSetIPForwardingValue, mockSetIPForwardingValue)
	switchValues(newSetupRoutesAndIPTablesValue, mockSetupRoutesAndIPTablesValue)

	SetIPForwarding = func(v bool) error {
		switchValues(newSetIPForwardingValue, oldSetIPForwardingValue)
		defer switchValues(newSetIPForwardingValue, mockSetIPForwardingValue)
		return setIPForwarding(v)
	}
	SetupRoutesAndIPTables = func(nic string, addr network.Address, bridge string, ifinfo []network.InterfaceInfo) error {
		switchValues(newSetupRoutesAndIPTablesValue, oldSetupRoutesAndIPTablesValue)
		defer switchValues(newSetupRoutesAndIPTablesValue, mockSetupRoutesAndIPTablesValue)
		return setupRoutesAndIPTables(nic, addr, bridge, ifinfo)
	}
}
