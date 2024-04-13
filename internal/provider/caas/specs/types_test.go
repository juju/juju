// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/provider/caas/specs"
	"github.com/juju/juju/testing"
)

type typesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&typesSuite{})

var strVal = specs.IntOrString{Type: specs.String, StrVal: "10%"}
var intVal = specs.IntOrString{Type: specs.Int, IntVal: 10}

func (s *typesSuite) TestString(c *gc.C) {
	c.Assert(strVal.String(), gc.DeepEquals, `10%`)
	c.Assert(intVal.String(), gc.DeepEquals, `10`)
}

func (s *typesSuite) TestIntValue(c *gc.C) {
	c.Assert(strVal.IntValue(), gc.DeepEquals, 0)
	c.Assert(intVal.IntValue(), gc.DeepEquals, 10)
}

func (s *typesSuite) TestMarshalJSON(c *gc.C) {
	o, err := strVal.MarshalJSON()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, gc.DeepEquals, []byte(`"10%"`))

	o, err = intVal.MarshalJSON()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(o, gc.DeepEquals, []byte(`10`))
}

func (s *typesSuite) TestUnmarshalJSON(c *gc.C) {
	var strVal1, intVal1 specs.IntOrString
	err := strVal1.UnmarshalJSON([]byte(`"10%"`))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strVal1, gc.DeepEquals, strVal)

	err = intVal1.UnmarshalJSON([]byte(`10`))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(intVal1, gc.DeepEquals, intVal)
}
