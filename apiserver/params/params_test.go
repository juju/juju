// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"encoding/json"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

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
	json: `["machine","change",{"Id":"Benji","InstanceId":"Shazam","HasVote":false,"WantsVote":false,"Status":"error","StatusInfo":"foo","StatusData":null,"Life":"alive","Series":"trusty","SupportedContainers":["lxc"],"SupportedContainersKnown":false,"Jobs":["JobManageEnviron"],"Addresses":[],"HardwareCharacteristics":{}}]`,
}, {
	about: "ServiceInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.ServiceInfo{
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
		},
	},
	json: `["service","change",{"CharmURL": "cs:quantal/name","Name":"Benji","Exposed":true,"Life":"dying","OwnerTag":"test-owner","MinUnits":42,"Constraints":{"arch":"armhf", "mem": 1024},"Config": {"hello":"goodbye","foo":false},"Subordinate":false}]`,
}, {
	about: "UnitInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.UnitInfo{
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
		},
	},
	json: `["unit", "change", {"CharmURL": "cs:~user/precise/wordpress-42", "MachineId": "1", "Series": "precise", "Name": "Benji", "PublicAddress": "testing.invalid", "Service": "Shazam", "PrivateAddress": "10.0.0.1", "Ports": [{"Protocol": "http", "Number": 80}], "PortRanges": [{"FromPort": 80, "ToPort": 80, "Protocol": "http"}], "Status": "error", "StatusInfo": "foo", "StatusData": null, "Subordinate": false}]`,
}, {
	about: "RelationInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.RelationInfo{
			Key: "Benji",
			Id:  4711,
			Endpoints: []multiwatcher.Endpoint{
				{ServiceName: "logging", Relation: charm.Relation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 1, Scope: "container"}},
				{ServiceName: "wordpress", Relation: charm.Relation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
		},
	},
	json: `["relation","change",{"Key":"Benji", "Id": 4711, "Endpoints": [{"ServiceName":"logging", "Relation":{"Name":"logging-directory", "Role":"requirer", "Interface":"logging", "Optional":false, "Limit":1, "Scope":"container"}}, {"ServiceName":"wordpress", "Relation":{"Name":"logging-dir", "Role":"provider", "Interface":"logging", "Optional":false, "Limit":0, "Scope":"container"}}]}]`,
}, {
	about: "AnnotationInfo Delta",
	value: multiwatcher.Delta{
		Entity: &multiwatcher.AnnotationInfo{
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
	value: multiwatcher.Delta{
		Removed: true,
		Entity: &multiwatcher.RelationInfo{
			Key: "Benji",
		},
	},
	json: `["relation","remove",{"Key":"Benji", "Id": 0, "Endpoints": null}]`,
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

// TODO(dimitern): apiserver/params should not depend on the network
// package: network types used as fields in request/response structs
// should be replaced with equivalent string, []string, or [][]string
// types, so the wire-format of the API protocol will remain stable,
// even if a network type changes its serialization format.
//
// This test ensures the following network package types used as
// fields are still properly serialized and deserialized:
//
// params.PortsResult.Ports                []params.Port
// params.MachinePortRange.PortRange       network.PortRange
// params.MachineAddresses.Addresses       []network.Address
// params.AddMachineParams.Addrs           []network.Address
// params.RsyslogConfigResult.HostPorts    []params.HostPort
// params.APIHostPortsResult.Servers       [][]params.HostPort
// params.LoginResult.Servers              [][]params.HostPort
// params.LoginResultV1.Servers            [][]params.HostPort
func (s *MarshalSuite) TestNetworkEntities(c *gc.C) {
	setPort := func(addrs []params.HostPort, port int) []params.HostPort {
		hps := make([]params.HostPort, len(addrs))
		for i, addr := range addrs {
			hps[i] = params.HostPort{
				Value:       addr.Value,
				Type:        addr.Type,
				NetworkName: addr.NetworkName,
				Scope:       addr.Scope,
				Port:        port,
			}
		}
		return hps
	}
	allBaseHostPorts := []params.HostPort{
		{Value: "foo0", Type: "bar0", NetworkName: "baz0", Scope: "none0"},
		{Type: "bar1", NetworkName: "baz1", Scope: "none1"},
		{Value: "foo2", NetworkName: "baz2", Scope: "none2"},
		{Value: "foo3", Type: "bar3", Scope: "none3"},
		{Value: "foo4", Type: "bar4", NetworkName: "baz4"},
		{Value: "foo5", Type: "bar5"},
		{Value: "foo6"},
		{},
	}
	allHostPortCombos := setPort(allBaseHostPorts, 1234)
	allHostPortCombos = append(allHostPortCombos, setPort(allBaseHostPorts, 0)...)
	allServerHostPorts := [][]params.HostPort{
		allHostPortCombos,
		setPort(allBaseHostPorts, 0),
		setPort(allBaseHostPorts, 1234),
		{},
	}

	for i, test := range []struct {
		about string
		input interface{}
	}{{
		about: "params.PortResult.Ports",
		input: []params.PortsResult{{
			Ports: []params.Port{
				{Protocol: "foo", Number: 42},
				{Protocol: "bar"},
				{Number: 99},
				{},
			},
		}, {},
		},
	}, {
		about: "params.MachinePortRange.PortRange",
		input: []params.MachinePortRange{{
			UnitTag:     "foo",
			RelationTag: "bar",
			PortRange:   network.PortRange{FromPort: 42, ToPort: 69, Protocol: "baz"},
		}, {
			PortRange: network.PortRange{ToPort: 69, Protocol: "foo"},
		}, {
			PortRange: network.PortRange{FromPort: 42, Protocol: "bar"},
		}, {
			PortRange: network.PortRange{Protocol: "baz"},
		}, {
			PortRange: network.PortRange{FromPort: 42, ToPort: 69},
		}, {
			PortRange: network.PortRange{},
		}, {},
		},
	}, {
		about: "params.MachineAddresses.Addresses",
		input: []params.MachineAddresses{{
			Tag:       "foo",
			Addresses: allBaseHostPorts,
		}, {},
		},
	}, {
		about: "params.AddMachineParams.Addrs",
		input: []params.AddMachineParams{{
			Series:    "foo",
			ParentId:  "bar",
			Placement: nil,
			Addrs:     allBaseHostPorts,
		}, {},
		},
	}, {
		about: "params.RsyslogConfigResult.HostPorts",
		input: []params.RsyslogConfigResult{{
			CACert:    "fake",
			HostPorts: allHostPortCombos,
		}, {},
		},
	}, {
		about: "params.APIHostPortsResult.Servers",
		input: []params.APIHostPortsResult{{
			Servers: allServerHostPorts,
		}, {},
		},
	}, {
		about: "params.LoginResult.Servers",
		input: []params.LoginResult{{
			Servers: allServerHostPorts,
		}, {},
		},
	}, {
		about: "params.LoginResultV1.Servers",
		input: []params.LoginResultV1{{
			Servers: allServerHostPorts,
		}, {},
		},
	}} {
		c.Logf("\ntest %d: %s", i, test.about)
		output, err := json.Marshal(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Logf("\nJSON output:\n%v", string(output))
		c.Assert(string(output), jc.JSONEquals, test.input)
	}
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
