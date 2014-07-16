// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"sort"

	"github.com/juju/juju/network"
)

// configState is a struct to store information required to setup networks,
// both the content of configuration file and data from the state.
type configState struct {
	// configFiles contains the content of configuration files.
	configFiles ConfigFiles

	// networkInfo contains the result of MachineNetworkInfo API call.
	networkInfo []network.Info

	// commands contains the generated list of commands to execute.
	commands []string
}

// apply writes updates to network config files and executes commands to bring up and down interfaces.
func (s *configState) apply() error {
	if err := s.configFiles.writeOrRemove(); err != nil {
		return err
	}
	if err := ExecuteCommands(s.commands); err != nil {
		return err
	}
	return nil
}

// resetCommands reset accumulated command slice.
func (s *configState) resetCommands() {
	s.commands = []string{}
}

// bringUpInterfaces generates a set of ifup commands to bring up required interfaces.
func (s *configState) bringUpInterfaces() {
	upIfaces := []string{}

	// Iterate by state networks infos.
	for _, info := range s.networkInfo {
		ifaceName := info.ActualInterfaceName()
		if ifaceName != privateInterface && ifaceName != privateBridge {
			if info.Disabled {
				s.configFiles.removeManaged(ifaceName)
			} else {
				configText := s.configText(ifaceName, &info)
				if s.configFiles.isChanged(ifaceName, configText) {
					s.configFiles.addManaged(ifaceName, configText)
					upIfaces = append(upIfaces, ifaceName)
				} else if !InterfaceIsUp(ifaceName) {
					upIfaces = append(upIfaces, ifaceName)
				}
			}
		}
	}

	// Sort the interfaces to ensure that raw interface goes up before his virtual descendants.
	// E.g. eth1 go up before eth1.42 and eth1:2.
	sort.Sort(sort.StringSlice(upIfaces))
	for _, ifaceName := range upIfaces {
		s.commands = append(s.commands, "ifup "+ifaceName)
	}
}

// lookupNetworkInfo lookups for in network.Info slice for interface by its actual name.
func (s *configState) lookupNetworkInfo(ifaceName string) *network.Info {
	for _, info := range s.networkInfo {
		if ifaceName == info.ActualInterfaceName() {
			return &info
		}
	}
	return nil
}

// bringDownInterfaces generates a set of commands to down unneeded interfaces.
// Changes to config files are done by bringUpInterfaces.
func (s *configState) bringDownInterfaces() {
	downIfaces := []string{}

	// Iterate by existing config files.
	for ifaceName, _ := range s.configFiles {
		if ifaceName != privateInterface && ifaceName != privateBridge {
			// Interface goes down if it was disabled or its config was changed
			info := s.lookupNetworkInfo(ifaceName)
			if (info == nil || info.Disabled) && InterfaceIsUp(ifaceName) {
				downIfaces = append(downIfaces, ifaceName)
			} else if s.configFiles.isChanged(ifaceName, s.configText(ifaceName, info)) {
				downIfaces = append(downIfaces, ifaceName)
			}
		}
	}
	// Sort the interfaces to ensure that raw interface goes down only after his virtual descendants.
	sort.Sort(sort.Reverse(sort.StringSlice(downIfaces)))
	for _, ifaceName := range downIfaces {
		s.commands = append(s.commands, "ifdown "+ifaceName)
	}
}

func (s *configState) ensureVLANModule() {
	commands := []string{
		`dpkg-query -s vlan || apt-get --option Dpkg::Options::=--force-confold --assume-yes install vlan`,
		`lsmod | grep -q 8021q || modprobe 8021q`,
		`grep -q 8021q /etc/modules || echo 8021q >> /etc/modules`,
		`vconfig set_name_type DEV_PLUS_VID_NO_PAD`,
	}
	s.commands = append(s.commands, commands...)
}

// configText generate configuration text for interface based on its configuration.
func (s *configState) configText(interfaceName string, info *network.Info) string {
	if info == nil {
		return ""
	}
	text := fmt.Sprintf("auto %s\niface %s inet dhcp\n", interfaceName, interfaceName)

	// Add vlan-raw-device line for VLAN interfaces.
	if info.VLANTag != 0 {
		suffix := fmt.Sprintf(".%d", info.VLANTag)
		if len(interfaceName) > len(suffix) && interfaceName[len(interfaceName)-len(suffix):] == suffix {
			text += fmt.Sprintf("\tvlan-raw-device %s\n", interfaceName[:len(interfaceName)-len(suffix)])
		}
	}
	return text
}
