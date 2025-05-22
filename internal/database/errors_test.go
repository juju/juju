// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	stdtesting "testing"

	dqlite "github.com/canonical/go-dqlite/v2/driver"
	"github.com/juju/tc"
	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/internal/testhelpers"
)

type errorSuite struct {
	testhelpers.IsolationSuite
}

func TestErrorSuite(t *stdtesting.T) {
	tc.Run(t, &errorSuite{})
}

func (s *errorSuite) TestIsErrConstraintUnique(c *tc.C) {
	c.Check(IsErrConstraintUnique(nil), tc.IsFalse)

	dErr := dqlite.Error{}
	c.Check(IsErrConstraintUnique(dErr), tc.IsFalse)

	dErr.Code = int(sqlite3.ErrConstraintUnique)
	c.Check(IsErrConstraintUnique(dErr), tc.IsTrue)

	sErr := sqlite3.Error{}
	c.Check(IsErrConstraintUnique(sErr), tc.IsFalse)

	sErr.ExtendedCode = sqlite3.ErrConstraintUnique
	c.Check(IsErrConstraintUnique(sErr), tc.IsTrue)
}
