package params_test

import (
	"encoding/json"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/api/params"
	"testing"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *testing.T) {
	TestingT(t)
}

type MarshalSuite struct{}

var _ = Suite(&MarshalSuite{})

var marshalTestCases = []struct {
	about string
	// Value holds a real Go struct.
	value params.Delta
	// JSON document.
	json string
}{{
	about: "MachineInfo Delta",
	value: params.Delta{
		Entity: &params.MachineInfo{
			Id:         "Benji",
			InstanceId: "Shazam",
		},
	},
	json: `["machine","change",{"Id":"Benji","InstanceId":"Shazam"}]`,
}, {
	about: "ServiceInfo Delta",
	value: params.Delta{
		Entity: &params.ServiceInfo{
			Name:    "Benji",
			Exposed: true,
		},
	},
	json: `["service","change",{"Name":"Benji","Exposed":true}]`,
}, {
	about: "UnitInfo Delta",
	value: params.Delta{
		Entity: &params.UnitInfo{
			Name:    "Benji",
			Service: "Shazam",
		},
	},
	json: `["unit","change",{"Name":"Benji","Service":"Shazam"}]`,
}, {
	about: "RelationInfo Delta",
	value: params.Delta{
		Entity: &params.RelationInfo{
			Key: "Benji",
		},
	},
	json: `["relation","change",{"Key":"Benji"}]`,
}, {
	about: "AnnotationInfo Delta",
	value: params.Delta{
		Entity: &params.AnnotationInfo{
			EntityName: "machine-0",
			Annotations: map[string]string{
				"foo":   "bar",
				"arble": "2 4",
			},
		},
	},
	json: `["annotation","change",{"EntityName":"machine-0","Annotations":{"foo":"bar","arble":"2 4"}}]`,
}, {
	about: "Delta Removed True",
	value: params.Delta{
		Removed: true,
		Entity: &params.RelationInfo{
			Key: "Benji",
		},
	},
	json: `["relation","remove",{"Key":"Benji"}]`,
}}

func (s *MarshalSuite) TestDeltaMarshalJSON(c *C) {
	for _, t := range marshalTestCases {
		c.Log(t.about)
		output, err := t.value.MarshalJSON()
		c.Check(err, IsNil)
		// We check unmarshalled output both to reduce the fragility of the
		// tests (because ordering in the maps can change) and to verify that
		// the output is well-formed.
		var unmarshalledOutput interface{}
		err = json.Unmarshal(output, &unmarshalledOutput)
		c.Check(err, IsNil)
		var expected interface{}
		err = json.Unmarshal([]byte(t.json), &expected)
		c.Check(err, IsNil)
		c.Check(unmarshalledOutput, DeepEquals, expected)
	}
}

func (s *MarshalSuite) TestDeltaUnmarshalJSON(c *C) {
	for _, t := range marshalTestCases {
		c.Log(t.about)
		var unmarshalled params.Delta
		err := json.Unmarshal([]byte(t.json), &unmarshalled)
		c.Check(err, IsNil)
		c.Check(unmarshalled, DeepEquals, t.value)
	}
}

func (s *MarshalSuite) TestDeltaMarshalJSONCardinality(c *C) {
	err := json.Unmarshal([]byte(`[1,2]`), new(params.Delta))
	c.Check(err, ErrorMatches, "Expected 3 elements in top-level of JSON but got 2")
}

func (s *MarshalSuite) TestDeltaMarshalJSONUnknownOperation(c *C) {
	err := json.Unmarshal([]byte(`["relation","masticate",{}]`), new(params.Delta))
	c.Check(err, ErrorMatches, `Unexpected operation "masticate"`)
}

func (s *MarshalSuite) TestDeltaMarshalJSONUnknownEntity(c *C) {
	err := json.Unmarshal([]byte(`["qwan","change",{}]`), new(params.Delta))
	c.Check(err, ErrorMatches, `Unexpected entity name "qwan"`)
}
