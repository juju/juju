// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type PayloadSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&PayloadSerializationSuite{})

func (s *PayloadSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "payloads"
	s.sliceName = "payloads"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importPayloads(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["payloads"] = []interface{}{}
	}
}

func allPayloadArgs() PayloadArgs {
	return PayloadArgs{
		Name:   "bob",
		Type:   "docker",
		RawID:  "d06f00d",
		State:  "running",
		Labels: []string{"auto", "foo"},
	}
}

func (s *PayloadSerializationSuite) TestNewPayload(c *gc.C) {
	p := newPayload(allPayloadArgs())
	c.Check(p.Name(), gc.Equals, "bob")
	c.Check(p.Type(), gc.Equals, "docker")
	c.Check(p.RawID(), gc.Equals, "d06f00d")
	c.Check(p.State(), gc.Equals, "running")
	c.Check(p.Labels(), jc.DeepEquals, []string{"auto", "foo"})
}

func (s *PayloadSerializationSuite) exportImport(c *gc.C, p *payload) *payload {
	initial := payloads{
		Version:   1,
		Payloads_: []*payload{p},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	payloads, err := importPayloads(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(payloads, gc.HasLen, 1)
	return payloads[0]
}

func (s *PayloadSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := newPayload(allPayloadArgs())
	imported := s.exportImport(c, initial)
	c.Assert(imported, jc.DeepEquals, initial)
}

func (s *PayloadSerializationSuite) TestImportEmpty(c *gc.C) {
	payloads, err := importPayloads(emptyPayloadMap())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(payloads, gc.HasLen, 0)
}

func emptyPayloadMap() map[string]interface{} {
	return map[string]interface{}{
		"version":  1,
		"payloads": []interface{}{},
	}
}
