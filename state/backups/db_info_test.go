// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/dustin/go-humanize"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&dbInfoSuite{})

type dbInfoSuite struct {
	testing.BaseSuite
}

type fakeSession struct {
	dbNames []string
	db      *fakeDatabase
}

type fakeDatabase struct {
	result bson.M
}

func (f *fakeSession) DatabaseNames() ([]string, error) {
	return f.dbNames, nil
}

func (f *fakeSession) DB(name string) backups.Database {
	return f.db
}

func (f *fakeDatabase) Run(cmd interface{}, result interface{}) error {
	cmdInfo, ok := cmd.(bson.D)
	if !ok || len(cmdInfo) < 2 {
		return errors.Errorf("unexpected cmd data %#v", cmd)
	}
	if name := cmdInfo[0].Name; name != "dbStats" {
		return errors.Errorf("unexpected cmd %q", name)
	}
	if cmdInfo[1].Name != "scale" || cmdInfo[1].Value != humanize.MiByte {
		return errors.Errorf("unexpected cmd parameter %#v", cmdInfo[1])
	}
	*(result.(*bson.M)) = f.result
	return nil
}

func (s *dbInfoSuite) TestNewDBInfoOkay(c *gc.C) {
	session := fakeSession{
		dbNames: []string{"juju"},
		db: &fakeDatabase{
			result: bson.M{"dataSize": 666.0},
		}}

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
	c.Check(dbInfo.ApproxSizeMB, gc.Equals, 666)
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
