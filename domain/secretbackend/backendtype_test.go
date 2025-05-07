// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
)

type backendtypeSuite struct {
	schematesting.ControllerSuite
}

var _ = tc.Suite(&backendtypeSuite{})

// TestBackendTypeDBValues ensures there's no skew between what's in the
// database table for role and the typed consts used in the secretbackend package.
func (s *backendtypeSuite) TestBackendTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, type FROM secret_backend_type")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[BackendType]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[BackendType(id)] = value
	}
	c.Assert(dbValues, jc.DeepEquals, map[BackendType]string{
		BackendTypeController: juju.BackendType,
		BackendTypeKubernetes: kubernetes.BackendType,
		BackendTypeVault:      vault.BackendType,
	})
}
