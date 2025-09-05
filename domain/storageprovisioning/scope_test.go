// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type scopeSuite struct {
	schematesting.ModelSuite
}

// TestScopeSuite runs all of the tests located in the [scopeSuite].
func TestScopeSuite(t *testing.T) {
	tc.Run(t, &scopeSuite{})
}

// TestProvisionScopeValuesAligned asserts that the provision scope values that
// exist in the database schema align with the enum values defined in this
// package.
//
// If this test fails it indicates that either a new value has been added to the
// schema and a new enum needs to be created or a value has been modified or
// removed that will result in a breaking change.
func (s *scopeSuite) TestProvisionScopeValuesAligned(c *tc.C) {

	rows, err := s.DB().QueryContext(
		c.Context(),
		"SELECT id, scope FROM storage_provision_scope",
	)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	dbValues := map[ProvisionScope]string{}
	for rows.Next() {
		var (
			id    int
			scope string
		)

		c.Assert(rows.Scan(&id, &scope), tc.ErrorIsNil)
		dbValues[ProvisionScope(id)] = scope
	}

	c.Check(dbValues, tc.DeepEquals, map[ProvisionScope]string{
		ProvisionScopeModel:   "model",
		ProvisionScopeMachine: "machine",
	})
}
