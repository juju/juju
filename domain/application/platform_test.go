// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/os/ostype"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type ostypeSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&ostypeSuite{})

// TestOsTypeDBValues ensures there's no skew between what's in the
// database table for os type and the typed consts used in the state packages.
func (s *ostypeSuite) TestOsTypeDBValues(c *gc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM os")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[OSType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[OSType(id)] = name
	}
	c.Assert(dbValues, jc.DeepEquals, map[OSType]string{
		Ubuntu: "ubuntu",
	})
	// Also check the core os type enums match.
	for _, os := range dbValues {
		c.Assert(ostype.IsValidOSTypeName(os), jc.IsTrue)
	}
}

type architectureSuite struct {
	schematesting.ModelSuite
}

var _ = gc.Suite(&architectureSuite{})

// TestArchitectureDBValues ensures there's no skew between what's in the
// database table for architecture and the typed consts used in the state packages.
func (s *architectureSuite) TestArchitectureDBValues(c *gc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM architecture")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[Architecture]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[Architecture(id)] = name
	}
	c.Assert(dbValues, jc.DeepEquals, map[Architecture]string{
		AMD64:   "amd64",
		ARM64:   "arm64",
		PPC64EL: "ppc64el",
		S390X:   "s390x",
		RISV64:  "riscv64",
	})
	// Also check the core arch enums match.
	for _, a := range dbValues {
		c.Assert(arch.IsSupportedArch(a), jc.IsTrue)
	}
}
