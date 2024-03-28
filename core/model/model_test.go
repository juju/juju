// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/uuid"
)

type ModelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ModelSuite{})

func (*ModelSuite) TestValidateBranchName(c *gc.C) {
	for _, t := range []struct {
		branchName string
		valid      bool
	}{
		{branchName: "", valid: false},
		{branchName: GenerationMaster, valid: false},
		{branchName: "something else", valid: true},
	} {
		err := ValidateBranchName(t.branchName)
		if t.valid {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.ErrorIs, errors.NotValid)
		}
	}
}

func (*ModelSuite) TestValidModelTypes(c *gc.C) {
	validTypes := []ModelType{
		CAAS,
		IAAS,
	}

	for _, vt := range validTypes {
		c.Assert(vt.IsValid(), jc.IsTrue)
	}
}

func (*ModelSuite) TestUUIDValidate(c *gc.C) {
	tests := []struct {
		uuid string
		err  error
	}{
		{
			uuid: "",
			err:  errors.NotValid,
		},
		{
			uuid: "invalid",
			err:  errors.NotValid,
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
