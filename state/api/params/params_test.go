// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"encoding/json"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type MarshalSuite struct{}

var _ = gc.Suite(&MarshalSuite{})

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
			Status:     "error",
			StatusInfo: "foo",
		},
	},
	json: `["machine","change",{"Id":"Benji","InstanceId":"Shazam","Status":"error","StatusInfo":"foo","StatusData":null}]`,
}, {
	about: "ServiceInfo Delta",
	value: params.Delta{
		Entity: &params.ServiceInfo{
			Name:        "Benji",
			Exposed:     true,
			CharmURL:    "cs:quantal/name",
			Life:        params.Life(state.Dying.String()),
			OwnerTag:    "test-owner",
			MinUnits:    42,
			Constraints: constraints.MustParse("arch=arm mem=1024M"),
			Config: charm.Settings{
				"hello": "goodbye",
				"foo":   false,
			},
		},
	},
	json: `["service","change",{"CharmURL": "cs:quantal/name","Name":"Benji","Exposed":true,"Life":"dying","OwnerTag":"test-owner","MinUnits":42,"Constraints":{"arch":"arm", "mem": 1024},"Config": {"hello":"goodbye","foo":false}}]`,
}, {
	about: "UnitInfo Delta",
	value: params.Delta{
		Entity: &params.UnitInfo{
			Name:     "Benji",
			Service:  "Shazam",
			Series:   "precise",
			CharmURL: "cs:~user/precise/wordpress-42",
			Ports: []instance.Port{
				{
					Protocol: "http",
					Number:   80},
			},
			PublicAddress:  "testing.invalid",
			PrivateAddress: "10.0.0.1",
			MachineId:      "1",
			Status:         "error",
			StatusInfo:     "foo",
		},
	},
	json: `["unit", "change", {"CharmURL": "cs:~user/precise/wordpress-42", "MachineId": "1", "Series": "precise", "Name": "Benji", "PublicAddress": "testing.invalid", "Service": "Shazam", "PrivateAddress": "10.0.0.1", "Ports": [{"Protocol": "http", "Number": 80}], "Status": "error", "StatusInfo": "foo","StatusData":null}]`,
}, {
	about: "RelationInfo Delta",
	value: params.Delta{
		Entity: &params.RelationInfo{
			Key: "Benji",
			Id:  4711,
			Endpoints: []params.Endpoint{
				{ServiceName: "logging", Relation: charm.Relation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 1, Scope: "container"}},
				{ServiceName: "wordpress", Relation: charm.Relation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
		},
	},
	json: `["relation","change",{"Key":"Benji", "Id": 4711, "Endpoints": [{"ServiceName":"logging", "Relation":{"Name":"logging-directory", "Role":"requirer", "Interface":"logging", "Optional":false, "Limit":1, "Scope":"container"}}, {"ServiceName":"wordpress", "Relation":{"Name":"logging-dir", "Role":"provider", "Interface":"logging", "Optional":false, "Limit":0, "Scope":"container"}}]}]`,
}, {
	about: "AnnotationInfo Delta",
	value: params.Delta{
		Entity: &params.AnnotationInfo{
			Tag: "machine-0",
			Annotations: map[string]string{
				"foo":   "bar",
				"arble": "2 4",
			},
		},
	},
	json: `["annotation","change",{"Tag":"machine-0","Annotations":{"foo":"bar","arble":"2 4"}}]`,
}, {
	about: "Delta Removed True",
	value: params.Delta{
		Removed: true,
		Entity: &params.RelationInfo{
			Key: "Benji",
		},
	},
	json: `["relation","remove",{"Key":"Benji", "Id": 0, "Endpoints": null}]`,
}}

func (s *MarshalSuite) TestDeltaMarshalJSON(c *gc.C) {
	for _, t := range marshalTestCases {
		c.Log(t.about)
		output, err := t.value.MarshalJSON()
		c.Check(err, gc.IsNil)
		// We check unmarshalled output both to reduce the fragility of the
		// tests (because ordering in the maps can change) and to verify that
		// the output is well-formed.
		var unmarshalledOutput interface{}
		err = json.Unmarshal(output, &unmarshalledOutput)
		c.Check(err, gc.IsNil)
		var expected interface{}
		err = json.Unmarshal([]byte(t.json), &expected)
		c.Check(err, gc.IsNil)
		c.Check(unmarshalledOutput, gc.DeepEquals, expected)
	}
}

func (s *MarshalSuite) TestDeltaUnmarshalJSON(c *gc.C) {
	for i, t := range marshalTestCases {
		c.Logf("test %d. %s", i, t.about)
		var unmarshalled params.Delta
		err := json.Unmarshal([]byte(t.json), &unmarshalled)
		c.Check(err, gc.IsNil)
		c.Check(unmarshalled, gc.DeepEquals, t.value)
	}
}

func (s *MarshalSuite) TestDeltaMarshalJSONCardinality(c *gc.C) {
	err := json.Unmarshal([]byte(`[1,2]`), new(params.Delta))
	c.Check(err, gc.ErrorMatches, "Expected 3 elements in top-level of JSON but got 2")
}

func (s *MarshalSuite) TestDeltaMarshalJSONUnknownOperation(c *gc.C) {
	err := json.Unmarshal([]byte(`["relation","masticate",{}]`), new(params.Delta))
	c.Check(err, gc.ErrorMatches, `Unexpected operation "masticate"`)
}

func (s *MarshalSuite) TestDeltaMarshalJSONUnknownEntity(c *gc.C) {
	err := json.Unmarshal([]byte(`["qwan","change",{}]`), new(params.Delta))
	c.Check(err, gc.ErrorMatches, `Unexpected entity name "qwan"`)
}
