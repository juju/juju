// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	dqlite "github.com/canonical/go-dqlite/v2/driver"
	"github.com/juju/tc"
	"github.com/juju/testing/checkers"
	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/internal/testhelpers"
)

type errorSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&errorSuite{})

func (s *errorSuite) TestIsErrConstraintUnique(c *tc.C) {
	c.Check(IsErrConstraintUnique(nil), checkers.IsFalse)

	dErr := dqlite.Error{}
	c.Check(IsErrConstraintUnique(dErr), checkers.IsFalse)

	dErr.Code = int(sqlite3.ErrConstraintUnique)
	c.Check(IsErrConstraintUnique(dErr), checkers.IsTrue)

	sErr := sqlite3.Error{}
	c.Check(IsErrConstraintUnique(sErr), checkers.IsFalse)

	sErr.ExtendedCode = sqlite3.ErrConstraintUnique
	c.Check(IsErrConstraintUnique(sErr), checkers.IsTrue)
}
