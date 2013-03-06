package params_test

import (
	"encoding/json"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/params.api/params"
)

var marshalTestCases = []struct {
	about  string
	input  params.Delta
	output string
}{{
	about: "MachineInfo Delta",
	input: params.Delta{
		Removed: false,
		Entity: &params.MachineInfo{
			Id:         "Benji",
			InstanceId: "Shazam",
		},
	},
	output: `["machine","change",{"Id":"Benji","InstanceId":"Shazam"}]`,
}, {
	about: "ServiceInfo Delta",
	input: params.Delta{
		Removed: false,
		Entity: &params.ServiceInfo{
			Name:    "Benji",
			Exposed: true,
		},
	},
	output: `["service","change",{"Name":"Benji","Exposed":true}]`,
}, {
	about: "UnitInfo Delta",
	input: params.Delta{
		Removed: false,
		Entity: &params.UnitInfo{
			Name:    "Benji",
			Service: "Shazam",
		},
	},
	output: `["unit","change",{"Name":"Benji","Service":"Shazam"}]`,
}, {
	about: "RelationInfo Delta",
	input: params.Delta{
		Removed: false,
		Entity: &params.RelationInfo{
			Key: "Benji",
		},
	},
	output: `["relation","change",{"Key":"Benji"}]`,
}, {
	about: "Delta Removed True",
	input: params.Delta{
		Removed: true,
		Entity: &params.RelationInfo{
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
