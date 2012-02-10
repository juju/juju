package state

import (
	"errors"
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"strings"
)

// Info encapsulates information about a juju state server
// or servers and can be used to make a connection to
// that state.
type Info struct {
	// Addrs gives the addresses of the zookeeper
	// servers for the state. Each address should be in the form
	// expected by net.Dial (i.e. address:port).
	Addrs []string
}

// Open connects to the server described by the given
// info and returns a new State representing the environment
// connected to.
func Open(info *Info) (*State, error) {
	if len(info.Addrs) == 0 {
		return nil, fmt.Errorf("no zookeeper addresses")
	}
	addrs := strings.Join(info.Addrs, ",")
	zk, session, err := zookeeper.Dial(addrs, 5e9)
	if err != nil {
		return nil, fmt.Errorf("zookeeper dial %q failed: %v", addrs, err)
	}
	if !(<-session).Ok() {
		return nil, errors.New("Could not connect to zookeeper")
	}
	return &State{zk}, nil
}
