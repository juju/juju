// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"fmt"

	"github.com/juju/loggo"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs/network"
	apinetworker "github.com/juju/juju/state/api/networker"
	"github.com/juju/juju/state/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.networker")

// NetworkerWorker ensures the minimum number of units for services is respected.
type NetworkerWorker struct {
	st *apinetworker.State
	tag string
}

// NewNetworkerWorker returns a Worker that brings up and down networks.
func NewNetworkerWorker(st *apinetworker.State, agentConfig agent.Config) worker.Worker {
	nw := &NetworkerWorker{
		st:  st,
		tag: agentConfig.Tag(),
	}
	return worker.NewStringsWorker(nw)
}

func (nw *NetworkerWorker) SetUp() (watcher.StringsWatcher, error) {
	networks, err := nw.st.MachineNetworkInfo(nw.tag)
	if err != nil {
		logger.Errorf("failed to process network info: %v", err)
		return nil, err
	}
	for _, network := range networks {
		name := network.NetworkName
		logger.Infof("processing network %q", name)
		if err := nw.handleOneNetwork(name, networks); err != nil {
			logger.Errorf("failed to process network %q: %v", name, err)
			return nil, err
		}
	}

	//return nw.st.WatchNetworks(), nil
	return nil, nil
}

func (nw *NetworkerWorker) handleOneNetwork(networkName string, networks []network.Info) error {
	found := false
	var network network.Info
	for _, network = range networks {
		if network.NetworkName == networkName {
			found = true
			logger.Infof("setting network %#v", network)
		}
	}
	if !found {
		logger.Errorf("failed to find network %q", networkName)
		return fmt.Errorf("failed to find network %q", networkName)
	}
	logger.Infof("network=%#v", network)
	return nil // service.EnsureMinUnits()
}

func (nw *NetworkerWorker) Handle(networkNames []string) error {
	networks, err := nw.st.MachineNetworkInfo(nw.tag)
	if err != nil {
		logger.Errorf("failed to process network info: %v", err)
		return err
	}
	for _, name := range networkNames {
		logger.Infof("processing network %q", name, networks)
		if err := nw.handleOneNetwork(name, networks); err != nil {
			logger.Errorf("failed to process network %q: %v", name, err)
			return err
		}
	}
	return nil
}

func (nw *NetworkerWorker) TearDown() error {
	// Nothing to do here.
	return nil
}
