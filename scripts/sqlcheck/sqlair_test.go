// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"strings"
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type sqlairSuite struct {
	coretesting.BaseSuite
}

func TestSQLairSuite(t *testing.T) {
	tc.Run(t, &sqlairSuite{})
}

func (s *sqlairSuite) TestReplacements(c *tc.C) {
	tests := []struct {
		in    string
		token string
	}{{
		in:    "SELECT &Foo.x FROM t WHERE id = $Bar.id",
		token: "c",
	}, {
		in:    "SELECT &Type.* FROM t",
		token: "c",
	}, {
		in:    "SELECT (name, postcode) AS &M.* FROM t",
		token: "FROM t",
	}, {
		in:    "SELECT (*) AS (&cloudWithAuthType.*) FROM v_cloud_auth",
		token: "FROM v_cloud_auth",
	}, {
		in:    "SELECT p.* AS &Person.* FROM p",
		token: "FROM p",
	}, {
		in:    "WHERE name IN ($Names[:])",
		token: "1",
	}, {
		in:    "INSERT INTO secret_reservation (*) VALUES ($secretReservation.*)",
		token: "(c)",
	}, {
		in:    "INSERT INTO person (name, postcode) VALUES ($Person.name, $Person.postcode)",
		token: "VALUES",
	}, {
		in:    "SELECT COUNT(*) FROM t WHERE uuid = $cloudID.uuid",
		token: "1",
	}, {
		in:    "SELECT other_city AS &M.city FROM other_people",
		token: "AS c",
	}, {
		in:    "SELECT (p.name, a.address_id) AS (&Person.*) FROM p",
		token: "FROM p",
	}}
	for i, test := range tests {
		c.Logf("case %d: %s", i, test.in)
		got := replaceSQLair(literalEval(test.in))
		c.Check(strings.Contains(got.Content, "$"), tc.IsFalse)
		c.Check(strings.Contains(got.Content, "&"), tc.IsFalse)
		if strings.Contains(test.in, "COUNT(*)") {
			c.Check(strings.Contains(got.Content, "COUNT(*)"), tc.IsTrue)
		}
		c.Check(strings.Contains(got.Content, test.token), tc.IsTrue)
	}
}

func (s *sqlairSuite) TestSQLPlaceholders(c *tc.C) {
	got := toSQLite(literalEval(`SELECT uuid FROM t WHERE name = ? AND id = ?1`))
	c.Check(strings.Contains(got.Content, "?"), tc.IsFalse)
	c.Check(strings.Contains(got.Content, "name = 1"), tc.IsTrue)
}

func (s *sqlairSuite) TestDoesNotRewriteSQLStringsOrComments(c *tc.C) {
	in := "SELECT '&Foo.x' FROM t -- $Bar.id\n"
	got := replaceSQLair(literalEval(in))
	c.Check(got.Content, tc.Equals, in)
}

func literalEval(s string) *evaluated {
	m := make([]origByte, len(s))
	for i := range s {
		m[i] = origByte{File: "foo.go", Offset: 10 + i, Kind: KindLiteral}
	}
	return &evaluated{Content: s, Map: m}
}
