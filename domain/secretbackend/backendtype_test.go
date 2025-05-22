// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	stdtesting "testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/vault"
)

type backendtypeSuite struct {
	schematesting.ControllerSuite
}

func TestBackendtypeSuite(t *stdtesting.T) {
	tc.Run(t, &backendtypeSuite{})
}

// TestBackendTypeDBValues ensures there's no skew between what's in the
// database table for role and the typed consts used in the secretbackend package.
func (s *backendtypeSuite) TestBackendTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, type FROM secret_backend_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[BackendType]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[BackendType(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[BackendType]string{
		BackendTypeController: juju.BackendType,
		BackendTypeKubernetes: kubernetes.BackendType,
		BackendTypeVault:      vault.BackendType,
	})
}
