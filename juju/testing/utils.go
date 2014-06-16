// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// AddStateServerMachine adds a "state server" machine to the state so
// that State.Addresses and State.APIAddresses will work. It returns the
// added machine. The addresses that those methods will return bear no
// relation to the addresses actually used by the state and API servers.
// It returns the addresses that will be returned by the State.Addresses
// and State.APIAddresses methods, which will not bear any relation to
// the be the addresses used by the state servers.
func AddStateServerMachine(c *gc.C, st *state.State) *state.Machine {
	machine, err := st.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	err = machine.SetAddresses(network.NewAddress("0.1.2.3", network.ScopeUnknown))
	c.Assert(err, gc.IsNil)

	hostPorts := [][]network.HostPort{{{
		Address: network.NewAddress("0.1.2.3", network.ScopeUnknown),
		Port:    1234,
	}}}
	err = st.SetAPIHostPorts(hostPorts)
	c.Assert(err, gc.IsNil)

	return machine
}
