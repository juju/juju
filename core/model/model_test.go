// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
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
		{"", false},
		{GenerationMaster, false},
		{"something else", true},
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
