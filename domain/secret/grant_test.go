// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coresecrets "github.com/juju/juju/core/secrets"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type grantSuite struct {
	schematesting.ModelSuite
}

func TestGrantSuite(t *stdtesting.T) { tc.Run(t, &grantSuite{}) }

// TestRoleDBValues ensures there's no skew between what's in the
// database table for role and the typed consts used in the secret package.
func (s *grantSuite) TestRoleDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, role FROM secret_role")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Role]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[Role(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[Role]string{
		RoleNone:   "none",
		RoleView:   "view",
		RoleManage: "manage",
	})
	// Also check the core secret enums match.
	for _, p := range dbValues {
		if p == "none" {
			p = ""
		}
		c.Assert(coresecrets.SecretRole(p).IsValid(), tc.IsTrue)
	}
}

func (s *grantSuite) TestGrantSubjectTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, type FROM secret_grant_subject_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[GrantSubjectType]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[GrantSubjectType(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[GrantSubjectType]string{
		SubjectUnit:              "unit",
		SubjectApplication:       "application",
		SubjectModel:             "model",
		SubjectRemoteApplication: "remote-application",
	})
}

func (s *grantSuite) TestGrantScopeTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, type FROM secret_grant_scope_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[GrantScopeType]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[GrantScopeType(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[GrantScopeType]string{
		ScopeUnit:        "unit",
		ScopeApplication: "application",
		ScopeModel:       "model",
		ScopeRelation:    "relation",
	})
}
