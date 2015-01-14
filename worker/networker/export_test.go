// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"github.com/juju/names"

	"github.com/juju/juju/network"
)

// NewConfigFile is a helper use to create a *configFile for testing.
func NewConfigFile(interfaceName, fileName string, info network.InterfaceInfo, data []byte) ConfigFile {
	return &configFile{
		interfaceName: interfaceName,
		fileName:      fileName,
		interfaceInfo: info,
		data:          data,
	}
}

// IsRunningInLXC is a helper for testing isRunningInLXC.
func IsRunningInLXC(machineId string) bool {
	nw := &Networker{tag: names.NewMachineTag(machineId)}
	return nw.isRunningInLXC()
}

// IsVLANModuleLoaded returns whether 8021q kernel module has been
// loaded.
func (nw *Networker) IsVLANModuleLoaded() bool {
	return nw.isVLANSupportInstalled
}
