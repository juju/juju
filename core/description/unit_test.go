// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"
)

type UnitSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&UnitSerializationSuite{})

func (s *UnitSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "units"
	s.sliceName = "units"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importUnits(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["units"] = []interface{}{}
	}
}

func minimalUnitMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"name":                     "ubuntu/0",
		"machine":                  "0",
		"agent-status":             minimalStatusMap(),
		"agent-status-history":     emptyStatusHistoryMap(),
		"workload-status":          minimalStatusMap(),
		"workload-status-history":  emptyStatusHistoryMap(),
		"workload-version-history": emptyStatusHistoryMap(),
		"password-hash":            "secure-hash",
		"tools":                    minimalAgentToolsMap(),
		"payloads": map[interface{}]interface{}{
			"version":  1,
			"payloads": []interface{}{},
		},
	}
}

func minimalUnit() *unit {
	u := newUnit(minimalUnitArgs())
	u.SetAgentStatus(minimalStatusArgs())
	u.SetWorkloadStatus(minimalStatusArgs())
	u.SetTools(minimalAgentToolsArgs())
	return u
}

func minimalUnitArgs() UnitArgs {
	return UnitArgs{
		Tag:          names.NewUnitTag("ubuntu/0"),
		Machine:      names.NewMachineTag("0"),
		PasswordHash: "secure-hash",
	}
}

func (s *UnitSerializationSuite) completeUnit() *unit {
	// This unit is about completeness, not reasonableness. That is why the
	// unit has a principal (normally only for subordinates), and also a list
	// of subordinates.
	args := UnitArgs{
		Tag:          names.NewUnitTag("ubuntu/0"),
		Machine:      names.NewMachineTag("0"),
		PasswordHash: "secure-hash",
		Principal:    names.NewUnitTag("principal/0"),
		Subordinates: []names.UnitTag{
			names.NewUnitTag("sub1/0"),
			names.NewUnitTag("sub2/0"),
		},
		WorkloadVersion: "malachite",
		MeterStatusCode: "meter code",
		MeterStatusInfo: "meter info",
	}
	unit := newUnit(args)
	unit.SetAgentStatus(minimalStatusArgs())
	unit.SetWorkloadStatus(minimalStatusArgs())
	unit.SetTools(minimalAgentToolsArgs())
	return unit
}

func (s *UnitSerializationSuite) TestNewUnit(c *gc.C) {
	unit := s.completeUnit()

	c.Assert(unit.Tag(), gc.Equals, names.NewUnitTag("ubuntu/0"))
	c.Assert(unit.Name(), gc.Equals, "ubuntu/0")
	c.Assert(unit.Machine(), gc.Equals, names.NewMachineTag("0"))
	c.Assert(unit.PasswordHash(), gc.Equals, "secure-hash")
	c.Assert(unit.Principal(), gc.Equals, names.NewUnitTag("principal/0"))
	c.Assert(unit.Subordinates(), jc.DeepEquals, []names.UnitTag{
		names.NewUnitTag("sub1/0"),
		names.NewUnitTag("sub2/0"),
	})
	c.Assert(unit.WorkloadVersion(), gc.Equals, "malachite")
	c.Assert(unit.MeterStatusCode(), gc.Equals, "meter code")
	c.Assert(unit.MeterStatusInfo(), gc.Equals, "meter info")
	c.Assert(unit.Tools(), gc.NotNil)
	c.Assert(unit.WorkloadStatus(), gc.NotNil)
	c.Assert(unit.AgentStatus(), gc.NotNil)
}

func (s *UnitSerializationSuite) TestMinimalUnitValid(c *gc.C) {
	unit := minimalUnit()
	c.Assert(unit.Validate(), jc.ErrorIsNil)
}

func (s *UnitSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalUnit())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalUnitMap())
}

func (s *UnitSerializationSuite) exportImport(c *gc.C, unit_ *unit) *unit {
	initial := units{
		Version: 1,
		Units_:  []*unit{unit_},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	units, err := importUnits(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	return units[0]
}

func (s *UnitSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := s.completeUnit()
	unit := s.exportImport(c, initial)
	c.Assert(unit, jc.DeepEquals, initial)
}

func (s *UnitSerializationSuite) TestAnnotations(c *gc.C) {
	initial := minimalUnit()
	annotations := map[string]string{
		"string":  "value",
		"another": "one",
	}
	initial.SetAnnotations(annotations)

	unit := s.exportImport(c, initial)
	c.Assert(unit.Annotations(), jc.DeepEquals, annotations)
}

func (s *UnitSerializationSuite) TestConstraints(c *gc.C) {
	initial := minimalUnit()
	args := ConstraintsArgs{
		Architecture: "amd64",
		Memory:       8 * gig,
		RootDisk:     40 * gig,
	}
	initial.SetConstraints(args)

	unit := s.exportImport(c, initial)
	c.Assert(unit.Constraints(), jc.DeepEquals, newConstraints(args))
}

func (s *UnitSerializationSuite) TestAgentStatusHistory(c *gc.C) {
	initial := minimalUnit()
	args := testStatusHistoryArgs()
	initial.SetAgentStatusHistory(args)

	unit := s.exportImport(c, initial)
	for i, point := range unit.AgentStatusHistory() {
		c.Check(point.Value(), gc.Equals, args[i].Value)
		c.Check(point.Message(), gc.Equals, args[i].Message)
		c.Check(point.Data(), jc.DeepEquals, args[i].Data)
		c.Check(point.Updated(), gc.Equals, args[i].Updated)
	}
}

func (s *UnitSerializationSuite) TestWorkloadStatusHistory(c *gc.C) {
	initial := minimalUnit()
	args := testStatusHistoryArgs()
	initial.SetWorkloadStatusHistory(args)

	unit := s.exportImport(c, initial)
	for i, point := range unit.WorkloadStatusHistory() {
		c.Check(point.Value(), gc.Equals, args[i].Value)
		c.Check(point.Message(), gc.Equals, args[i].Message)
		c.Check(point.Data(), jc.DeepEquals, args[i].Data)
		c.Check(point.Updated(), gc.Equals, args[i].Updated)
	}
}

func (s *UnitSerializationSuite) TestPayloads(c *gc.C) {
	initial := minimalUnit()
	expected := initial.AddPayload(allPayloadArgs())
	c.Check(expected.Name(), gc.Equals, "bob")
	c.Check(expected.Type(), gc.Equals, "docker")
	c.Check(expected.RawID(), gc.Equals, "d06f00d")
	c.Check(expected.State(), gc.Equals, "running")
	c.Check(expected.Labels(), jc.DeepEquals, []string{"auto", "foo"})

	unit := s.exportImport(c, initial)

	payloads := unit.Payloads()
	c.Assert(payloads, gc.HasLen, 1)
	c.Assert(payloads[0], jc.DeepEquals, expected)
}
