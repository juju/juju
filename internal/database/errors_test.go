// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	dqlite "github.com/canonical/go-dqlite/driver"
	"github.com/juju/testing"
	"github.com/juju/testing/checkers"
	"github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"
)

type errorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&errorSuite{})

func (s *errorSuite) TestIsErrConstraintUnique(c *gc.C) {
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

func (s *errorSuite) TestIsErrCode(c *gc.C) {
	c.Check(isErrCode(nil, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	dErr := dqlite.Error{}
	c.Check(isErrCode(dErr, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	dErr.Code = int(sqlite3.ErrConstraintUnique)
	c.Check(isErrCode(dErr, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	dErr.Code = int(sqlite3.ErrConstraintUnique)
	c.Check(isErrCode(dErr, sqlite3.ErrConstraintUnique), checkers.IsTrue)

	sErr := sqlite3.Error{}
	c.Check(isErrCode(sErr, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	sErr.ExtendedCode = sqlite3.ErrConstraintUnique
	c.Check(isErrCode(sErr, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	sErr.ExtendedCode = sqlite3.ErrConstraintUnique
	c.Check(isErrCode(sErr, sqlite3.ErrConstraintUnique), checkers.IsTrue)
}
