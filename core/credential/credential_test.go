// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type typeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typeSuite{})

func (s *typeSuite) TestCredentialIdIsZero(c *gc.C) {
	c.Assert(ID{}.IsZero(), jc.IsTrue)
}

func (s *typeSuite) TestCredentialIdIsNotZero(c *gc.C) {
	tests := []ID{
		{
			Owner: "wallyworld",
		},
		{
			Cloud: "somecloud",
		},
		{
			Name: "mycred",
		},
		{
			Cloud: "somecloud",
			Owner: "wallyworld",
			Name:  "somecred",
		},
	}

	for _, test := range tests {
		c.Assert(test.IsZero(), jc.IsFalse)
	}
}

func (s *typeSuite) TestCredentialIdValidate(c *gc.C) {
	tests := []struct {
		Id  ID
		Err error
	}{
		{
			Id: ID{
				Cloud: "",
				Name:  "wallyworld",
				Owner: "wallyworld",
			},
			Err: errors.NotValid,
		},
		{
			Id: ID{
				Cloud: "my-cloud",
				Name:  "",
				Owner: "wallyworld",
			},
			Err: errors.NotValid,
		},
		{
			Id: ID{
				Cloud: "my-cloud",
				Name:  "wallyworld",
				Owner: "",
			},
			Err: errors.NotValid,
		},
		{
			Id: ID{
				Cloud: "my-cloud",
				Name:  "wallyworld",
				Owner: "wallyworld",
			},
			Err: nil,
		},
	}

	for _, test := range tests {
		err := test.Id.Validate()
		if test.Err == nil {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, jc.ErrorIs, test.Err)
		}
	}
}
