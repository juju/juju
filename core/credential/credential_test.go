// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credential

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type typeSuite struct {
	testhelpers.IsolationSuite
}

func TestTypeSuite(t *stdtesting.T) {
	tc.Run(t, &typeSuite{})
}

func (s *typeSuite) TestCredentialKeyIsZero(c *tc.C) {
	c.Assert(Key{}.IsZero(), tc.IsTrue)
}

func (s *typeSuite) TestCredentialKeyIsNotZero(c *tc.C) {
	tests := []Key{
		{
			Owner: usertesting.GenNewName(c, "wallyworld"),
		},
		{
			Cloud: "somecloud",
		},
		{
			Name: "mycred",
		},
		{
			Cloud: "somecloud",
			Owner: usertesting.GenNewName(c, "wallyworld"),
			Name:  "somecred",
		},
	}

	for _, test := range tests {
		c.Assert(test.IsZero(), tc.IsFalse)
	}
}

func (s *typeSuite) TestCredentialKeyValidate(c *tc.C) {
	tests := []struct {
		Key Key
		Err error
	}{
		{
			Key: Key{
				Cloud: "",
				Name:  "wallyworld",
				Owner: usertesting.GenNewName(c, "wallyworld"),
			},
			Err: coreerrors.NotValid,
		},
		{
			Key: Key{
				Cloud: "my-cloud",
				Name:  "",
				Owner: usertesting.GenNewName(c, "wallyworld"),
			},
			Err: coreerrors.NotValid,
		},
		{
			Key: Key{
				Cloud: "my-cloud",
				Name:  "wallyworld",
				Owner: user.Name{},
			},
			Err: coreerrors.NotValid,
		},
		{
			Key: Key{
				Cloud: "my-cloud",
				Name:  "wallyworld",
				Owner: usertesting.GenNewName(c, "wallyworld"),
			},
			Err: nil,
		},
	}

	for _, test := range tests {
		err := test.Key.Validate()
		if test.Err == nil {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Assert(err, tc.ErrorIs, test.Err)
		}
	}
}

func (*typeSuite) TestUUIDValidate(c *tc.C) {
	tests := []struct {
		id  string
		err error
	}{
		{
			id:  "",
			err: coreerrors.NotValid,
		},
		{
			id:  "invalid",
			err: coreerrors.NotValid,
		},
		{
			id: uuid.MustNewUUID().String(),
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.id)
		err := UUID(test.id).Validate()

		if test.err == nil {
			c.Check(err, tc.IsNil)
			continue
		}

		c.Check(err, tc.ErrorIs, test.err)
	}
}
