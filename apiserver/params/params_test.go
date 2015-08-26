// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"encoding/json"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type MarshalSuite struct{}

var _ = gc.Suite(&MarshalSuite{})

var marshalTestCases = []struct {
	about string
	// Value holds a real Go struct.
	value multiwatcher.Delta
	// JSON document.
	json string
}{{
	about: "MachineInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:                 "uuid",
			Id:                      "Benji",
			InstanceId:              "Shazam",
			Status:                  "error",
			StatusInfo:              "foo",
			Life:                    multiwatcher.Life("alive"),
			Series:                  "trusty",
			SupportedContainers:     []instance.ContainerType{instance.LXC},
			Jobs:                    []multiwatcher.MachineJob{state.JobManageEnviron.ToParams()},
			Addresses:               []network.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
		},
	},
	json: `["machine","change",{"EnvUUID": "uuid", "Id":"Benji","InstanceId":"Shazam","HasVote":false,"WantsVote":false,"Status":"error","StatusInfo":"foo","StatusData":null,"Life":"alive","Series":"trusty","SupportedContainers":["lxc"],"SupportedContainersKnown":false,"Jobs":["JobManageEnviron"],"Addresses":[],"HardwareCharacteristics":{}}]`,
}, {
	about: "ServiceInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.ServiceInfo{
			EnvUUID:     "uuid",
			Name:        "Benji",
			Exposed:     true,
			CharmURL:    "cs:quantal/name",
			Life:        multiwatcher.Life("dying"),
			OwnerTag:    "test-owner",
			MinUnits:    42,
			Constraints: constraints.MustParse("arch=armhf mem=1024M"),
			Config: charm.Settings{
				"hello": "goodbye",
				"foo":   false,
			},
			Status: multiwatcher.StatusInfo{
				Current: multiwatcher.Status("active"),
				Message: "all good",
			},
		},
	},
	json: `["service","change",{"EnvUUID": "uuid", "CharmURL": "cs:quantal/name","Name":"Benji","Exposed":true,"Life":"dying","OwnerTag":"test-owner","MinUnits":42,"Constraints":{"arch":"armhf", "mem": 1024},"Config": {"hello":"goodbye","foo":false},"Subordinate":false,"Status":{"Current":"active", "Message":"all good", "Version": "", "Err": null, "Data": null, "Since": null}}]`,
}, {
	about: "UnitInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.UnitInfo{
			EnvUUID:  "uuid",
			Name:     "Benji",
			Service:  "Shazam",
			Series:   "precise",
			CharmURL: "cs:~user/precise/wordpress-42",
			Ports: []network.Port{{
				Protocol: "http",
				Number:   80,
			}},
			PortRanges: []network.PortRange{{
				FromPort: 80,
				ToPort:   80,
				Protocol: "http",
			}},
			PublicAddress:  "testing.invalid",
			PrivateAddress: "10.0.0.1",
			MachineId:      "1",
			Status:         "error",
			StatusInfo:     "foo",
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: multiwatcher.Status("active"),
				Message: "all good",
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: multiwatcher.Status("idle"),
			},
		},
	},
	json: `["unit", "change", {"EnvUUID": "uuid", "CharmURL": "cs:~user/precise/wordpress-42", "MachineId": "1", "Series": "precise", "Name": "Benji", "PublicAddress": "testing.invalid", "Service": "Shazam", "PrivateAddress": "10.0.0.1", "Ports": [{"Protocol": "http", "Number": 80}], "PortRanges": [{"FromPort": 80, "ToPort": 80, "Protocol": "http"}], "Status": "error", "StatusInfo": "foo", "StatusData": null, "WorkloadStatus":{"Current":"active", "Message":"all good", "Version": "", "Err": null, "Data": null, "Since": null}, "AgentStatus":{"Current":"idle", "Message":"", "Version": "", "Err": null, "Data": null, "Since": null}, "Subordinate": false}]`,
}, {
	about: "RelationInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.RelationInfo{
			EnvUUID: "uuid",
			Key:     "Benji",
			Id:      4711,
			Endpoints: []multiwatcher.Endpoint{
				{ServiceName: "logging", Relation: charm.Relation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 1, Scope: "container"}},
				{ServiceName: "wordpress", Relation: charm.Relation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
		},
	},
	json: `["relation","change",{"EnvUUID": "uuid", "Key":"Benji", "Id": 4711, "Endpoints": [{"ServiceName":"logging", "Relation":{"Name":"logging-directory", "Role":"requirer", "Interface":"logging", "Optional":false, "Limit":1, "Scope":"container"}}, {"ServiceName":"wordpress", "Relation":{"Name":"logging-dir", "Role":"provider", "Interface":"logging", "Optional":false, "Limit":0, "Scope":"container"}}]}]`,
}, {
	about: "AnnotationInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.AnnotationInfo{
			EnvUUID: "uuid",
			Tag:     "machine-0",
			Annotations: map[string]string{
				"foo":   "bar",
				"arble": "2 4",
			},
		},
	},
	json: `["annotation","change",{"EnvUUID": "uuid", "Tag":"machine-0","Annotations":{"foo":"bar","arble":"2 4"}}]`,
}, {
	about: "Delta Removed True",
	value: multiwatcher.Delta{
		Removed: true,
		Entity: &multiwatcher.RelationInfo{
			EnvUUID: "uuid",
			Key:     "Benji",
		},
	},
	json: `["relation","remove",{"EnvUUID": "uuid", "Key":"Benji", "Id": 0, "Endpoints": null}]`,
}}

