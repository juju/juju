// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

// type RelationSerializationSuite struct {
// 	SliceSerializationSuite
// }

// var _ = gc.Suite(&RelationSerializationSuite{})

// func (s *RelationSerializationSuite) SetUpTest(c *gc.C) {
// 	s.SliceSerializationSuite.SetUpTest(c)
// 	s.importName = "relations"
// 	s.sliceName = "relations"
// 	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
// 		return importRelations(m)
// 	}
// 	s.testFields = func(m map[string]interface{}) {
// 		m["relations"] = []interface{}{}
// 	}
// }

// func minimalRelationMap() map[interface{}]interface{} {
// 	return map[interface{}]interface{}{
// 		"name":            "ubuntu/0",
// 		"machine":         "0",
// 		"agent-status":    minimalStatusMap(),
// 		"workload-status": minimalStatusMap(),
// 		"password-hash":   "secure-hash",
// 		"tools":           minimalAgentToolsMap(),
// 	}
// }

// func minimalRelation() *relation {
// 	u := newRelation(minimalRelationArgs())
// 	u.SetAgentStatus(minimalStatusArgs())
// 	u.SetWorkloadStatus(minimalStatusArgs())
// 	u.SetTools(minimalAgentToolsArgs())
// 	return u
// }

// func minimalRelationArgs() RelationArgs {
// 	return RelationArgs{
// 		Tag:          names.NewRelationTag("ubuntu/0"),
// 		Machine:      names.NewMachineTag("0"),
// 		PasswordHash: "secure-hash",
// 	}
// }

// func (s *RelationSerializationSuite) completeRelation() *relation {
// 	// This relation is about completeness, not reasonableness. That is why the
// 	// relation has a principle (normally only for subordinates), and also a list
// 	// of subordinates.
// 	args := RelationArgs{
// 		Tag:          names.NewRelationTag("ubuntu/0"),
// 		Machine:      names.NewMachineTag("0"),
// 		PasswordHash: "secure-hash",
// 		Principal:    names.NewRelationTag("principal/0"),
// 		Subordinates: []names.RelationTag{
// 			names.NewRelationTag("sub1/0"),
// 			names.NewRelationTag("sub2/0"),
// 		},
// 		MeterStatusCode: "meter code",
// 		MeterStatusInfo: "meter info",
// 	}
// 	relation := newRelation(args)
// 	relation.SetAgentStatus(minimalStatusArgs())
// 	relation.SetWorkloadStatus(minimalStatusArgs())
// 	relation.SetTools(minimalAgentToolsArgs())
// 	return relation
// }

// func (s *RelationSerializationSuite) TestNewRelation(c *gc.C) {
// 	relation := s.completeRelation()

// 	c.Assert(relation.Tag(), gc.Equals, names.NewRelationTag("ubuntu/0"))
// 	c.Assert(relation.Name(), gc.Equals, "ubuntu/0")
// 	c.Assert(relation.Machine(), gc.Equals, names.NewMachineTag("0"))
// 	c.Assert(relation.PasswordHash(), gc.Equals, "secure-hash")
// 	c.Assert(relation.Principal(), gc.Equals, names.NewRelationTag("principal/0"))
// 	c.Assert(relation.Subordinates(), jc.DeepEquals, []names.RelationTag{
// 		names.NewRelationTag("sub1/0"),
// 		names.NewRelationTag("sub2/0"),
// 	})
// 	c.Assert(relation.MeterStatusCode(), gc.Equals, "meter code")
// 	c.Assert(relation.MeterStatusInfo(), gc.Equals, "meter info")
// 	c.Assert(relation.Tools(), gc.NotNil)
// 	c.Assert(relation.WorkloadStatus(), gc.NotNil)
// 	c.Assert(relation.AgentStatus(), gc.NotNil)
// }

