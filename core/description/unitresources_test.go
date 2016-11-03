// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type UnitResourceSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&UnitResourceSuite{})

func (s *UnitResourceSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "resources"
	s.sliceName = "resources"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importUnitResources(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["resources"] = []interface{}{}
	}
}

func (s *UnitResourceSuite) TestNew(c *gc.C) {
	ur := newUnitResource(UnitResourceArgs{
		Name:     "blah",
		Revision: 99,
	})
	c.Check(ur.Name(), gc.Equals, "blah")
	c.Check(ur.Revision(), gc.Equals, 99)
}

func (s *UnitResourceSuite) TestParsingSerializedData(c *gc.C) {
	initial := newUnitResource(UnitResourceArgs{
		Name:     "foo",
		Revision: 2,
	})
	imported := s.exportImport(c, initial)
	c.Assert(imported, jc.DeepEquals, initial)
}

func (s *UnitResourceSuite) TestImportEmpty(c *gc.C) {
	r, err := importUnitResources(map[string]interface{}{
		"version":   1,
		"resources": []interface{}{},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.HasLen, 0)
}

func (s *UnitResourceSuite) TestUnsupportedVersion(c *gc.C) {
	_, err := importUnitResources(map[string]interface{}{
		"version":   999,
		"resources": []interface{}{},
	})
	c.Assert(err, gc.ErrorMatches, "version 999 not valid")
}

func (s *UnitResourceSuite) exportImport(c *gc.C, ur *unitResource) *unitResource {
	initial := unitResources{
		Version:    1,
		Resources_: []*unitResource{ur},
	}
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	urs, err := importUnitResources(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(urs, gc.HasLen, 1)
	return urs[0]
}
