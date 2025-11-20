// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type originSuite struct {
	schematesting.ControllerSuite
}

func TestOriginSuite(t *testing.T) {
	tc.Run(t, &originSuite{})
}

// TestOriginDBValues ensures there's no skew between what's in the
// database table for secret backend origin and the typed consts used in the
// domain packages.
func (s *originSuite) TestOriginDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, origin FROM secret_backend_origin")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Origin]string)
	for rows.Next() {
		var (
			id     int
			origin string
		)
		err := rows.Scan(&id, &origin)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[Origin(id)] = origin
	}
	c.Assert(dbValues, tc.DeepEquals, map[Origin]string{
		BuiltIn: "built-in",
		User:    "user",
	})
}

func (s *originSuite) TestValueBuiltIn(c *tc.C) {
	result, err := BuiltIn.Value()
	c.Assert(err, tc.IsNil)
	c.Check(result, tc.Equals, "built-in")
}

func (s *originSuite) TestValueUser(c *tc.C) {
	result, err := User.Value()
	c.Assert(err, tc.IsNil)
	c.Check(result, tc.Equals, "user")
}

func (s *originSuite) TestValueInvalid(c *tc.C) {
	_, err := Origin(42).Value()
	c.Assert(err, tc.ErrorMatches, "invalid origin value 42")
}
