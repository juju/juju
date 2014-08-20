// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

type upgradesSuite struct {
	testing.BaseSuite
	gitjujutesting.MgoSuite
	state *State
}

func (s *upgradesSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *upgradesSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *upgradesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	var err error
	s.state, err = Initialize(TestingMongoInfo(), testing.EnvironConfig(c), TestingDialOpts(), Policy(nil))
	c.Assert(err, gc.IsNil)
}

func (s *upgradesSuite) TearDownTest(c *gc.C) {
	s.state.Close()
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

var _ = gc.Suite(&upgradesSuite{})

func (s *upgradesSuite) TestLastLoginMigrate(c *gc.C) {
	now := time.Now().UTC().Round(time.Second)
	userId := "foobar"
	oldDoc := bson.M{
		"_id_":           userId,
		"_id":            userId,
		"displayname":    "foo bar",
		"deactivated":    false,
		"passwordhash":   "hash",
		"passwordsalt":   "salt",
		"createdby":      "creator",
		"datecreated":    now,
		"lastconnection": now,
	}

	ops := []txn.Op{
		txn.Op{
			C:      "users",
			Id:     userId,
			Assert: txn.DocMissing,
			Insert: oldDoc,
		},
	}
	err := s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)

	err = MigrateUserLastConnectionToLastLogin(s.state)
	c.Assert(err, gc.IsNil)
	user, err := s.state.User(userId)
	c.Assert(err, gc.IsNil)
	c.Assert(*user.LastLogin(), gc.Equals, now)

	// check to see if _id_ field is removed
	userMap := map[string]interface{}{}
	users, closer := s.state.getCollection("users")
	defer closer()
	err = users.Find(bson.D{{"_id", userId}}).One(&userMap)
	c.Assert(err, gc.IsNil)
	_, keyExists := userMap["_id_"]
	c.Assert(keyExists, jc.IsFalse)
}

func (s *upgradesSuite) TestAddStateUsersToEnviron(c *gc.C) {
	adminTag := s.createUser(c, "admin", "admin")
	bobTag := s.createUser(c, "bob", "admin")

	_, err := s.state.EnvironmentUser(adminTag)
	c.Assert(err, gc.ErrorMatches, `envUser "admin@local" not found`)
	_, err = s.state.EnvironmentUser(bobTag)
	c.Assert(err, gc.ErrorMatches, `envUser "bob@local" not found`)

	err = AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, gc.IsNil)

	admin, err := s.state.EnvironmentUser(adminTag)
	c.Assert(err, gc.IsNil)
	c.Assert(admin.UserTag().Username(), gc.DeepEquals, adminTag.Username())
	bob, err := s.state.EnvironmentUser(bobTag)
	c.Assert(err, gc.IsNil)
	c.Assert(bob.UserTag().Username(), gc.DeepEquals, bobTag.Username())
}

func (s *upgradesSuite) TestAddStateUsersToEnvironIdempotent(c *gc.C) {
	s.createUser(c, "admin", "admin")
	s.createUser(c, "bob", "admin")

	err := AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, gc.IsNil)

	err = AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, gc.IsNil)
}

func (s *upgradesSuite) createUser(c *gc.C, userID, creatorID string) names.UserTag {
	now := time.Now().UTC().Round(time.Second)
	uDoc := bson.M{
		"_id_":           userID,
		"_id":            userID,
		"displayname":    "admin",
		"deactivated":    false,
		"passwordhash":   "hash",
		"passwordsalt":   "salt",
		"createdby":      creatorID,
		"datecreated":    now,
		"lastconnection": now,
	}

	ops := []txn.Op{
		txn.Op{
			C:      "users",
			Id:     userID,
			Assert: txn.DocMissing,
			Insert: uDoc,
		},
	}

	err := s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)

	return names.NewUserTag(userID)
}

func (s *upgradesSuite) TestAddEnvironmentUUIDToStateServerDoc(c *gc.C) {
	info, err := s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
	uuid := info.EnvUUID

	// force remove the uuid.
	ops := []txn.Op{{
		C:      stateServersC,
		Id:     environGlobalKey,
		Assert: txn.DocExists,
		Update: bson.D{{"$unset", bson.D{
			{"env-uuid", nil},
		}}},
	}}
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)
	// Make sure it has gone.
	info, err = s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(info.EnvUUID, gc.Equals, "")

	// Run the upgrade step
	err = AddEnvironmentUUIDToStateServerDoc(s.state)
	c.Assert(err, gc.IsNil)
	// Make sure it is there now
	info, err = s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(info.EnvUUID, gc.Equals, uuid)
}

func (s *upgradesSuite) TestAddEnvironmentUUIDToStateServerDocIdempotent(c *gc.C) {
	info, err := s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
	uuid := info.EnvUUID

	err = AddEnvironmentUUIDToStateServerDoc(s.state)
	c.Assert(err, gc.IsNil)

	info, err = s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(info.EnvUUID, gc.Equals, uuid)
}
