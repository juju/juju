// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/uuid"
)

type typeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typeSuite{})

func (s *typeSuite) TestCredentialKeyIsZero(c *gc.C) {
	c.Assert(Key{}.IsZero(), jc.IsTrue)
}

func (s *typeSuite) TestCredentialKeyIsNotZero(c *gc.C) {
	tests := []Key{
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

func (s *typeSuite) TestCredentialKeyValidate(c *gc.C) {
	tests := []struct {
		Key Key
		Err error
	}{
		{
			Key: Key{
				Cloud: "",
				Name:  "wallyworld",
				Owner: "wallyworld",
			},
			Err: errors.NotValid,
		},
		{
			Key: Key{
				Cloud: "my-cloud",
				Name:  "",
				Owner: "wallyworld",
			},
			Err: errors.NotValid,
		},
		{
			Key: Key{
				Cloud: "my-cloud",
				Name:  "wallyworld",
				Owner: "",
			},
			Err: errors.NotValid,
		},
		{
			Key: Key{
				Cloud: "my-cloud",
				Name:  "wallyworld",
				Owner: "wallyworld",
			},
			Err: nil,
		},
	}

	for _, test := range tests {
		err := test.Key.Validate()
		if test.Err == nil {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, jc.ErrorIs, test.Err)
		}
	}
}

func (*typeSuite) TestIDValidate(c *gc.C) {
	tests := []struct {
		id  string
		err error
	}{
		{
			id:  "",
			err: errors.NotValid,
		},
		{
			id:  "invalid",
			err: errors.NotValid,
		},
		{
			id: uuid.MustNewUUID().String(),
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.id)
		err := ID(test.id).Validate()

		if test.err == nil {
			c.Check(err, gc.IsNil)
			continue
		}

		c.Check(err, jc.ErrorIs, test.err)
	}
}
