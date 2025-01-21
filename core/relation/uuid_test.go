// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/uuid"
)

type relationUUIDSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&relationUUIDSuite{})

func (*relationUUIDSuite) TestUUIDValidate(c *gc.C) {
	// Test that the uuid.Validate method succeeds and
	// fails as expected.
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

type relationUnitUUIDSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&relationUnitUUIDSuite{})

func (*relationUnitUUIDSuite) TestUUIDValidate(c *gc.C) {
	// Test that the uuid.Validate method succeeds and
	// fails as expected.
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
