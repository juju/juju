// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"encoding/json"
	stdtesting "testing"

	"github.com/juju/charm/v7"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
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
	value params.Delta
	// JSON document.
	json string
}{{
	about: "MachineInfo Delta",
	value: params.Delta{
		Entity: &params.MachineInfo{
			ModelUUID:  "uuid",
			Id:         "Benji",
			InstanceId: "Shazam",
			AgentStatus: params.StatusInfo{
				Current: status.Error,
				Message: "foo",
			},
			InstanceStatus: params.StatusInfo{
				Current: status.Pending,
			},
			Life:                    life.Alive,
			Series:                  "trusty",
			SupportedContainers:     []instance.ContainerType{instance.LXD},
			Jobs:                    []model.MachineJob{state.JobManageModel.ToParams()},
			Addresses:               []params.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
		},
	},
	json: `["machine","change",{"model-uuid":"uuid","id":"Benji","instance-id":"Shazam","container-type":"","agent-status":{"current":"error","message":"foo","version":""},"instance-status":{"current":"pending","message":"","version":""},"life":"alive","series":"trusty","supported-containers":["lxd"],"supported-containers-known":false,"hardware-characteristics":{},"jobs":["JobManageModel"],"addresses":[],"has-vote":false,"wants-vote":false}]`,
}, {
	about: "ApplicationInfo Delta",
	value: params.Delta{
		Entity: &params.ApplicationInfo{
			ModelUUID:   "uuid",
			Name:        "Benji",
			Exposed:     true,
			CharmURL:    "cs:quantal/name",
			Life:        life.Dying,
			OwnerTag:    "test-owner",
			MinUnits:    42,
			Constraints: constraints.MustParse("arch=armhf mem=1024M"),
			Config: charm.Settings{
				"hello": "goodbye",
				"foo":   false,
			},
			Status: params.StatusInfo{
				Current: status.Active,
				Message: "all good",
			},
			WorkloadVersion: "42.47",
		},
	},
	json: `["application","change",{"model-uuid": "uuid", "charm-url": "cs:quantal/name","name":"Benji","exposed":true,"life":"dying","owner-tag":"test-owner","workload-version":"42.47","min-units":42,"constraints":{"arch":"armhf", "mem": 1024},"config": {"hello":"goodbye","foo":false},"subordinate":false,"status":{"current":"active", "message":"all good", "version": ""}}]`,
}, {
	about: "CharmInfo Delta",
	value: params.Delta{
		Entity: &params.CharmInfo{
			ModelUUID:    "uuid",
			CharmURL:     "cs:quantal/name",
			Life:         life.Dying,
			CharmVersion: "3",
			LXDProfile:   &params.Profile{},
		},
	},
	json: `["charm","change",{"model-uuid": "uuid", "charm-url": "cs:quantal/name", "charm-version":"3", "life":"dying","profile":{}}]`,
}, {
	about: "UnitInfo Delta",
	value: params.Delta{
		Entity: &params.UnitInfo{
			ModelUUID:   "uuid",
			Name:        "Benji",
			Application: "Shazam",
			Series:      "precise",
			CharmURL:    "cs:~user/precise/wordpress-42",
			Life:        life.Alive,
			Ports: []params.Port{{
				Protocol: "http",
				Number:   80,
			}},
			PortRanges: []params.PortRange{{
				FromPort: 80,
				ToPort:   80,
				Protocol: "http",
			}},
			PublicAddress:  "testing.invalid",
			PrivateAddress: "10.0.0.1",
			MachineId:      "1",
			WorkloadStatus: params.StatusInfo{
				Current: status.Active,
				Message: "all good",
			},
			AgentStatus: params.StatusInfo{
				Current: status.Idle,
			},
		},
	},
	json: `["unit","change",{"model-uuid":"uuid","name":"Benji","application":"Shazam","series":"precise","charm-url":"cs:~user/precise/wordpress-42","life":"alive","public-address":"testing.invalid","private-address":"10.0.0.1","machine-id":"1","principal":"","ports":[{"protocol":"http","number":80}],"port-ranges":[{"from-port":80,"to-port":80,"protocol":"http"}],"subordinate":false,"workload-status":{"current":"active","message":"all good","version":""},"agent-status":{"current":"idle","message":"","version":""}}]`,
}, {
	about: "RelationInfo Delta",
	value: params.Delta{
		Entity: &params.RelationInfo{
			ModelUUID: "uuid",
			Key:       "Benji",
			Id:        4711,
			Endpoints: []params.Endpoint{
				{
					ApplicationName: "logging",
					Relation: params.CharmRelation{
						Name:      "logging-directory",
						Role:      "requirer",
						Interface: "logging",
						Optional:  false,
						Limit:     1,
						Scope:     "container"},
				},
				{
					ApplicationName: "wordpress",
					Relation: params.CharmRelation{
						Name:      "logging-dir",
						Role:      "provider",
						Interface: "logging",
						Optional:  false,
						Limit:     0,
						Scope:     "container"},
				},
			},
		},
	},
	json: `["relation","change",{"model-uuid": "uuid", "key":"Benji", "id": 4711, "endpoints": [{"application-name":"logging", "relation":{"name":"logging-directory", "role":"requirer", "interface":"logging", "optional":false, "limit":1, "scope":"container"}}, {"application-name":"wordpress", "relation":{"name":"logging-dir", "role":"provider", "interface":"logging", "optional":false, "limit":0, "scope":"container"}}]}]`,
}, {
	about: "AnnotationInfo Delta",
	value: params.Delta{
		Entity: &params.AnnotationInfo{
			ModelUUID: "uuid",
			Tag:       "machine-0",
			Annotations: map[string]string{
				"foo":   "bar",
				"arble": "2 4",
			},
		},
	},
	json: `["annotation","change",{"model-uuid": "uuid", "tag":"machine-0","annotations":{"foo":"bar","arble":"2 4"}}]`,
}, {
	about: "Delta Removed True",
	value: params.Delta{
		Removed: true,
		Entity: &params.RelationInfo{
			ModelUUID: "uuid",
			Key:       "Benji",
		},
	},
	json: `["relation","remove",{"model-uuid": "uuid", "key":"Benji", "id": 0, "endpoints": null}]`,
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
		var unmarshalled params.Delta
		err := json.Unmarshal([]byte(t.json), &unmarshalled)
		c.Check(err, jc.ErrorIsNil)
		c.Check(unmarshalled, jc.DeepEquals, t.value)
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
