// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"text/template"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// AddControllerMachine adds a "controller" machine to the state so
// that State.Addresses and State.APIAddresses will work. It returns the
// added machine. The addresses that those methods will return bear no
// relation to the addresses actually used by the state and API servers.
// It returns the addresses that will be returned by the State.Addresses
// and State.APIAddresses methods, which will not bear any relation to
// the be the addresses used by the controllers.
func AddControllerMachine(c *gc.C, st *state.State) *state.Machine {
	machine, err := st.AddMachine("quantal", state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	hostPorts := [][]network.HostPort{
		network.NewHostPorts(1234, "0.1.2.3"),
	}
	err = st.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	return machine
}

// AddSubnetsWithTemplate adds numSubnets subnets, using the given
// infoTemplate. Any string field in the infoTemplate can be specified
// as a text/template string containing {{.}}, which is the current
// index of the subnet-to-add (between 0 and numSubnets-1).
//
// Example:
//
// AddSubnetsWithTemplate(c, st, 2, state.SubnetInfo{
//     CIDR: "10.10.{{.}}.0/24",
//     ProviderId: "subnet-{{.}}",
//     SpaceName: "space1",
//     AvailabilityZone: "zone-{{.}}",
//     AllocatableIPLow: "{{if (gt . 0)}}10.10.{{.}}.5{{end}}",
//     AllocatableIPHigh: "{{if (gt . 0)}}10.10.{{.}}.254{{end}}",
//     VLANTag: 42,
// })
//
// This is equivalent to the following calls:
//
// _, err := st.AddSubnet(state.SubnetInfo{
//     CIDR: "10.10.0.0/24",
//     ProviderId: "subnet-0",
//     SpaceName: "space1",
//     AvailabilityZone: "zone-0",
//     VLANTag: 42,
// })
// c.Assert(err, jc.ErrorIsNil)
// _, err = st.AddSubnet(state.SubnetInfo{
//     CIDR: "10.10.1.0/24",
//     ProviderId: "subnet-1",
//     SpaceName: "space1",
//     AvailabilityZone: "zone-1",
//     AllocatableIPLow: "10.10.1.5",
//     AllocatableIPHigh: "10.10.1.254",
//     VLANTag: 42,
// })
func AddSubnetsWithTemplate(c *gc.C, st *state.State, numSubnets uint, infoTemplate state.SubnetInfo) {
	for subnetIndex := 0; subnetIndex < int(numSubnets); subnetIndex++ {
		info := infoTemplate // make a copy each time.

		// permute replaces the contents of *s with the result of interpreting
		// *s as a template.
		permute := func(s string) string {
			t, err := template.New("").Parse(s)
			c.Assert(err, jc.ErrorIsNil)

			var buf bytes.Buffer
			err = t.Execute(&buf, subnetIndex)
			c.Assert(err, jc.ErrorIsNil)
			return buf.String()
		}

		info.ProviderId = network.Id(permute(string(info.ProviderId)))
		info.CIDR = permute(info.CIDR)
		info.AllocatableIPHigh = permute(info.AllocatableIPHigh)
		info.AllocatableIPLow = permute(info.AllocatableIPLow)
		info.AvailabilityZone = permute(info.AvailabilityZone)
		info.SpaceName = permute(info.SpaceName)

		_, err := st.AddSubnet(info)
		c.Assert(err, jc.ErrorIsNil)
	}
}
