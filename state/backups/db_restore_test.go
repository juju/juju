// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&mongoRestoreSuite{})

type mongoRestoreSuite struct {
	testing.BaseSuite
}

func (s *mongoRestoreSuite) TestRestoreDatabase24(c *gc.C) {
	s.PatchValue(backups.GetMongorestorePath, func() (string, error) { return "/a/fake/mongorestore", nil })
	var ranCommand string
	var ranWithArgs []string
	fakeRunCommand := func(c string, args ...string) error {
		ranCommand = c
		ranWithArgs = args
		return nil
	}
	args := backups.RestorerArgs{
		Version:         mongo.Mongo24,
		TagUser:         "machine-0",
		TagUserPassword: "fakePassword",
		RunCommandFn:    fakeRunCommand,
		StartMongo:      func() error { return nil },
		StopMongo:       func() error { return nil },
	}

	s.PatchValue(backups.MongoInstalledVersion, func() mongo.Version { return mongo.Mongo24 })
	restorer, err := backups.NewDBRestorer(args)
	c.Assert(err, jc.ErrorIsNil)
	err = restorer.Restore("fakePath", nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ranCommand, gc.Equals, "/a/fake/mongorestore")
	c.Assert(ranWithArgs, gc.DeepEquals, []string{"--drop", "--journal", "--oplogReplay", "--dbpath", "/var/lib/juju/db", "fakePath"})
}

type mongoDb struct {
	user *mgo.User
}

func (m *mongoDb) UpsertUser(u *mgo.User) error {
	m.user = u
	return nil
}

type mongoSession struct {
	closed          bool
	createRoleCount int
	cmd             []bson.D
}

func (m *mongoSession) Run(cmd interface{}, result interface{}) error {
	bsoncmd, ok := cmd.(bson.D)
	if !ok {
		return errors.New("unexpected cmd")
	}
	m.cmd = append(m.cmd, bsoncmd)
	bsoncmdMap := bsoncmd.Map()
	if _, ok := bsoncmdMap["createRole"]; ok {
		m.createRoleCount += 1
		if m.createRoleCount > 1 {
			return &mgo.QueryError{
				Code:      11000,
				Message:   fmt.Sprintf("Role %q already exists", "oploger@admin"),
				Assertion: false,
			}
		}
	}
	return nil
}

func (m *mongoSession) Close() {
	m.closed = true
}

func (m *mongoSession) DB(_ string) *mgo.Database {
	return nil
}

func (s *mongoRestoreSuite) assertRestore(c *gc.C) {
	s.PatchValue(backups.GetMongorestorePath, func() (string, error) { return "/a/fake/mongorestore", nil })
	var ranCommand string
	var ranWithArgs []string
	fakeRunCommand := func(c string, args ...string) error {
		ranCommand = c
		ranWithArgs = args
		return nil
	}
	mgoDb := &mongoDb{}
	mgoSession := &mongoSession{}

	args := backups.RestorerArgs{
		DialInfo: &mgo.DialInfo{
			Username: "fakeUsername",
			Password: "fakePassword",
			Addrs:    []string{"127.0.0.1"},
		},
		Version:         mongo.Mongo32wt,
		TagUser:         "machine-0",
		TagUserPassword: "fakePassword",
		GetDB:           func(string, backups.MongoSession) backups.MongoDB { return mgoDb },
		NewMongoSession: func(dialInfo *mgo.DialInfo) (backups.MongoSession, error) {
			return mgoSession, nil
		},
		RunCommandFn: fakeRunCommand,
	}
	s.PatchValue(backups.MongoInstalledVersion, func() mongo.Version { return mongo.Mongo32wt })
	restorer, err := backups.NewDBRestorer(args)
	c.Assert(err, jc.ErrorIsNil)
	err = restorer.Restore("fakePath", nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(ranCommand, gc.Equals, "/a/fake/mongorestore")
	c.Assert(ranWithArgs, gc.DeepEquals, []string{"--ssl", "--sslAllowInvalidCertificates", "--authenticationDatabase", "admin", "--host", "127.0.0.1", "--username", "fakeUsername", "--password", "fakePassword", "--drop", "--oplogReplay", "--batchSize", "10", "fakePath"})
	user := &mgo.User{Username: "machine-0", Password: "fakePassword"}
	c.Assert(mgoDb.user, gc.DeepEquals, user)
	c.Assert(mgoSession.closed, jc.IsTrue)
	mgoSessionCmd := []bson.D{
		{
			bson.DocElem{Name: "createRole", Value: "oploger"},
			bson.DocElem{Name: "privileges", Value: []bson.D{
				{
					bson.DocElem{Name: "resource", Value: bson.M{"anyResource": true}},
					bson.DocElem{Name: "actions", Value: []string{"anyAction"}}}}},
			bson.DocElem{Name: "roles", Value: []string{}}},
		{
			bson.DocElem{Name: "grantRolesToUser", Value: "fakeUsername"},
			bson.DocElem{Name: "roles", Value: []string{"oploger"}}},
		{
			bson.DocElem{Name: "grantRolesToUser", Value: "admin"},
			bson.DocElem{Name: "roles", Value: []string{"oploger"}}}}
	c.Assert(mgoSession.cmd, gc.DeepEquals, mgoSessionCmd)
}

func (s *mongoRestoreSuite) TestRestoreDatabase32(c *gc.C) {
	s.assertRestore(c)
}

func (s *mongoRestoreSuite) TestRestoreIdempotent(c *gc.C) {
	s.assertRestore(c)
	// Run a 2nd time, lp:1740969
	s.assertRestore(c)
}

func (s *mongoRestoreSuite) TestRestoreFailsOnOlderMongo(c *gc.C) {
	s.PatchValue(backups.GetMongorestorePath, func() (string, error) { return "/a/fake/mongorestore", nil })
	args := backups.RestorerArgs{
		DialInfo: &mgo.DialInfo{
			Username: "fakeUsername",
			Password: "fakePassword",
			Addrs:    []string{"127.0.0.1"},
		},
		Version:         mongo.Mongo32wt,
		TagUser:         "machine-0",
		TagUserPassword: "fakePassword",
	}
	s.PatchValue(backups.MongoInstalledVersion, func() mongo.Version { return mongo.Mongo24 })
	_, err := backups.NewDBRestorer(args)
	c.Assert(err, gc.ErrorMatches, "restore mongo version 3.2/wiredTiger into version 2.4/mmapv1 not supported")
}
