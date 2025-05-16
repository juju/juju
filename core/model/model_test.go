// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type ModelSuite struct {
	testhelpers.IsolationSuite
}

func TestModelSuite(t *stdtesting.T) { tc.Run(t, &ModelSuite{}) }
func (*ModelSuite) TestValidModelTypes(c *tc.C) {
	validTypes := []ModelType{
		CAAS,
		IAAS,
	}

	for _, vt := range validTypes {
		c.Assert(vt.IsValid(), tc.IsTrue)
	}
}

func (*ModelSuite) TestParseModelTypes(c *tc.C) {
	validTypes := []string{
		"caas",
		"iaas",
	}

	for _, vt := range validTypes {
		mt, err := ParseModelType(vt)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mt.IsValid(), tc.IsTrue)
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

		c.Check(err, tc.ErrorIs, test.err)
	}
}
