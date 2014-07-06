// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudinit/sshinit"
	"github.com/juju/juju/network"
	apinetworker "github.com/juju/juju/state/api/networker"
	//"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.networker")

// Patches for testing.
var (
	NetworkDir          = "/etc/network"
	InterfaceIsUp       = interfaceIsUp
	InterfaceHasAddress = interfaceHasAddress
	ExecuteCommands     = executeCommands
)

// Networker configures network interfaces on the machine, as needed.
type Networker struct {
	st  *apinetworker.State
	tag string
}

// Internal struct to store information required to setup networks.
type setupInfo struct {
	// The current contents of '/etc/network/interfaces' file.
	ifxData []byte

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
	return worker.NewNotifyWorker(nw)
}

func (nw *Networker) SetUp() (watcher.NotifyWatcher, error) {
	return nw.st.WatchInterfaces(nw.tag)
}

func (s *setupInfo) ensureVLANModule() {
	commands := []string{
		`dpkg-query -s vlan || ` + sshinit.Aptget + ` install vlan`,
		`lsmod | grep -q 8021q || modprobe 8021q`,
		`grep -q 8021q /etc/modules || echo 8021q >> /etc/modules`,
		`vconfig set_name_type DEV_PLUS_VID_NO_PAD`,
	}
	s.commands = append(s.commands, commands...)
}

func (s *setupInfo) bringUpInterfaces() {
	for _, network := range s.networks {
		name := network.ActualInterfaceName()
		if name != "eth0" && !InterfaceIsUp(name) {
			command := fmt.Sprintf(`ifup %s`, name)
			s.commands = append(s.commands, command)
		}
	}
}

func interfacePattern(interfaceName string) string {
	if interfaceName == "eth0" {
		return fmt.Sprintf("\nauto %s\nsource %s/%s.config\n", interfaceName, NetworkDir, interfaceName)
	}
	return fmt.Sprintf("\nauto %s\niface %s inet dhcp\n", interfaceName, interfaceName)
}

func (s *setupInfo) interfaceIsConfigured(interfaceName string) bool {
	substr := interfacePattern(interfaceName)
	return strings.Contains(string(s.ifxData), substr)
}

func (s *setupInfo) configureInterfaces() {
	for _, network := range s.networks {
		name := network.ActualInterfaceName()
		if name != "eth0" {
			s.configureInterface(name)
		}
	}
}

func (s *setupInfo) configureInterface(interfaceName string) {
	substr := interfacePattern(interfaceName)
	if !strings.Contains(string(s.ifxData), substr) {
		s.ifxData = append(s.ifxData, []byte(substr)...)
	}
}

// Execute slice of commands one by one.
func executeCommands(commands []string) error {
	for _, command := range commands {
		result, err := exec.RunCommands(exec.RunParams{
			Commands:   command,
			WorkingDir: "/",
		})
		if err != nil {
			err := fmt.Errorf("failed to execute %q: %v", command, err)
			logger.Errorf("%s", err.Error())
			return err
		}
		if result.Code != 0 {
			err := fmt.Errorf("command %q failed (code: %d, stdout: %s, stderr: %s)",
				command, result.Code, result.Stdout, result.Stderr)
			logger.Errorf("%s", err.Error())
			return err
		}
	}
	return nil
}

// Verify that the interface is up.
func interfaceIsUp(interfaceName string) bool {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		logger.Errorf("cannot find network interface %q: %v", interfaceName, err)
		return false
	}
	return (iface.Flags & net.FlagUp) != 0
}

// Verify that the interface has assigned address.
func interfaceHasAddress(interfaceName string) bool {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		logger.Errorf("cannot find network interface %q: %v", interfaceName, err)
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		logger.Errorf("cannot get addresses for network interface %q: %v", interfaceName, err)
		return false
	}
	return len(addrs) != 0
}

func (nw *Networker) Handle() error {
	s := &setupInfo{}
	var err error
	interfacesFile := NetworkDir + "/interfaces"
	s.ifxData, err = ioutil.ReadFile(interfacesFile)
	if err != nil {
		logger.Errorf("failed to read %s: %v", interfacesFile, err)
		return err
	}
	s.networks, err = nw.st.MachineNetworkInfo(nw.tag)
	logger.Infof("s.networks=%#v", s.networks)
	if err != nil {
		logger.Errorf("failed to process network info: %v", err)
		return err
	}

	// Verify that eth0 interfaces is properly configured and brought up.
	if !s.interfaceIsConfigured("eth0") || !InterfaceHasAddress("eth0") {
		if !s.interfaceIsConfigured("eth0") {
			logger.Errorf("interface eth0 has to be configured")
		}
		if !InterfaceHasAddress("eth0") {
			logger.Errorf("interface eth0 has to be bring up")
		}
		logger.Errorf("interface eth0 has to be configured and bring up")
		return fmt.Errorf("interface eth0 has to be configured and bring up")
	}

	// Update interface file
	s.configureInterfaces()
	err = ioutil.WriteFile(interfacesFile, s.ifxData, 0644)
	if err != nil {
		logger.Errorf("failed to write %s: %v", interfacesFile, err)
		return err
	}

	// Generate a list of commands to configure networks.
	s.ensureVLANModule()
	s.bringUpInterfaces()

	err = ExecuteCommands(s.commands)
	return err
}

func (nw *Networker) TearDown() error {
	// Nothing to do here.
	return nil
}
