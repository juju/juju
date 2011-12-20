// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

// --------------------
// IMPORT
// --------------------

import (
	"errors"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"strings"
)

// --------------------
// CONST
// --------------------

const (
	Version = 1
)

// --------------------
// GLOBAL VARIABLES
// --------------------

// systemTopology is the global topology that will be
// initialized with the first call of retrieveTopology().
// Then it will be reused and updated automatically after
// signals via the watch.
var systemTopology *topology

// --------------------
// TOPOLOGY NODES
// --------------------

// topologyNodes represents a set of nodes of the
// topology informations.
type topologyNodes map[interface{}]interface{}

// newTopologyNodes creates node set with the version set.
func newTopologyNodes() topologyNodes {
	tn := make(topologyNodes)

	tn["version"] = Version

	return tn
}

// find looks for a value (a topologyNode or its end value) by
// a given path. This path is a slice of strings, the end criteria
// for this recursive call is an empty path. Here the node will
// be returned.
func (tn topologyNodes) find(path []string) (interface{}, error) {
	if len(path) == 0 {
		return tn, nil
	}

	value, ok := tn[path[0]]

	if !ok {
		// Not found!
		return nil, errors.New("topology nodes: node '" + path[0] + "' not found")
	}

	if v, ok := value.(map[interface{}]interface{}); ok {
		// More nodes.
		vtn := topologyNodes(v)

		return vtn.find(path[1:])
	}

	return value, nil
}

// getString retrieves the string value of a path. If it's a node
// an error will be returned. The path will be passed as a string
// with slashes as separators.
func (tn topologyNodes) getString(path string) (string, error) {
	value, err := tn.find(pathToSlice(path))

	if err != nil {
		return "", err
	}

	if v, ok := value.(string); ok {
		// It's a string, yeah.
		return v, nil
	}

	return "", errors.New("topology nodes: path '" + path + "' leads to topology nodes, no string")
}

// getNodes retrieves the nodes value of a path. If it's a string
// an error will be returned. The path will be passed as a string
// with slashes as separators.
func (tn topologyNodes) getNodes(path string) (topologyNodes, error) {
	value, err := tn.find(pathToSlice(path))

	if err != nil {
		return nil, err
	}

	if v, ok := value.(topologyNodes); ok {
		// It's a topologyNodes, got it.
		return v, nil
	}

	return nil, errors.New("topology nodes: path '" + path + "' leads to a string, no topology nodes")
}

// searchFunc defines the signature of a function for searches inside the topology nodes.
// The arguments are the current path and the value. It has to return true if the search
// matches.
type searchFunc func(path []string, value interface{}) bool

// search executes a search function recursively on the topology nodes. If this
// function returns true the full path and its value will be returned.
func (tn topologyNodes) search(sf searchFunc) ([]string, interface{}, error) {
	path, value := tn.pathSearch([]string{}, sf)

	if len(path) == 0 {
		// Nothing found!
		return nil, nil, errors.New("topology nodes: search has no results")
	}

	// Success, yay!
	return path, value, nil
}

// pathSearch is used by search and has the current path as argument.
func (tn topologyNodes) pathSearch(path []string, sf searchFunc) ([]string, interface{}) {
	for key, value := range tn {
		p := append(path, key.(string))

		if sf(p, value) {
			// Found it.
			return p, value
		}

		if v, ok := value.(map[interface{}]interface{}); ok {
			// Search not yet ok, but value is a topology node.
			vtn := topologyNodes(v)
			dp, dv := vtn.pathSearch(p, sf)

			if len(dp) > 0 {
				return dp, dv
			}
		}
	}

	// Found nothing.
	return []string{}, nil
}

// pathToSlice converts a path string into a slice of strings.
func pathToSlice(path string) []string {
	pathSlice := strings.Split(path, "/")
	cleanPathSlice := []string{}

	for _, ps := range pathSlice {
		if len(ps) > 0 {
			cleanPathSlice = append(cleanPathSlice, ps)
		}
	}

	return cleanPathSlice
}

// --------------------
// COMMAND
// --------------------

// commandFunc is a function that will be sent to the backend
// to analyze the topology nodes. It can return any result.
type commandFunc func(topologyNodes) (interface{}, error)

// commandResult encapsulates the result and an error for
// transportation over a channel.
type commandResult struct {
	result interface{}
	err    error
}

// command will be sent to the backend goroutine to perform
// the task and return the answer.
type command struct {
	task       commandFunc
	resultChan chan *commandResult
}

// newCommand creates a new command with the given task
// and a new channel for the result.
func newCommand(cf commandFunc) *command {
	return &command{cf, make(chan *commandResult)}
}

// perform performs the task and returns the result
// via the channel.
func (c *command) perform(tn topologyNodes) {
	result, err := c.task(tn)

	c.resultChan <- &commandResult{result, err}
}

// --------------------
// TOPOLOGY
// --------------------

// topology is an internal helper handling the topology informations
// inside of ZooKeeper.
type topology struct {
	zkConn      *zookeeper.Conn
	zkEventChan <-chan zookeeper.Event
	commandChan chan *command
	nodes       topologyNodes
}

// retrieveTopology connects ZooKeeper, retrieves the data as YAML,
// parses and stores it.
func retrieveTopology(zkc *zookeeper.Conn) (*topology, error) {
	if systemTopology == nil {
		systemTopology = &topology{
			zkConn:      zkc,
			commandChan: make(chan *command),
			nodes:       newTopologyNodes(),
		}

		data, _, session, err := zkc.GetW("/topology")

		if err != nil {
			return nil, err
		}

		if err = goyaml.Unmarshal([]byte(data), systemTopology.nodes); err != nil {
			return nil, err
		}

		systemTopology.zkEventChan = session

		go systemTopology.backend()
	}

	return systemTopology, nil
}

// getString retrieves the string value of a path. If it's a node
// an error will be returned. The path will be passed as a string
// with slashes as separators.
func (t *topology) getString(path string) (string, error) {
	value, err := t.execute(func(tn topologyNodes) (interface{}, error) {
		return tn.getString(path)
	})

	return value.(string), err
}

// execute sends a command function to the backend for execution. It returns 
// the result received from the backend. This way all requests are searialized.
func (t *topology) execute(cf commandFunc) (interface{}, error) {
	cmd := newCommand(cf)

	t.commandChan <- cmd

	result := <-cmd.resultChan

	return result.result, result.err
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
				t.reload()
			}
		case cmd := <-t.commandChan:
			// Perform the given command on the
			// topology nodes.
			cmd.perform(t.nodes)
		}
	}
}

// reload retrieves and parses the topology from ZooKeeper.
func (t *topology) reload() error {
	data, _, err := t.zkConn.Get("/topology")

	if err != nil {
		// TODO: Error handling, logging!
		return err
	}

	if err = goyaml.Unmarshal([]byte(data), t.nodes); err != nil {
		// TODO: Error handling, logging!
		return err
	}

	return nil
}

// reset empties the topology.
func (t *topology) reset() {
	t.nodes = newTopologyNodes()
}

// dump returns the current topology as byte slice
func (t *topology) dump() ([]byte, error) {
	return goyaml.Marshal(t.nodes)
}

// EOF
