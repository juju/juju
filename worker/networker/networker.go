// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
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
	st                     *apinetworker.State
	tag                    string
	isVLANSupportInstalled bool
}

// NewNetworker returns a Worker that handles machine networking configuration.
func NewNetworker(st *apinetworker.State, agentConfig agent.Config) worker.Worker {
	nw := &networker{
		st:  st,
		tag: agentConfig.Tag().String(),
	}
	return worker.NewNotifyWorker(nw)
}

func (nw *networker) SetUp() (watcher.NotifyWatcher, error) {
	s := &configState{}

	// Read network configuration files and revert modifications made by MAAS.
	if err := s.configFiles.readAll(); err != nil {
		return nil, err
	}
	if err := s.configFiles.fixMAAS(); err != nil {
		return nil, err
	}

	// Need to configure network interface for juju internal network.
	privateInterface = "eth0"
	if s.configFiles[ifaceConfigFileName("br0")] != nil {
		privateBridge = "br0"
	} else {
		privateBridge = privateInterface
	}

	// Apply changes.
	if err := s.apply(); err != nil {
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
	if err = s.apply(); err != nil {
		return err
	}

	// Reset the list of executed commands.
	s.resetCommands()

	// Add commands to install VLAN module, if required.
	if !nw.isVLANSupportInstalled {
		s.ensureVLANModule()
		nw.isVLANSupportInstalled = true
	}

	// Up configured interfaces.
	s.bringUpInterfaces()
	if err = s.apply(); err != nil {
		return err
	}

	return nil
}

func (nw *networker) TearDown() error {
	// Nothing to do here.
	return nil
}
