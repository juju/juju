// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
)

const (
	Version = 1
)

// commandFunc is a function that will be sent to the backend
// to work on the topology data.
type commandFunc func(*topologyData) error

// command will be sent to the backend goroutine to perform
// the task and return a possible error.
type command struct {
	cmd       commandFunc
	errorChan chan error
}

// newCommand creates a new command with the given task
// and a new channel for the feedback.
func newCommand(cf commandFunc) *command {
	return &command{cf, make(chan error)}
}

// execute performs the command and returns a
// possible error
func (c *command) execute(td *topologyData) {
	c.errorChan <- c.cmd(td)
}

// topologyData is the data stored inside a topology.
type topologyData struct {
	Version      int                 "version"
	Services     map[string]*Service "services"
	UnitSequence map[string]int      "unit-sequence"
}

// newTopologyData creates an initial empty topology data with
// version 1.
func newTopologyData() *topologyData {
	return &topologyData{
		Version:      1,
		Services:     make(map[string]*Service),
		UnitSequence: make(map[string]int),
	}
}

// sync synchronizes the topology data with newly fetched
// data. So existing instances will be updated or maybe even
// invalidated and new ones added. This is done after receiving
// a modification signal via the ZK channel.
func (td *topologyData) sync(newTD *topologyData) error {
	// 1. Version.
	td.Version = newTD.Version

	// 2. Services. First store current ids, then update
	// or add new services and at last remove invalid
	// services.
	currentServiceIds := make(map[string]bool)

	for id, _ := range td.Services {
		currentServiceIds[id] = true
	}

	for id, s := range newTD.Services {
		if currentService, ok := td.Services[id]; ok {
			// Sync service and mark as synced.
			// OPEN: How to handle exposed services?
			currentService.sync(s)

			delete(currentServiceIds, id)
		} else {
			// Add new service.
			td.Services[id] = s
		}
	}

	for id, _ := range currentServiceIds {
		// Mark as exposed for those who still have references.
		td.Services[id].Exposed = true

		delete(td.Services, id)
	}

	// 3. UnitSequence. Just a simple map, so take the
	// new one.
	td.UnitSequence = newTD.UnitSequence

	return nil
}

// topology is an internal helper handling the topology informations
// inside of ZooKeeper.
type topology struct {
	zk          *zookeeper.Conn
	zkEventChan <-chan zookeeper.Event
	commandChan chan *command
	data        *topologyData
}

// newTopology connects ZooKeeper, retrieves the data as YAML,
// parses it and stores the data.
func newTopology(zk *zookeeper.Conn) (*topology, error) {
	t := &topology{
		zk:          zk,
		commandChan: make(chan *command),
		data:        newTopologyData(),
	}

	rawData, _, ec, err := zk.GetW("/topology")

	t.zkEventChan = ec

	if err != nil {
		return nil, err
	}

	if err = goyaml.Unmarshal([]byte(rawData), t.data); err != nil {
		return nil, err
	}

	go t.backend()

	return t, nil
}

// execute sends a command function to the backend for execution. 
// This way the execution is serialized.
func (t *topology) execute(cf commandFunc) error {
	cmd := newCommand(cf)

	t.commandChan <- cmd

	return <-cmd.errorChan
}

// backend manages the topology as a goroutine.
func (t *topology) backend() {
	for {
		select {
		case evt := <-t.zkEventChan:
			// Change happened inside the topology.
			if !evt.Ok() {
				// TODO: Error handling, logging!
			}

			switch evt.Type {
			case zookeeper.EVENT_CHANGED:
				// Reload the data and sync it.
				t.sync()
			}
		case cmd := <-t.commandChan:
			// Perform the given command on the
			// topology nodes.
			cmd.execute(t.data)
		}
	}
}

// sync retrieves and parses the topology from ZooKeeper. This
// data is passed to the topology data to synchronize the topology
// entities.
func (t *topology) sync() error {
	rawData, _, err := t.zk.Get("/topology")

	if err != nil {
		// TODO: Error handling, logging!
		return err
	}

	td := newTopologyData()

	if err = goyaml.Unmarshal([]byte(rawData), td); err != nil {
		// TODO: Error handling, logging!
		return err
	}

	return t.data.sync(td)
}
