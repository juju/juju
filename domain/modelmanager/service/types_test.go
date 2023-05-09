// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/testing"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestUUIDValidate(c *gc.C) {
	tests := []struct {
		uuid string
		err  *string
	}{
		{
			uuid: "",
			err:  ptr("empty uuid"),
		},
		{
			uuid: "invalid",
			err:  ptr("invalid uuid.*"),
		},
		{
			uuid: utils.MustNewUUID().String(),
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.uuid)
		err := UUID(test.uuid).Validate()

		if test.err == nil {
			c.Check(err, gc.IsNil)
			continue
		}

		c.Check(err, gc.ErrorMatches, *test.err)
	}
}

func ptr[T any](v T) *T {
	return &v
}
