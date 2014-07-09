// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"net"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/network"
	apinetworker "github.com/juju/juju/state/api/networker"
	//"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.networker")

// Patches for testing.
var (
	InterfaceIsUp       = interfaceIsUp
	InterfaceHasAddress = interfaceHasAddress
	ExecuteCommands     = executeCommands
)

// Interface for juju internal network
var InternalInterface = "eth0"

// Networker configures network interfaces on the machine, as needed.
type Networker struct {
	st  *apinetworker.State
	tag string
}

// Internal struct to store information required to setup networks.
type setupInfo struct {
	// The name and contents of '/etc/network/interfaces' file.
	FileInfo

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

const newSchema = `
# Source interfaces
# Please check %s/interfaces.d before changing this file
# as interfaces may have been defined in %s/interfaces.d
# NOTE: the primary ethernet device is defined in
# %s/interfaces.d/eth0.cfg
# See LP: #1262951
source %s/interfaces.d/*.cfg
`

func (s *setupInfo) ensureVLANModule() {
	commands := []string{
		`dpkg-query -s vlan || apt-get --option Dpkg::Options::=--force-confold --assume-yes install vlan`,
		`lsmod | grep -q 8021q || modprobe 8021q`,
		`grep -q 8021q /etc/modules || echo 8021q >> /etc/modules`,
		`vconfig set_name_type DEV_PLUS_VID_NO_PAD`,
	}
	s.commands = append(s.commands, commands...)
}

func (s *setupInfo) bringUpInterfaces() {
	for _, network := range s.networks {
		name := network.ActualInterfaceName()
		if name != InternalInterface && !InterfaceIsUp(name) {
			command := fmt.Sprintf(`ifup %s`, name)
			s.commands = append(s.commands, command)
		}
	}
}

func interfacePattern(interfaceName string) string {
	if interfaceName == InternalInterface {
		return fmt.Sprintf("\nauto %s\nsource %s/%s.config\n", interfaceName, NetworkDir, interfaceName)
	}
	return fmt.Sprintf("\nauto %s\niface %s inet dhcp\n", interfaceName, interfaceName)
}

func (s *setupInfo) interfaceIsConfigured(interfaceName string) bool {
	substr := interfacePattern(interfaceName)
	file, ok := s.Files[interfaceName]
	return ok && strings.Contains(file.Data, substr)
}

func (s *setupInfo) configureInterfaces() {
	for _, network := range s.networks {
		name := network.ActualInterfaceName()
		if name != InternalInterface {
			s.configureInterface(name)
		}
	}
}

func (s *setupInfo) configureInterface(interfaceName string) {
	substr := interfacePattern(interfaceName)
	if !strings.Contains(s.Files[interfaceName].Data, substr) {
		s.Files[interfaceName] = &File{
			FileName: fmt.Sprintf("%s/%s.cfg", IfacesDirName, interfaceName),
			Data:     substr,
			Op:       DoWrite,
		}
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
	err := s.FileInfo.ReadInterfacesFiles()
	if err != nil {
		return err
	}
	s.networks, err = nw.st.MachineNetworkInfo(nw.tag)
	logger.Infof("s.networks=%#v", s.networks)
	if err != nil {
		logger.Errorf("failed to process network info: %v", err)
		return err
	}

	// Verify that internal interface is properly configured and brought up.
	if !s.interfaceIsConfigured(InternalInterface) || !InterfaceHasAddress(InternalInterface) {
		if !s.interfaceIsConfigured(InternalInterface) {
			logger.Errorf("interface %q has to be configured", InternalInterface)
		}
		if !InterfaceHasAddress(InternalInterface) {
			logger.Errorf("interface %q has to be bring up", InternalInterface)
		}
		return fmt.Errorf("interface %q has to be configured and bring up", InternalInterface)
	}

	// Update interface file
	s.configureInterfaces()
	err = s.FileInfo.WriteInterfacesFiles()
	if err != nil {
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