// func (s *RelationSerializationSuite) TestMinimalRelationValid(c *gc.C) {
// 	relation := minimalRelation()
// 	c.Assert(relation.Validate(), jc.ErrorIsNil)
// }

// func (s *RelationSerializationSuite) TestMinimalMatches(c *gc.C) {
// 	bytes, err := yaml.Marshal(minimalRelation())
// 	c.Assert(err, jc.ErrorIsNil)

// 	var source map[interface{}]interface{}
// 	err = yaml.Unmarshal(bytes, &source)
// 	c.Assert(err, jc.ErrorIsNil)
// 	c.Assert(source, jc.DeepEquals, minimalRelationMap())
// }

// func (s *RelationSerializationSuite) TestParsingSerializedData(c *gc.C) {
// 	initial := relations{
// 		Version:    1,
// 		Relations_: []*relation{s.completeRelation()},
// 	}

// 	bytes, err := yaml.Marshal(initial)
// 	c.Assert(err, jc.ErrorIsNil)

// 	var source map[string]interface{}
// 	err = yaml.Unmarshal(bytes, &source)
// 	c.Assert(err, jc.ErrorIsNil)

// 	relations, err := importRelations(source)
// 	c.Assert(err, jc.ErrorIsNil)

// 	c.Assert(relations, jc.DeepEquals, initial.Relations_)
// }

type EndPointSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&EndPointSerializationSuite{})

func (s *EndPointSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "endpoints"
	s.sliceName = "endpoints"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importEndPoints(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["endpoints"] = []interface{}{}
	}
}

func minimalEndPointMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"service-name":  "ubuntu",
		"name":          "juju-meta",
		"role":          "peer",
		"interface":     "something",
		"optional":      true,
		"limit":         1,
		"scope":         "container",
		"unit-settings": map[interface{}]interface{}{},
	}
}

func minimalEndPoint() *endpoint {
	return newEndPoint(minimalEndPointArgs())
}

func minimalEndPointArgs() EndPointArgs {
	return EndPointArgs{
		ServiceName: "ubuntu",
		Name:        "juju-meta",
		Role:        "peer",
		Interface:   "something",
		Optional:    true,
		Limit:       1,
		Scope:       "container",
	}
}

func endpointWithSettings() *endpoint {
	endpoint := minimalEndPoint()
	u1Settings := map[string]interface{}{
		"name": "unit one",
		"key":  42,
	}
	u2Settings := map[string]interface{}{
		"name": "unit two",
		"foo":  "bar",
	}
	endpoint.SetUnitSettings("ubuntu/0", u1Settings)
	endpoint.SetUnitSettings("ubuntu/1", u2Settings)
	return endpoint
}

func (s *EndPointSerializationSuite) TestNewEndPoint(c *gc.C) {
	endpoint := endpointWithSettings()

	c.Assert(endpoint.ServiceName(), gc.Equals, "ubuntu")
	c.Assert(endpoint.Name(), gc.Equals, "juju-meta")
	c.Assert(endpoint.Role(), gc.Equals, "peer")
	c.Assert(endpoint.Interface(), gc.Equals, "something")
	c.Assert(endpoint.Optional(), jc.IsTrue)
	c.Assert(endpoint.Limit(), gc.Equals, 1)
	c.Assert(endpoint.Scope(), gc.Equals, "container")
	c.Assert(endpoint.Settings("ubuntu/0"), jc.DeepEquals, map[string]interface{}{
		"name": "unit one",
		"key":  42,
	})
	c.Assert(endpoint.Settings("ubuntu/1"), jc.DeepEquals, map[string]interface{}{
		"name": "unit two",
		"foo":  "bar",
	})
}

func (s *EndPointSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalEndPoint())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalEndPointMap())
}

func (s *EndPointSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := endpoints{
		Version:    1,
		EndPoints_: []*endpoint{endpointWithSettings()},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	endpoints, err := importEndPoints(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(endpoints, jc.DeepEquals, initial.EndPoints_)
}
