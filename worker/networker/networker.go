// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"

	"github.com/juju/loggo"

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

// NewNetworker returns a Worker that handles machine networking configuration.
func NewNetworker(st *apinetworker.State, agentConfig agent.Config) worker.Worker {
	nw := &Networker{
		st:  st,
		tag: agentConfig.Tag(),
	}
	return worker.NewStringsWorker(nw)
}

func (nw *Networker) SetUp() (watcher.StringsWatcher, error) {
	networks, err := nw.st.MachineNetworkInfo(nw.tag)
	if err != nil {
		logger.Errorf("failed to process network info: %v", err)
		return nil, err
	}
	for _, network := range networks {
		name := network.NetworkName
		logger.Debugf("processing network %q", name)
		if err := nw.handleOneNetwork(name, networks); err != nil {
			logger.Errorf("failed to process network %q: %v", name, err)
			return nil, err
		}
	}

	return nil, nil
}

func (nw *Networker) handleOneNetwork(networkName string, networks []network.Info) error {
	found := false
	var network network.Info
	for _, network = range networks {
		if network.NetworkName == networkName {
			found = true
			logger.Debugf("setting network %#v", network)
		}
	}
	if !found {
		logger.Errorf("failed to find network %q", networkName)
		return fmt.Errorf("failed to find network %q", networkName)
	}
	logger.Debugf("network=%#v", network)
	return nil // service.EnsureMinUnits()
}

// The options specified are to prevent any kind of prompting.
//  * --assume-yes answers yes to any yes/no question in apt-get;
//  * the --force-confold option is passed to dpkg, and tells dpkg
//    to always keep old configuration files in the face of change.
const aptget = "apt-get --option Dpkg::Options::=--force-confold --assume-yes "

func installNetworkPackages(commands *[]string) error {
	newCommands := []string {
		`dpkg-query -s bridge-utils || "+aptget+"install bridge-utils`,
		`dpkg-query -s vlan || "+aptget+"install vlan`,
		`lsmod | grep -q 8021q || modprobe 8021q`,
		`grep -q 8021q /etc/modules || echo 8021q >> /etc/modules'`,
		`vconfig set_name_type DEV_PLUS_VID_NO_PAD`,
	}
	commands = append(commands, newCommands...)
	return nil
}

func verifyInterfaceConfigFile(commands *[]string) error {
	
}


func (nw *Networker) Handle(networkNames []string) error {
	// Nothing to do here.
	return nil
}

func (nw *Networker) TearDown() error {
	// Nothing to do here.
	return nil
}
