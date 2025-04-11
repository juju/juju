// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/uuid"
)

type ModelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ModelSuite{})

func (*ModelSuite) TestValidModelTypes(c *gc.C) {
	validTypes := []ModelType{
		CAAS,
		IAAS,
	}

	for _, vt := range validTypes {
		c.Assert(vt.IsValid(), jc.IsTrue)
	}
}

func (*ModelSuite) TestParseModelTypes(c *gc.C) {
	validTypes := []string{
		"caas",
		"iaas",
	}

	for _, vt := range validTypes {
		mt, err := ParseModelType(vt)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(mt.IsValid(), jc.IsTrue)
	}
}

func (*ModelSuite) TestParseModelTypesInvalid(c *gc.C) {
	_, err := ParseModelType("foo")
	c.Assert(err, gc.ErrorMatches, `unknown model type "foo"`)
}

func (*ModelSuite) TestUUIDValidate(c *gc.C) {
	tests := []struct {
		uuid string
		err  error
	}{
		{
			uuid: "",
			err:  coreerrors.NotValid,
		},
		{
			uuid: "invalid",
			err:  coreerrors.NotValid,
		},
		{
			uuid: uuid.MustNewUUID().String(),
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.uuid)
		err := UUID(test.uuid).Validate()

		if test.err == nil {
			c.Check(err, gc.IsNil)
			continue
		}

		c.Check(err, jc.ErrorIs, test.err)
	}
}
