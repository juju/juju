// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/mongo/utils"
)

type ValidFieldSuite struct{}

var _ = gc.Suite(&ValidFieldSuite{})

func (s *ValidFieldSuite) TestOk(c *gc.C) {
	c.Check(utils.IsValidFieldName("foo"), jc.IsTrue)
}

func (s *ValidFieldSuite) TestEmpty(c *gc.C) {
	c.Check(utils.IsValidFieldName(""), jc.IsFalse)
}

func (s *ValidFieldSuite) TestDollarPrefix(c *gc.C) {
	c.Check(utils.IsValidFieldName("$foo"), jc.IsFalse)
}

func (s *ValidFieldSuite) TestEmbeddedDollar(c *gc.C) {
	c.Check(utils.IsValidFieldName("foo$bar"), jc.IsTrue)
}

func (s *ValidFieldSuite) TestDot(c *gc.C) {
	c.Check(utils.IsValidFieldName(".foo"), jc.IsFalse)
	c.Check(utils.IsValidFieldName("foo.bar"), jc.IsFalse)
	c.Check(utils.IsValidFieldName("bar."), jc.IsFalse)
}

func (s *ValidFieldSuite) TestCheckStorableOk(c *gc.C) {
	type Doc struct {
		A string            `bson:"a"`
		B map[string]string `bson:"b"`
	}

	c.Check(utils.CheckStorable(Doc{
		A: "hi",
		B: map[string]string{
			"some":  "thing",
			"other": "thing",
		},
	}), jc.ErrorIsNil)
}

func (s *ValidFieldSuite) TestCheckStorableBad(c *gc.C) {
	type Doc struct {
		A string `bson:"$a"`
	}
	c.Check(utils.CheckStorable(Doc{
		A: "hi",
	}), gc.ErrorMatches, `"\$a" is not a valid field name`)
}

func (s *ValidFieldSuite) TestCheckStorableBadNested(c *gc.C) {
	type Doc struct {
		A map[string]string `bson:"a"`
	}

	c.Check(utils.CheckStorable(Doc{
		A: map[string]string{
			"some": "thing",
			"$foo": "thing",
		},
	}), gc.ErrorMatches, `"\$foo" is not a valid field name`)
}

func (s *ValidFieldSuite) TestCheckStorableBadDeepNested(c *gc.C) {
	type SubDoc struct {
		A int               `bson:"a"`
		B map[string]string `bson:"b"`
	}
	type Doc struct {
		A map[string]map[string]SubDoc `bson:"a"`
	}

	c.Check(utils.CheckStorable(Doc{
		A: map[string]map[string]SubDoc{
			"foo": {
				"foo": {
					A: 99,
					B: map[string]string{
						"some":    "thing",
						"foo.bar": "thing",
					},
				},
			},
		},
	}), gc.ErrorMatches, `"foo.bar" is not a valid field name`)
}
