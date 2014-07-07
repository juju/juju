// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"syscall"

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
	// The name and contents of '/etc/network/interfaces' file.
	ifaceFileName string
	ifaceFileData []byte

	// The name and contents of files from '/etc/network/interfaces.d/' directory.
	ifaceDirName string
	ifaceDirData map[string][]byte

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
	return strings.Contains(string(s.ifaceFileData), substr)
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
	if !strings.Contains(string(s.ifaceFileData), substr) {
		s.ifaceFileData = append(s.ifaceFileData, []byte(substr)...)
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

func (s *setupInfo) ReadInterfacesFiles() error {
	var err error
	s.ifaceFileData, err = ioutil.ReadFile(s.ifaceFileName)
	if err != nil {
		logger.Errorf("failed to read file %q: %v", s.ifaceFileName, err)
		return err
	}
	files, err := ioutil.ReadDir(s.ifaceDirName)
	if err != nil {
		if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOENT {
                        // ignore error when directory is absent
                } else {
                        logger.Errorf("failed to read directory %q: %#v\n", s.ifaceDirName, err)
                        return err
                }
	}
	for _, file := range files {
		if file.Mode().IsRegular() {
			fileName := s.ifaceDirName + "/" + file.Name()
			data, err := ioutil.ReadFile(fileName)
			if err != nil {
				// just report and ignore the error
				logger.Errorf("failed to read file %q: %v", fileName, err)
			}
			s.ifaceDirData[file.Name()] = data
		}
	}
	return nil
}

func (s *setupInfo) WriteInterfacesFiles() error {
	err := ioutil.WriteFile(s.ifaceFileName, s.ifaceFileData, 0644)
	if err != nil {
		logger.Errorf("failed to white file %q: %v", s.ifaceFileName, err)
		return err
	}
	err = os.Mkdir(s.ifaceDirName, 0755)
	if err != nil {
		if e, ok := err.(*os.PathError); ok && e.Err == syscall.EEXIST {
                        // ignore error when directory already exists
                } else {
                        logger.Errorf("failed to create directory %q: %#v\n", s.ifaceDirName, err)
                        return err
                }
	}
	for name, data := range s.ifaceDirData {
		fileName := s.ifaceDirName + "/" + name
		err = ioutil.WriteFile(fileName, data, 0644)
		if err != nil {
			logger.Errorf("failed to write file %q: %v", fileName, err)
			return err
		}
	}
	return nil
}

func (nw *Networker) Handle() error {
	s := &setupInfo{
		ifaceFileName: NetworkDir + "/interfaces",
		ifaceDirName: NetworkDir + "/interfaces.d",
		ifaceDirData: make(map[string][]byte),
	}
	err := s.ReadInterfacesFiles()
	if err != nil {
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
	err = s.WriteInterfacesFiles()
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
