// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&dbInfoSuite{})

type dbInfoSuite struct {
	testing.BaseSuite
}

type fakeSession struct {
	dbNames []string
}

func (f *fakeSession) DatabaseNames() ([]string, error) {
	return f.dbNames, nil
}

func (s *dbInfoSuite) TestNewDBInfoOkay(c *gc.C) {
	session := fakeSession{}

	tag, err := names.ParseTag("machine-0")
	c.Assert(err, jc.ErrorIsNil)
	mgoInfo := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Tag:      tag,
		Password: "eggs",
	}
	dbInfo, err := backups.NewDBInfo(mgoInfo, &session)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(dbInfo.Address, gc.Equals, "localhost:8080")
	c.Check(dbInfo.Username, gc.Equals, "machine-0")
	c.Check(dbInfo.Password, gc.Equals, "eggs")
}

func (s *dbInfoSuite) TestNewDBInfoMissingTag(c *gc.C) {
	session := fakeSession{}

	mgoInfo := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs: []string{"localhost:8080"},
		},
		Password: "eggs",
	}
	dbInfo, err := backups.NewDBInfo(mgoInfo, &session)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(dbInfo.Username, gc.Equals, "")
	c.Check(dbInfo.Address, gc.Equals, "localhost:8080")
	c.Check(dbInfo.Password, gc.Equals, "eggs")
}