func (s *MarshalSuite) TestDeltaMarshalJSON(c *gc.C) {
	for _, t := range marshalTestCases {
		c.Log(t.about)
		output, err := t.value.MarshalJSON()
		c.Check(err, jc.ErrorIsNil)
		// We check unmarshalled output both to reduce the fragility of the
		// tests (because ordering in the maps can change) and to verify that
		// the output is well-formed.
		var unmarshalledOutput interface{}
		err = json.Unmarshal(output, &unmarshalledOutput)
		c.Check(err, jc.ErrorIsNil)
		var expected interface{}
		err = json.Unmarshal([]byte(t.json), &expected)
		c.Check(err, jc.ErrorIsNil)
		c.Check(unmarshalledOutput, jc.DeepEquals, expected)
	}
}

func (s *MarshalSuite) TestDeltaUnmarshalJSON(c *gc.C) {
	for i, t := range marshalTestCases {
		c.Logf("test %d. %s", i, t.about)
		var unmarshalled multiwatcher.Delta
		err := json.Unmarshal([]byte(t.json), &unmarshalled)
		c.Check(err, jc.ErrorIsNil)
		c.Check(unmarshalled, gc.DeepEquals, t.value)
	}
}

func (s *MarshalSuite) TestDeltaMarshalJSONCardinality(c *gc.C) {
	err := json.Unmarshal([]byte(`[1,2]`), new(multiwatcher.Delta))
	c.Check(err, gc.ErrorMatches, "Expected 3 elements in top-level of JSON but got 2")
}

func (s *MarshalSuite) TestDeltaMarshalJSONUnknownOperation(c *gc.C) {
	err := json.Unmarshal([]byte(`["relation","masticate",{}]`), new(multiwatcher.Delta))
	c.Check(err, gc.ErrorMatches, `Unexpected operation "masticate"`)
}

func (s *MarshalSuite) TestDeltaMarshalJSONUnknownEntity(c *gc.C) {
	err := json.Unmarshal([]byte(`["qwan","change",{}]`), new(multiwatcher.Delta))
	c.Check(err, gc.ErrorMatches, `Unexpected entity name "qwan"`)
}

type ErrorResultsSuite struct{}

var _ = gc.Suite(&ErrorResultsSuite{})

func (s *ErrorResultsSuite) TestOneError(c *gc.C) {
	for i, test := range []struct {
		results  params.ErrorResults
		errMatch string
	}{
		{
			errMatch: "expected 1 result, got 0",
		}, {
			results: params.ErrorResults{
				[]params.ErrorResult{{nil}},
			},
		}, {
			results: params.ErrorResults{
				[]params.ErrorResult{{nil}, {nil}},
			},
			errMatch: "expected 1 result, got 2",
		}, {
			results: params.ErrorResults{
				[]params.ErrorResult{
					{&params.Error{Message: "test error"}},
				},
			},
			errMatch: "test error",
		},
	} {
		c.Logf("test %d", i)
		err := test.results.OneError()
		if test.errMatch == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *ErrorResultsSuite) TestCombine(c *gc.C) {
	for i, test := range []struct {
		msg      string
		results  params.ErrorResults
		errMatch string
	}{
		{
			msg: "no results, no error",
		}, {
			msg: "single nil result",
			results: params.ErrorResults{
				[]params.ErrorResult{{nil}},
			},
		}, {
			msg: "multiple nil results",
			results: params.ErrorResults{
				[]params.ErrorResult{{nil}, {nil}},
			},
		}, {
			msg: "one error result",
			results: params.ErrorResults{
				[]params.ErrorResult{
					{&params.Error{Message: "test error"}},
				},
			},
			errMatch: "test error",
		}, {
			msg: "mixed error results",
			results: params.ErrorResults{
				[]params.ErrorResult{
					{&params.Error{Message: "test error"}},
					{nil},
					{&params.Error{Message: "second error"}},
				},
			},
			errMatch: "test error\nsecond error",
		},
	} {
		c.Logf("test %d: %s", i, test.msg)
		err := test.results.Combine()
		if test.errMatch == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

type importSuite struct{}

var _ = gc.Suite(&importSuite{})

func (*importSuite) TestParamsDoesNotDependOnState(c *gc.C) {
	imports := testing.FindJujuCoreImports(c, "github.com/juju/juju/apiserver/params")
	for _, i := range imports {
		c.Assert(i, gc.Not(gc.Equals), "state")
	}
}
