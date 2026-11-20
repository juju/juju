// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

// architectureControllerDDLSuite contains a set of tests for asserting the const
// values defined for [Architecture] align with the DDL values defined in
// domain/schema/model/sql/0070-platform.sql.
type architectureControllerDDLSuite struct {
	schematesting.ModelSuite
}

// TestArchitectureControllerDDLSuite runs the tests contained within
// [architectureControllerDDLSuite].
func TestArchitectureControllerDDLSuite(t *testing.T) {
	tc.Run(t, &architectureControllerDDLSuite{})
}

// TestArchitectureValuesAgainstDDL tests that architectures values in the
// controller  DDL aligns with the [Architecture] constants defined in this package.
func (s *architectureControllerDDLSuite) TestArchitectureValuesAgainstDDL(c *tc.C) {
	rows, err := s.DB().QueryContext(
		c.Context(),
		"SELECT id, name FROM architecture",
	)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	type architecture struct {
		Id   int
		Name string
	}

	var arch architecture
	var archs []architecture
	for rows.Next() {
		err := rows.Scan(&arch.Id, &arch.Name)
		c.Assert(err, tc.ErrorIsNil)
		archs = append(archs, arch)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)

	c.Assert(archs, tc.SameContents, []architecture{
		{
			Id:   int(AMD64),
			Name: AMD64.String(),
		},
		{
			Id:   int(ARM64),
			Name: ARM64.String(),
		},
		{
			Id:   int(PPC64EL),
			Name: PPC64EL.String(),
		},
		{
			Id:   int(S390X),
			Name: S390X.String(),
		},
		{
			Id:   int(RISCV64),
			Name: RISCV64.String(),
		},
	})
}
