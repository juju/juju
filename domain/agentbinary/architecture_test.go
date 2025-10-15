// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinary

import (
	"database/sql"
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type controllerSuite struct {
	schematesting.ControllerSuite
}

type modelSuite struct {
	schematesting.ModelSuite
}

func TestControllerSuite(t *testing.T) {
	tc.Run(t, &controllerSuite{})
}

func TestModelSuite(t *testing.T) {
	tc.Run(t, &modelSuite{})
}

// testArchitectureValuesAlignedToDB tests that architectures values in the DB
// aligns with the architecture names and IDs we have defined in the application level.
func testArchitectureValuesAlignedToDB(c *tc.C, db *sql.DB) {
	rows, err := db.Query("SELECT id, name FROM architecture ORDER BY ID ASC")
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

	err = rows.Err()
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(archs, tc.DeepEquals, []architecture{
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

// TestArchitectureValuesAlignedToControllerDB tests for controller DB.
func (s *controllerSuite) TestArchitectureValuesAlignedToControllerDB(c *tc.C) {
	testArchitectureValuesAlignedToDB(c, s.DB())
}

// TestArchitectureValuesAlignedToControllerDB tests for model DB.
func (s *modelSuite) TestArchitectureValuesAlignedToModelDB(c *tc.C) {
	testArchitectureValuesAlignedToDB(c, s.DB())
}
