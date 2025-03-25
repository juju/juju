// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/uuid"
)

type resourcesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&resourcesSuite{})

func (*resourcesSuite) TestUUIDValidate(c *gc.C) {
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

func (*resourcesSuite) TestParseUUID(c *gc.C) {
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
		id, err := ParseUUID(test.uuid)

		if test.err == nil {
			if c.Check(err, gc.IsNil) {
				c.Check(id.String(), gc.Equals, test.uuid)
			}
			continue
		}

		c.Check(err, jc.ErrorIs, test.err)
	}
}
