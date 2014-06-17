// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"net"
	"strings"
	"io/ioutil"

	"github.com/juju/loggo"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/network"
	apinetworker "github.com/juju/juju/state/api/networker"
	"github.com/juju/juju/state/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.networker")

// Networker ensures the minimum number of units for services is respected.
type Networker struct {
	st *apinetworker.State
	tag string
}

// Ifternal struct to store information required to setup networks.
type SetupInfo struct {
	// The current contents of '/etc/network/interfaces' file.
	ifxData []byte

	// The result of net.Interfaces() call.
	ifxs []net.Interface

	// The result of MachineNetworkInfo API call.
	networks []network.Info

	// Generated list of commands to execute.
	commands []string
}

// NewNetworker returns a Worker that handles machine networking configuration.
func NewNetworker(st *apinetworker.State, agentConfig agent.Config) worker.Worker {
	nw := &Networker{
		st:  st,
		tag: agentConfig.Tag(),
	}
	return worker.NewStringsWorker(nw)
}

func (nw *Networker) SetUp() (watcher.StringsWatcher, error) {
	s := &SetupInfo{}
	var err error
	s.ifxs, err = net.Interfaces()
	if err != nil {
		logger.Errorf("failed to get OS interfaces: %v", err)
		return nil, err
	}
	s.ifxData, err = ioutil.ReadFile("/etc/network/interfaces")
	if err != nil {
		logger.Errorf("failed to read /etc/network/interfaces: %v", err)
		return nil, err
	}
	s.networks, err = nw.st.MachineNetworkInfo(nw.tag)
	if err != nil {
		logger.Errorf("failed to process network info: %v", err)
		return nil, err
	}

	// Verify that eth0 interfaces is properly configured and brought up in cloud-init.
	if (!s.interfaceIsConfigured("eth0") || !s.interfaceHasAddress("eth0")) {
		return nil, fmt.Errorf("interface eth0 has to be configured and bring up in cloud-init")
	}

	// Generate a list of commands to configure networks.
	s.ensureVLANModule()
	s.configureInterfaces()
	s.bringUpInterfaces()

	logger.Errorf("commands=%#v", s.commands)

	// Execute all commands one by one.
	for _, command := range s.commands {
		result, err := exec.RunCommands(exec.RunParams{
			Commands: command,
			WorkingDir: "/",
			Environment: nil,
		})
		if err != nil {
			err := fmt.Errorf("failed to execute %q: %v", command, err)
			logger.Errorf("%v", err)
			return nil, err
		}
		if result.Code != 0 {
			err := fmt.Errorf("command %q failed with the status %d", command, result.Code)
			logger.Errorf("%v", err)
			logger.Errorf("stdout: %v", result.Stdout)
			logger.Errorf("stderr: %v", result.Stderr)
			return nil, err
		}
	}
	return nil, nil
}

// The options specified are to prevent any kind of prompting.
//  * --assume-yes answers yes to any yes/no question in apt-get;
//  * the --force-confold option is passed to dpkg, and tells dpkg
//    to always keep old configuration files in the face of change.
const aptget = "apt-get --option Dpkg::Options::=--force-confold --assume-yes "

func (s *SetupInfo)ensureVLANModule() {
	commands := []string {
		`dpkg-query -s vlan || "+aptget+"install vlan`,
		`lsmod | grep -q 8021q || modprobe 8021q`,
		`grep -q 8021q /etc/modules || echo 8021q >> /etc/modules'`,
		`vconfig set_name_type DEV_PLUS_VID_NO_PAD`,
	}
	s.commands = append(s.commands, commands...)
}

func (s *SetupInfo) configureInterfaces() {
	for _, network := range s.networks {
		name := network.ActualInterfaceName()
		if name != "eth0" && !s.interfaceIsConfigured(name) {
			command := fmt.Sprintf(`printf "\nauto %s\niface %s inet dhcp\n" >>/etc/network/interfaces`, name, name)
			s.commands = append(s.commands, command)
		}
	}
}

func (s *SetupInfo) bringUpInterfaces() {
	for _, network := range s.networks {
		name := network.ActualInterfaceName()
		if name != "eth0" && !s.interfaceIsUp(name) {
			command := fmt.Sprintf(`ifup %s`, name)
			s.commands = append(s.commands, command)
		}
	}
}

func (s *SetupInfo) interfaceIsConfigured(name string) bool {
	var substr string
	if name == "eth0" {
		substr = fmt.Sprintf("\nauto %s\nsource /etc/network/%s.config\n", name, name)
	} else {
		substr = fmt.Sprintf("\nauto %s\niface %s inet dhcp\n", name, name)
	}
	return strings.Contains(string(s.ifxData), substr)
}

func (s *SetupInfo) interfaceIsUp(name string) bool {
	for _, ifx := range s.ifxs {
		if ifx.Name == name {
			return (ifx.Flags & net.FlagUp) != 0
		}
	}
	return false
}

func (s *SetupInfo) interfaceHasAddress(name string) bool {
	for _, ifx := range s.ifxs {
		if ifx.Name == name {
			addrs, err := ifx.Addrs()
			if err != nil {
				return false
			}
			return len(addrs) != 0
		}
	}
	return false
}

func (nw *Networker) Handle(networkNames []string) error {
	// Nothing to do here.
	return nil
}

func (nw *Networker) TearDown() error {
	// Nothing to do here.
	return nil
}
