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
	rows, err := db.Query("SELECT origin FROM secret_backend_origin")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	var dbValues []string
	for rows.Next() {
		var origin string
		err := rows.Scan(&origin)
		c.Assert(err, tc.ErrorIsNil)
		dbValues = append(dbValues, origin)
	}
	c.Assert(dbValues, tc.SameContents, []string{
		string(BuiltIn),
		string(User),
	})
}

func (s *originSuite) TestValidOrigins(c *tc.C) {
	validOrigins := []Origin{
		BuiltIn,
		User,
	}

	for _, vo := range validOrigins {
		c.Assert(vo.IsValid(), tc.IsTrue)
	}
}

func (s *originSuite) TestParseOrigins(c *tc.C) {
	validOrigins := []string{
		"built-in",
		"user",
	}

	for _, vo := range validOrigins {
		mt, err := ParseOrigin(vo)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(mt.IsValid(), tc.IsTrue)
	}
}

func (s *originSuite) TestParseOriginsInvalid(c *tc.C) {
	_, err := ParseOrigin("foo")
	c.Assert(err, tc.ErrorMatches, `unknown origin "foo"`)
}
