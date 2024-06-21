// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/uuid"
)

type machineSuite struct {
}

var _ = gc.Suite(&machineSuite{})

// TestIdValidate is testing several good and not so good machine id's to check
// that the validate method produces the correct result.
func (*machineSuite) TestIdValidate(c *gc.C) {
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
		err := Id(test.id).Validate()

		if test.err == nil {
			c.Check(err, gc.IsNil)
			continue
		}

		c.Check(err, jc.ErrorIs, test.err)
	}
}
