// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

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
	closed bool
	cmd    []bson.D
}

func (m *mongoSession) Run(cmd interface{}, result interface{}) error {
	bsoncmd, ok := cmd.(bson.D)
	if !ok {
		return errors.New("unexpected cmd")
	}
	m.cmd = append(m.cmd, bsoncmd)
	return nil
}

func (m *mongoSession) Close() {
	m.closed = true
}

func (m *mongoSession) DB(_ string) *mgo.Database {
	return nil
}

func (s *mongoRestoreSuite) TestRestoreDatabase32(c *gc.C) {
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
	c.Assert(ranWithArgs, gc.DeepEquals, []string{"--ssl", "--authenticationDatabase", "admin", "--host", "127.0.0.1", "--username", "fakeUsername", "--password", "fakePassword", "--drop", "--oplogReplay", "--batchSize", "10", "fakePath"})
	user := &mgo.User{Username: "machine-0", Password: "fakePassword"}
	c.Assert(mgoDb.user, gc.DeepEquals, user)
	c.Assert(mgoSession.closed, jc.IsTrue)
	mgoSessionCmd := []bson.D{
		bson.D{
			bson.DocElem{Name: "createRole", Value: "oploger"},
			bson.DocElem{Name: "privileges", Value: []bson.D{
				bson.D{
					bson.DocElem{Name: "resource", Value: bson.M{"anyResource": true}},
					bson.DocElem{Name: "actions", Value: []string{"anyAction"}}}}},
			bson.DocElem{Name: "roles", Value: []string{}}},
		bson.D{
			bson.DocElem{Name: "grantRolesToUser", Value: "fakeUsername"},
			bson.DocElem{Name: "roles", Value: []string{"oploger"}}},
		bson.D{
			bson.DocElem{Name: "grantRolesToUser", Value: "admin"},
			bson.DocElem{Name: "roles", Value: []string{"oploger"}}}}
	c.Assert(mgoSession.cmd, gc.DeepEquals, mgoSessionCmd)
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
