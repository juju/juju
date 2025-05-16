// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/caas/specs"
	"github.com/juju/juju/internal/testing"
)

type typesSuite struct {
	testing.BaseSuite
}

func TestTypesSuite(t *stdtesting.T) { tc.Run(t, &typesSuite{}) }

var strVal = specs.IntOrString{Type: specs.String, StrVal: "10%"}
var intVal = specs.IntOrString{Type: specs.Int, IntVal: 10}

func (s *typesSuite) TestString(c *tc.C) {
	c.Assert(strVal.String(), tc.DeepEquals, `10%`)
	c.Assert(intVal.String(), tc.DeepEquals, `10`)
}

func (s *typesSuite) TestIntValue(c *tc.C) {
	c.Assert(strVal.IntValue(), tc.DeepEquals, 0)
	c.Assert(intVal.IntValue(), tc.DeepEquals, 10)
}

func (s *typesSuite) TestMarshalJSON(c *tc.C) {
	o, err := strVal.MarshalJSON()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(o, tc.DeepEquals, []byte(`"10%"`))

	o, err = intVal.MarshalJSON()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(o, tc.DeepEquals, []byte(`10`))
}

func (s *typesSuite) TestUnmarshalJSON(c *tc.C) {
	var strVal1, intVal1 specs.IntOrString
	err := strVal1.UnmarshalJSON([]byte(`"10%"`))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(strVal1, tc.DeepEquals, strVal)

	err = intVal1.UnmarshalJSON([]byte(`10`))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(intVal1, tc.DeepEquals, intVal)
}
