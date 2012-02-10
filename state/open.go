package state

import (
	"errors"
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"strings"
)

// Info encapsulates information about cluster of
// servers holding juju state and can be used to make a
// connection to that cluster.
type Info struct {
	// Addrs gives the addresses of the Zookeeper
	// servers for the state. Each address should be in the form
	// address:port.
	Addrs []string
}

const zkTimeout = 15e9

// Open connects to the server described by the given
// info and returns a new State representing the environment
// connected to.
func Open(info *Info) (*State, error) {
	if len(info.Addrs) == 0 {
		return nil, fmt.Errorf("no zookeeper addresses")
	}
	addrs := strings.Join(info.Addrs, ",")
	zk, session, err := zookeeper.Dial(addrs, zkTimeout)
	if err != nil {
		return nil, fmt.Errorf("zookeeper dial %q failed: %v", addrs, err)
	}
	if !(<-session).Ok() {
		return nil, errors.New("Could not connect to zookeeper")
	}

	// TODO decide what to do with session events - currently
	// we will panic if the session event channel fills up.
	return &State{zk}, nil
}
