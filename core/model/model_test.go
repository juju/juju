// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/uuid"
)

type ModelSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ModelSuite{})

func (*ModelSuite) TestValidModelTypes(c *tc.C) {
	validTypes := []ModelType{
		CAAS,
		IAAS,
	}

	for _, vt := range validTypes {
		c.Assert(vt.IsValid(), jc.IsTrue)
	}
}

func (*ModelSuite) TestParseModelTypes(c *tc.C) {
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

func (*ModelSuite) TestParseModelTypesInvalid(c *tc.C) {
	_, err := ParseModelType("foo")
	c.Assert(err, tc.ErrorMatches, `unknown model type "foo"`)
}

func (*ModelSuite) TestUUIDValidate(c *tc.C) {
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
			c.Check(err, tc.IsNil)
			continue
		}

		c.Check(err, jc.ErrorIs, test.err)
	}
}
