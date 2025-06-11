// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type uuidSuite struct {
	testhelpers.IsolationSuite
}

func TestUUIDSuite(t *testing.T) {
	tc.Run(t, &uuidSuite{})
}

type subTest struct {
	uuid string
	err  *string
}

func (s *uuidSuite) TestSubnetUUIDValidate(c *tc.C) {
	for i, test := range getSubTests() {
		c.Logf("test %d: %q", i, test.uuid)

		c.Run(fmt.Sprintf("Test%d", i), func(t *testing.T) {
			c := &tc.TBC{TB: t}

			err := SubnetUUID(test.uuid).Validate()

			if test.err == nil {
				c.Check(err, tc.IsNil)
				return
			}

			c.Check(err, tc.ErrorMatches, *test.err)
		})
	}
}

func (s *uuidSuite) TestInterfaceUUIDValidate(c *tc.C) {
	for i, test := range getSubTests() {
		c.Logf("test %d: %q", i, test.uuid)

		c.Run(fmt.Sprintf("Test%d", i), func(t *testing.T) {
			c := &tc.TBC{TB: t}

			err := InterfaceUUID(test.uuid).Validate()

			if test.err == nil {
				c.Check(err, tc.IsNil)
				return
			}

			c.Check(err, tc.ErrorMatches, *test.err)
		})
	}
}

func (s *uuidSuite) TestAddressUUIDValidate(c *tc.C) {
	for i, test := range getSubTests() {
		c.Logf("test %d: %q", i, test.uuid)

		c.Run(fmt.Sprintf("Test%d", i), func(t *testing.T) {
			c := &tc.TBC{TB: t}

			err := AddressUUID(test.uuid).Validate()

			if test.err == nil {
				c.Check(err, tc.IsNil)
				return
			}

			c.Check(err, tc.ErrorMatches, *test.err)
		})
	}
}

func (s *uuidSuite) TestNetNodeUUIDValidate(c *tc.C) {
	for i, test := range getSubTests() {
		c.Logf("test %d: %q", i, test.uuid)

		c.Run(fmt.Sprintf("Test%d", i), func(t *testing.T) {
			c := &tc.TBC{TB: t}

			err := NetNodeUUID(test.uuid).Validate()

			if test.err == nil {
				c.Check(err, tc.IsNil)
				return
			}

			c.Check(err, tc.ErrorMatches, *test.err)
		})
	}
}

func getSubTests() []subTest {
	return []subTest{
		{
			uuid: "",
			err:  ptr("empty uuid"),
		},
		{
			uuid: "invalid",
			err:  ptr("invalid uuid.*"),
		},
		{
			uuid: internaluuid.MustNewUUID().String(),
		},
	}
}

func ptr[T any](v T) *T {
	return &v
}
