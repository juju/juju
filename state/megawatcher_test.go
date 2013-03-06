package state_test

import (
	"encoding/json"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

var marshalTestCases = []struct {
	about  string
	input  state.Delta
	output string
}{{
	about: "MachineInfo Delta",
	input: state.Delta{
		Removed: false,
		Entity: &state.MachineInfo{
			Id:         "Benji",
			InstanceId: "Shazam",
		},
	},
	output: `["machine","change",{"Id":"Benji","InstanceId":"Shazam"}]`,
}, {
	about: "ServiceInfo Delta",
	input: state.Delta{
		Removed: false,
		Entity: &state.ServiceInfo{
			Name:    "Benji",
			Exposed: true,
		},
	},
	output: `["service","change",{"Name":"Benji","Exposed":true}]`,
}, {
	about: "UnitInfo Delta",
	input: state.Delta{
		Removed: false,
		Entity: &state.UnitInfo{
			Name:    "Benji",
			Service: "Shazam",
		},
	},
	output: `["unit","change",{"Name":"Benji","Service":"Shazam"}]`,
}, {
	about: "RelationInfo Delta",
	input: state.Delta{
		Removed: false,
		Entity: &state.RelationInfo{
			Key: "Benji",
		},
	},
	output: `["relation","change",{"Key":"Benji"}]`,
}, {
	about: "Delta Removed True",
	input: state.Delta{
		Removed: true,
		Entity: &state.RelationInfo{
			Key: "Benji",
		},
	},
	output: `["relation","remove",{"Key":"Benji"}]`,
},
}

type MarshalSuite struct{}

var _ = Suite(&MarshalSuite{})

func (s *MarshalSuite) TestDeltaMarshalJSON(c *C) {
	for _, t := range marshalTestCases {
		c.Log(t.about)
		output, err := t.input.MarshalJSON()
		c.Check(err, IsNil)
		// We check unmarshalled output both to reduce the fragility of the
		// tests (because ordering in the maps can change) and to verify that
		// the output is well-formed.
		var unmarshalledOutput interface{}
		err = json.Unmarshal(output, &unmarshalledOutput)
		c.Check(err, IsNil)
		var expected interface{}
		err = json.Unmarshal([]byte(t.output), &expected)
		c.Check(err, IsNil)
		c.Check(unmarshalledOutput, DeepEquals, expected)
	}
}
