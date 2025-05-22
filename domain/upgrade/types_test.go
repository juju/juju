// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrade

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *stdtesting.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestUUIDValidate(c *tc.C) {
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

		c.Check(err, tc.ErrorMatches, *test.err)
	}
}

func ptr[T any](v T) *T {
	return &v
}
