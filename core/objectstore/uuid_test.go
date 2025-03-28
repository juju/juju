// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/uuid"
)

type ObjectStoreUUIDSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ObjectStoreUUIDSuite{})

func (*ObjectStoreUUIDSuite) TestUUIDValidate(c *gc.C) {
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

func (*ObjectStoreUUIDSuite) TestUUIDIsEmpty(c *gc.C) {
	tests := []struct {
		uuid  string
		value bool
	}{
		{
			uuid:  "",
			value: true,
		},
		{
			uuid:  "invalid",
			value: false,
		},
		{
			uuid:  uuid.MustNewUUID().String(),
			value: false,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.uuid)
		empty := UUID(test.uuid).IsEmpty()

		c.Check(empty, gc.Equals, test.value)
	}
}
