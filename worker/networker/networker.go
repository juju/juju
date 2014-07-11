// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"

	"github.com/juju/loggo"

	"github.com/juju/juju/agent"
	apinetworker "github.com/juju/juju/state/api/networker"
	"github.com/juju/juju/state/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.networker")

// Interface and bridge interface for juju internal network
var privateInterface string
var privateBridge string

// networker configures network interfaces on the machine, as needed.
type networker struct {
	st  *apinetworker.State
	tag string
}

// NewNetworker returns a Worker that handles machine networking configuration.
func NewNetworker(st *apinetworker.State, agentConfig agent.Config) worker.Worker {
	nw := &networker{
		st:  st,
		tag: agentConfig.Tag(),
	}
	return worker.NewNotifyWorker(nw)
}

func (nw *networker) SetUp() (watcher.NotifyWatcher, error) {
	var err error
	s := &configState{}

	// Read network configuration files and revert modifications made by MAAS.
	if err = s.configFiles.readAll(); err != nil {
		return nil, err
	}
	if err = s.configFiles.fixMAAS(); err != nil {
		return nil, err
	}

	// Need to configure network interface for juju internal network.
	privateInterface = "eth0"
	if s.configFiles["br0"] != nil {
		privateBridge = "br0"
	} else {
		privateBridge = privateInterface
	}

	// Verify that internal interface is properly configured and brought up.
	if !InterfaceIsUp(privateInterface) {
		logger.Errorf("interface %q has to be up", privateInterface)
		return nil, fmt.Errorf("interface %q has to be up", privateInterface)
	}
	if !InterfaceIsUp(privateBridge) || !InterfaceHasAddress(privateBridge) {
		logger.Errorf("interface %q has to be up", privateBridge)
		return nil, fmt.Errorf("interface %q has to be up", privateBridge)
	}

	// Add commands to install VLAN module.
	s.ensureVLANModule()

	// Apply changes.
	if err = s.apply(); err != nil {
		return nil, err
	}
	return nw.st.WatchInterfaces(nw.tag)
}

func (nw *networker) Handle() error {
	var err error
	s := &configState{}

	// Read configuration files for managed interfaces.
	if err = s.configFiles.readManaged(); err != nil {
		return err
	}

	// Read network info from the state
	if s.networkInfo, err = nw.st.MachineNetworkInfo(nw.tag); err != nil {
		logger.Errorf("failed to process network info: %v", err)
		return err
	}

	// Down disabled interfaces.
	s.bringDownInterfaces()
	if err := s.apply(); err != nil {
		return err
	}

	// Up configured interfaces.
	s.bringUpInterfaces()
	if err := s.apply(); err != nil {
		return err
	}

	return nil
}

func (nw *networker) TearDown() error {
	// Nothing to do here.
	return nil
}
