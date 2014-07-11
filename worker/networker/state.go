// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"sort"

	"github.com/juju/juju/network"
)

// Internal struct to store information required to setup networks.
type configState struct {
	// The name and contents of '/etc/network/interfaces' file.
	configFiles ConfigFiles

	// The result of MachineNetworkInfo API call.
	networkInfo []network.Info

	// Generated list of commands to execute.
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

func (s *configState) ifup(ifaceName string) {
	command := fmt.Sprintf("ifup %s", ifaceName)
	s.commands = append(s.commands, command)
}

func (s *configState) ifdown(ifaceName string) {
	command := fmt.Sprintf("ifdown %s || :", ifaceName)
	s.commands = append(s.commands, command)
}

func (s *configState) bringUpInterfaces() {
	s.commands = nil

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
					s.ifup(ifaceName)
				} else if !InterfaceIsUp(ifaceName) {
					s.ifup(ifaceName)
				}
			}
		}
	}
	sort.Sort(sort.StringSlice(s.commands))
}

func (s *configState) lookupNetworkInfo(ifaceName string) *network.Info {
	for _, info := range s.networkInfo {
		if ifaceName == info.ActualInterfaceName() {
			return &info
		}
	}
	return nil
}

// bringDownInterfaces generates command to down interfaces.
// Changes to config files are done by bringUpInterfaces.
func (s *configState) bringDownInterfaces() {
	s.commands = nil

	// Iterate by existing config files.
	for ifaceName, _ := range s.configFiles {
		if ifaceName != privateInterface && ifaceName != privateBridge {
			// Interface goes down if it was disabled or its config was changed
			info := s.lookupNetworkInfo(ifaceName)
			if (info == nil || info.Disabled) && InterfaceIsUp(ifaceName) {
				s.ifdown(ifaceName)
			} else if s.configFiles.isChanged(ifaceName, s.configText(ifaceName, info)) {
				s.ifdown(ifaceName)
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(s.commands)))
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

// configText generate configuration text for interface based on the state
func (s *configState) configText(interfaceName string, info *network.Info) string {
	return fmt.Sprintf("inet dhcp\n")
}
