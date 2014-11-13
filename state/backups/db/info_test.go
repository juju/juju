// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&connInfoSuite{})

type connInfoSuite struct {
	testing.BaseSuite
}

func (s *connInfoSuite) TestDBConnInfoValidateOkay(c *gc.C) {
	connInfo := &db.ConnInfo{"a", "b", "c"}
	err := connInfo.Validate()

	c.Check(err, gc.IsNil)
}

func (s *connInfoSuite) TestDBConnInfoCheckMissingAddress(c *gc.C) {
	connInfo := &db.ConnInfo{"", "b", "c"}
	err := connInfo.Validate()

	c.Check(err, gc.ErrorMatches, "missing address")
}

func (s *connInfoSuite) TestDBConnInfoCheckMissingUsername(c *gc.C) {
	connInfo := &db.ConnInfo{"a", "", "c"}
	err := connInfo.Validate()

	c.Check(err, gc.ErrorMatches, "missing username")
}

func (s *connInfoSuite) TestDBConnInfoCheckMissingPassword(c *gc.C) {
	connInfo := &db.ConnInfo{"a", "b", ""}
	err := connInfo.Validate()

	c.Check(err, gc.ErrorMatches, "missing password")
}
