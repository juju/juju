// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

import (
	"math"
	"testing"

	"github.com/juju/tc"
)

type unitNameSuite struct{}

func TestUnitNameSuite(t *testing.T) {
	tc.Run(t, &unitNameSuite{})
}

func (*unitNameSuite) TestNameValidate(c *tc.C) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "",
			err:  InvalidUnitName,
		},
		{
			name: "/1",
			err:  InvalidUnitName,
		},
		{
			name: "app",
			err:  InvalidUnitName,
		},
		{
			name: "app-0",
			err:  InvalidUnitName,
		},
		{
			name: "app/0",
		},
		{
			name: "app/00",
			err:  InvalidUnitName,
		},
		{
			name: "app/01",
			err:  InvalidUnitName,
		},
		{
			name: "app_lication/0",
			err:  InvalidUnitName,
		},
		{
			name: "app/-1",
			err:  InvalidUnitName,
		},
		{
			name: "app/10000",
		},
		{
			name: "app/1/1",
			err:  InvalidUnitName,
		},
		{
			name: "app/1/1",
			err:  InvalidUnitName,
		},
		{
			name: "app-100/1",
			err:  InvalidUnitName,
		},
		{
			name: "app/1b",
			err:  InvalidUnitName,
		},
		{
			name: "app/b1",
			err:  InvalidUnitName,
		},
		{
			name: "app/1-1",
			err:  InvalidUnitName,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.name)
		err := Name(test.name).Validate()

		if test.err == nil {
			c.Check(err, tc.ErrorIsNil)
			continue
		}

		c.Check(err, tc.ErrorIs, test.err)
	}
}

func (*unitNameSuite) TestNewNameFromParts(c *tc.C) {
	tests := []struct {
		application string
		number      int
		err         error
	}{
		{
			application: "",
			number:      0,
			err:         InvalidUnitName,
		},
		{
			application: "app",
			number:      0,
		},
		{
			application: "app",
			number:      -1,
			err:         InvalidUnitName,
		},
		{
			application: "valid",
			number:      10000,
		},
		{
			application: "app",
			number:      math.MaxInt64,
		},
		{
			application: "app",
			number:      math.MinInt64,
			err:         InvalidUnitName,
		},
		{
			application: "app/0",
			number:      0,
			err:         InvalidUnitName,
		},
		{
			application: "app-0",
			number:      0,
			err:         InvalidUnitName,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %q", i, test.application)
		_, err := NewNameFromParts(test.application, test.number)

		if test.err == nil {
			c.Check(err, tc.ErrorIsNil)
			continue
		}

		c.Check(err, tc.ErrorIs, test.err)
	}
}

func (*unitNameSuite) TestApplicationName(c *tc.C) {
	unitName, err := NewName("app/666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitName.Application(), tc.Equals, "app")
}

func (*unitNameSuite) TestNumber(c *tc.C) {
	unitName, err := NewName("app/666")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitName.Number(), tc.Equals, 666)
}

func (*unitNameSuite) TestNumberInvalidName(c *tc.C) {
	c.Assert(Name("").Number(), tc.Equals, -1)
}
