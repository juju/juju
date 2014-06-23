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
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.networker")

// Networker configures network interfaces on the machine, as needed.
type Networker struct {
	st  *apinetworker.State
	tag string
}

// Internal struct to store information required to setup networks.
type setupInfo struct {
	// The current contents of '/etc/network/interfaces' file.
	ifxData []byte

	// The result of net.Interfaces() call.
	ifaces []net.Interface

	// The result of MachineNetworkInfo API call.
	networks []network.Info

	// Generated list of commands to execute.
	commands []string
}

// nopCaller implements base.Caller and never calls the API.
// Once we can watch networks, drop this.
type nopCaller struct{}

func (c *nopCaller) Call(_, _, _ string, _, _ interface{}) error {
	return nil
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
	s := &setupInfo{}
	var err error
	s.ifaces, err = net.Interfaces()
	if err != nil {
		logger.Errorf("failed to get OS interfaces: %v", err)
		return nil, err
	}
	s.ifxData, err = ioutil.ReadFile("/etc/network/interfaces")
	if err != nil {
		logger.Errorf("failed to read /etc/network/interfaces: %v", err)
		return nil, err
	}
	logger.Infof("nw.tag=%v", nw.tag)
	s.networks, err = nw.st.MachineNetworkInfo(nw.tag)
	logger.Infof("s.networks=%#v", s.networks)
	logger.Infof("err=%#v", err)
	if err != nil {
		logger.Errorf("failed to process network info: %v", err)
		return nil, err
	}

	// Verify that eth0 interfaces is properly configured and brought up.
	if !s.interfaceIsConfigured("eth0") || !s.interfaceHasAddress("eth0") {
		return nil, fmt.Errorf("interface eth0 has to be configured and bring up")
	}

	// Generate a list of commands to configure networks.
	s.ensureVLANModule()
	s.configureInterfaces()
	s.bringUpInterfaces()

	logger.Errorf("commands=%#v", s.commands)

	// Execute all commands one by one.
	for _, command := range s.commands {
		result, err := exec.RunCommands(exec.RunParams{
			Commands:    command,
			WorkingDir:  "/",
		})
		if err != nil {
			err := fmt.Errorf("failed to execute %q: %v", command, err)
			logger.Errorf("%s", err.Error())
			return nil, err
		}
		if result.Code != 0 {
			err := fmt.Errorf("command %q failed (code: %d, stdout: %v, stderr: %v)",
				command, result.Code, result.Stdout, result.Stderr)
			logger.Errorf("%s", err.Error())
			return nil, err
		}
	}
	return watcher.NewStringsWatcher(&nopCaller{}, params.StringsWatchResult{}), nil
}

func (s *setupInfo) ensureVLANModule() {
	commands := []string{
		`dpkg-query -s vlan || `+sshinit.Aptget+` install vlan`,
		`lsmod | grep -q 8021q || modprobe 8021q`,
		`grep -q 8021q /etc/modules || echo 8021q >> /etc/modules'`,
		`vconfig set_name_type DEV_PLUS_VID_NO_PAD`,
	}
	s.commands = append(s.commands, commands...)
}

func (s *setupInfo) configureInterfaces() {
	for _, network := range s.networks {
		name := network.ActualInterfaceName()
		if name != "eth0" && !s.interfaceIsConfigured(name) {
			command := fmt.Sprintf(`printf "\nauto %s\niface %s inet dhcp\n" >>/etc/network/interfaces`, name, name)
			s.commands = append(s.commands, command)
		}
	}
}

func (s *setupInfo) bringUpInterfaces() {
	for _, network := range s.networks {
		name := network.ActualInterfaceName()
		if name != "eth0" && !s.interfaceIsUp(name) {
			command := fmt.Sprintf(`ifup %s`, name)
			s.commands = append(s.commands, command)
		}
	}
}

func (s *setupInfo) interfaceIsConfigured(name string) bool {
	var substr string
	if name == "eth0" {
		substr = fmt.Sprintf("\nauto %s\nsource /etc/network/%s.config\n", name, name)
	} else {
		substr = fmt.Sprintf("\nauto %s\niface %s inet dhcp\n", name, name)
	}
	return strings.Contains(string(s.ifxData), substr)
}

func (s *setupInfo) interfaceIsUp(name string) bool {
	for _, iface := range s.ifaces {
		if iface.Name == name {
			return (iface.Flags & net.FlagUp) != 0
		}
	}
	return false
}

func (s *setupInfo) interfaceHasAddress(name string) bool {
	for _, iface := range s.ifaces {
		if iface.Name == name {
			addrs, err := iface.Addrs()
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
