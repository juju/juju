// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coresecrets "github.com/juju/juju/core/secrets"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type grantSuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&grantSuite{})

// TestRoleDBValues ensures there's no skew between what's in the
// database table for role and the typed consts used in the secret package.
func (s *grantSuite) TestRoleDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, role FROM secret_role")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Role]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[Role(id)] = value
	}
	c.Assert(dbValues, jc.DeepEquals, map[Role]string{
		RoleNone:   "none",
		RoleView:   "view",
		RoleManage: "manage",
	})
	// Also check the core secret enums match.
	for _, p := range dbValues {
		if p == "none" {
			p = ""
		}
		c.Assert(coresecrets.SecretRole(p).IsValid(), jc.IsTrue)
	}
}

func (s *grantSuite) TestGrantSubjectTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, type FROM secret_grant_subject_type")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[GrantSubjectType]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[GrantSubjectType(id)] = value
	}
	c.Assert(dbValues, jc.DeepEquals, map[GrantSubjectType]string{
		SubjectUnit:              "unit",
		SubjectApplication:       "application",
		SubjectModel:             "model",
		SubjectRemoteApplication: "remote-application",
	})
}

func (s *grantSuite) TestGrantScopeTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, type FROM secret_grant_scope_type")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[GrantScopeType]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[GrantScopeType(id)] = value
	}
	c.Assert(dbValues, jc.DeepEquals, map[GrantScopeType]string{
		ScopeUnit:        "unit",
		ScopeApplication: "application",
		ScopeModel:       "model",
		ScopeRelation:    "relation",
	})
}
