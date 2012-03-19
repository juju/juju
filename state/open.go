package state

import (
	"errors"
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/log"
	"strings"
	"time"
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
// info, and returns a new State
// representing the environment connected to.
// It is an error if the environment has not been initialized.
func Open(info *Info) (*State, error) {
	log.Printf("state: Open %v", info)
	st, err := open(info)
	if err != nil {
		return nil, err
	}
	doneInit, err := st.initialized()
	if err != nil {
		st.Close()
		return nil, err
	}
	if doneInit {
		return st, nil
	}
	st.Close()
	return nil, fmt.Errorf("state: not initialized")
}

// WaitOpen is like Open but will wait for the environment to be initialized
// in necessary. It will not wait longer than the given duration.
func WaitOpen(info *Info, timeout time.Duration) (*State, error) {
	log.Printf("state: WaitOpen %v (timeout %v)", info, timeout)
	st, err := open(info)
	if err != nil {
		return nil, err
	}
	doneInit, err := st.initialized()
	if err != nil {
		st.Close()
		return nil, err
	}
	if doneInit {
		return st, nil
	}

	log.Printf("state: waiting for initialization")
	err = st.waitForInitialization(timeout)
	if err != nil {
		return nil, err
	}
	return st, err
}

// Initialize performs an initialization of the ZooKeeper nodes
// described by the given Info and returns  a new State representing
// the environment connected to.
func Initialize(info *Info) (*State, error) {
	st, err := open(info)
	if err != nil {
		return nil, err
	}
	st.initialize()
	return st, nil
}

func open(info *Info) (*State, error) {
	if len(info.Addrs) == 0 {
		return nil, fmt.Errorf("no zookeeper addresses")
	}
	zk, session, err := zookeeper.Dial(strings.Join(info.Addrs, ","), zkTimeout)
	if err != nil {
		return nil, err
	}
	if !(<-session).Ok() {
		return nil, errors.New("Could not connect to zookeeper")
	}

	// TODO decide what to do with session events - currently
	// we will panic if the session event channel fills up.
	return &State{zk}, nil
}

func (st *State) Close() error {
	return st.zk.Close()
}
