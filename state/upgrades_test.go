// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testcharms"
)

type upgradesSuite struct {
	internalStateSuite
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
		{
			C:      "users",
			Id:     userId,
			Assert: txn.DocMissing,
			Insert: oldDoc,
		},
	}
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	err = MigrateUserLastConnectionToLastLogin(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// check to see if _id_ field is removed
	userMap := map[string]interface{}{}
	users, closer := s.state.getRawCollection("users")
	defer closer()
	err = users.Find(bson.D{{"_id", userId}}).One(&userMap)
	c.Assert(err, jc.ErrorIsNil)
	_, keyExists := userMap["_id_"]
	c.Assert(keyExists, jc.IsFalse)

	lastLogin, keyExists := userMap["lastlogin"]
	c.Assert(keyExists, jc.IsTrue)
	c.Assert(now.Equal(lastLogin.(time.Time)), jc.IsTrue)
}

func (s *upgradesSuite) TestLowerCaseEnvUsersID(c *gc.C) {
	UUID := s.state.EnvironUUID()
	s.addCaseSensitiveEnvUsers(c, UUID, [][]string{
		{"BoB@local", "Bob the Builder"},
		{"sAm@cRAZyCaSe", "Sam Smith"},
		{"adam@alllower", "Adam Apple"},
	})

	err := LowerCaseEnvUsersID(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvUserIDLowerCased(c, [][]string{
		{UUID, "bob@local", "BoB@local", "Bob the Builder"},
		{UUID, "adam@alllower", "adam@alllower", "Adam Apple"},
		{UUID, "sam@crazycase", "sAm@cRAZyCaSe", "Sam Smith"},
		{UUID, "test-admin@local", "test-admin@local", "test-admin"},
	})
}

func (s *upgradesSuite) TestDupeCaseSensitive(c *gc.C) {
	UUID := s.state.EnvironUUID()
	s.addCaseSensitiveEnvUsers(c, UUID, [][]string{
		{"BoB@local", "Bob the Builder"},
		{"bob@local", "Bobby Brown"},
	})

	err := LowerCaseEnvUsersID(s.state)
	// Yes, this means the upgrade step fails if there are two existing users
	// with the same username but different case.
	c.Assert(err, gc.ErrorMatches, "transaction aborted")
}

func (s *upgradesSuite) TestLowerCaseEnvUsersIDMultiEnvs(c *gc.C) {
	UUID1 := "6983ac70-b0aa-45c5-80fe-9f207bbb18d9"
	s.addCaseSensitiveEnvUsers(c, UUID1, [][]string{
		{"BoB@local", "Bob the Builder"},
		{"sAm@cRAZyCaSe", "Sam Smith"},
		{"adam@alllower", "Adam Apple"},
	})

	UUID2 := "7983ac70-b0aa-45c5-80fe-9f207bbb18d9"
	s.addCaseSensitiveEnvUsers(c, UUID2, [][]string{
		{"BoB@local", "Bob the Builder"},
		{"Joe@Yo", "Joe Yo"},
		{"adam@alllower", "Young Apple"},
	})

	err := LowerCaseEnvUsersID(s.state)
	c.Assert(err, jc.ErrorIsNil)

	stUUID := s.state.EnvironUUID()
	s.assertEnvUserIDLowerCased(c, [][]string{
		{UUID1, "bob@local", "BoB@local", "Bob the Builder"},
		{UUID2, "bob@local", "BoB@local", "Bob the Builder"},
		{UUID2, "joe@yo", "Joe@Yo", "Joe Yo"},
		{UUID1, "adam@alllower", "adam@alllower", "Adam Apple"},
		{UUID2, "adam@alllower", "adam@alllower", "Young Apple"},
		{UUID1, "sam@crazycase", "sAm@cRAZyCaSe", "Sam Smith"},
		{stUUID, "test-admin@local", "test-admin@local", "test-admin"},
	})
}

func (s *upgradesSuite) TestLowerCaseEnvUsersIDIdempotent(c *gc.C) {
	UUID := s.state.EnvironUUID()
	s.addCaseSensitiveEnvUsers(c, UUID, [][]string{
		{"BoB@local", "Bob the Builder"},
		{"sAm@cRAZyCaSe", "Sam Smith"},
		{"adam@alllower", "Adam Apple"},
	})

	err := LowerCaseEnvUsersID(s.state)
	c.Assert(err, jc.ErrorIsNil)
	err = LowerCaseEnvUsersID(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.assertEnvUserIDLowerCased(c, [][]string{
		{UUID, "bob@local", "BoB@local", "Bob the Builder"},
		{UUID, "adam@alllower", "adam@alllower", "Adam Apple"},
		{UUID, "sam@crazycase", "sAm@cRAZyCaSe", "Sam Smith"},
		{UUID, "test-admin@local", "test-admin@local", "test-admin"},
	})
}

// addCaseSensitiveEnvUsers adds an envUserDoc with a case sensitive "_id" for
// each {"_id", "displayname"} pair passed in.
func (s *upgradesSuite) addCaseSensitiveEnvUsers(c *gc.C, envUUID string, oldUsers [][]string) {
	c.Assert(utils.IsValidUUIDString(envUUID), jc.IsTrue)
	var ops []txn.Op
	for _, oldUser := range oldUsers {
		ops = append(ops, txn.Op{
			C:  envUsersC,
			Id: envUUID + ":" + oldUser[0],
			Insert: bson.D{
				{"user", oldUser[0]},
				{"displayname", oldUser[1]},
			}})
	}
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

// assertEnvUserIDLowerCased asserts across all environments that the
// <localID> part of each envUser's _id field, which has the format
// "<uuid>:<localID>", matches the respective expected lower-cased id.
func (s *upgradesSuite) assertEnvUserIDLowerCased(c *gc.C, expected [][]string) {
	users, closer := s.state.getRawCollection(envUsersC)
	defer closer()

	var obtained []bson.M
	err := users.Find(nil).Sort("user").All(&obtained)
	c.Assert(err, jc.ErrorIsNil)

	for i, expectedUser := range expected {
		ID := obtained[i]["_id"].(string)
		parts := strings.Split(ID, ":")
		c.Assert(parts, gc.HasLen, 2)
		UUID, name := parts[0], parts[1]

		c.Assert(UUID, gc.Equals, expectedUser[0])
		c.Assert(name, gc.Equals, expectedUser[1])
		c.Assert(obtained[i]["_id"], gc.Equals, expectedUser[0]+":"+expectedUser[1])
		c.Assert(obtained[i]["user"], gc.Equals, expectedUser[2])
		c.Assert(obtained[i]["displayname"], gc.Equals, expectedUser[3])
	}
	c.Assert(obtained, gc.HasLen, len(expected))
}

func (s *upgradesSuite) TestUserTagNameFallsBackToId(c *gc.C) {
	// Make old style user without name field set.
	user := User{
		st: s.state,
		doc: userDoc{
			DocID: "BoB",
		},
	}

	tag := user.UserTag()
	c.Assert(tag.Name(), gc.Equals, "BoB")
}

func (s *upgradesSuite) TestAddNameFieldLowerCaseIdOfUsers(c *gc.C) {
	s.addCaseSensitiveUsers(c, [][]string{
		{"BoB", "Bob the Builder"},
		{"sAm", "Sam Smith"},
		{"adam", "Adam Apple"},
	})

	err := AddNameFieldLowerCaseIdOfUsers(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.assertNameAddedIdLowerCased(c, [][]string{
		{"adam", "adam", "Adam Apple"},
		{"bob", "BoB", "Bob the Builder"},
		{"sam", "sAm", "Sam Smith"},
		{"test-admin", "test-admin", "test-admin"},
	})
}

func (s *upgradesSuite) TestAddNameFieldLowerCaseIdOfUsersIdempotent(c *gc.C) {
	s.addCaseSensitiveUsers(c, [][]string{
		{"BoB", "Bob the Builder"},
		{"sAm", "Sam Smith"},
	})

	err := AddNameFieldLowerCaseIdOfUsers(s.state)
	c.Assert(err, jc.ErrorIsNil)
	err = AddNameFieldLowerCaseIdOfUsers(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.assertNameAddedIdLowerCased(c, [][]string{
		{"bob", "BoB", "Bob the Builder"},
		{"sam", "sAm", "Sam Smith"},
		{"test-admin", "test-admin", "test-admin"},
	})
}

// addCaseSensitiveUsers adds a userDoc with a case sensitive "_id" for each
// {"_id", "displayname"} pair passed in.
func (s *upgradesSuite) addCaseSensitiveUsers(c *gc.C, oldUsers [][]string) {
	var ops []txn.Op
	for _, oldUser := range oldUsers {
		ops = append(ops, txn.Op{
			C:  usersC,
			Id: oldUser[0],
			Insert: bson.D{
				{"displayname", oldUser[1]},
			},
		})
	}
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

// assertNameAddedIdLowerCased asserts that all users in the usersC collection
// have the expected lower case _id and case preserved name.
func (s *upgradesSuite) assertNameAddedIdLowerCased(c *gc.C, expected [][]string) {
	users, closer := s.state.getCollection("users")
	defer closer()

	var obtained []bson.M
	err := users.Find(nil).Sort("_id").All(&obtained)
	c.Assert(err, jc.ErrorIsNil)

	for i, expectedUser := range expected {
		c.Assert(obtained[i]["_id"], gc.Equals, expectedUser[0])
		c.Assert(obtained[i]["name"], gc.Equals, expectedUser[1])
		c.Assert(obtained[i]["displayname"], gc.Equals, expectedUser[2])
	}
	c.Assert(len(obtained), gc.Equals, len(expected))
}

func (s *upgradesSuite) TestAddUniqueOwnerEnvNameForEnvirons(c *gc.C) {
	s.userEnvNameSetup(c, [][]string{
		{"bob", "bobsenv"},
		{"sam@remote", "samsenv"},
	})

	err := AddUniqueOwnerEnvNameForEnvirons(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.assertUserEnvNameObtained(c,
		"test-admin@local:testenv",
		"bob@local:bobsenv",
		"sam@remote:samsenv",
	)
}

func (s *upgradesSuite) TestAddUniqueOwnerEnvNameForEnvironsErrors(c *gc.C) {
	s.userEnvNameSetup(c, [][]string{
		{"bob", "bobsenv"},
		{"bob", "bobsenv"},
	})

	err := AddUniqueOwnerEnvNameForEnvirons(s.state)
	c.Assert(err, gc.ErrorMatches, `environment "bobsenv" for bob@local already exists`)
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)

	// we expect the userenvname doc for the server environ as it was inserted
	// when the environment was initialized.
	s.assertUserEnvNameObtained(c, "test-admin@local:testenv")
}

func (s *upgradesSuite) TestAddUniqueOwnerEnvNameForEnvironsIdempotent(c *gc.C) {
	s.userEnvNameSetup(c, [][]string{
		{"bob", "bobsenv"},
		{"sam@remote", "samsenv"},
	})

	err := AddUniqueOwnerEnvNameForEnvirons(s.state)
	c.Assert(err, jc.ErrorIsNil)
	err = AddUniqueOwnerEnvNameForEnvirons(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.assertUserEnvNameObtained(c,
		"bob@local:bobsenv",
		"sam@remote:samsenv",
		"test-admin@local:testenv",
	)
}

// userEnvNameSetup adds an environmentsC doc for each {"owner", "envName"} arg.
func (s *upgradesSuite) userEnvNameSetup(c *gc.C, userEnvNamePairs [][]string) {
	var ops []txn.Op
	for _, userEnvNamePair := range userEnvNamePairs {
		uuid, err := utils.NewUUID()
		c.Assert(err, jc.ErrorIsNil)
		ops = append(ops, createEnvironmentOp(
			s.state,
			names.NewUserTag(userEnvNamePair[0]),
			userEnvNamePair[1],
			uuid.String(),
			s.state.EnvironUUID(),
		))
	}
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

// assertUserEnvNameObtained asserts that all (and no more) expectedIds are found in
// the userenvnameC collection.
func (s *upgradesSuite) assertUserEnvNameObtained(c *gc.C, expectedIds ...string) {
	var obtained []bson.M
	userenvname, closer := s.state.getCollection(userenvnameC)
	defer closer()
	err := userenvname.Find(nil).All(&obtained)
	c.Assert(err, jc.ErrorIsNil)

	var obtainedIds []string
	for _, userEnvName := range obtained {
		obtainedIds = append(obtainedIds, userEnvName["_id"].(string))
	}
	c.Assert(obtainedIds, jc.SameContents, expectedIds)
}

func (s *upgradesSuite) TestAddStateUsersToEnviron(c *gc.C) {
	stateBob, err := s.state.AddUser("bob", "notused", "notused", "bob")
	c.Assert(err, jc.ErrorIsNil)
	bobTag := stateBob.UserTag()

	_, err = s.state.EnvironmentUser(bobTag)
	c.Assert(err, gc.ErrorMatches, `environment user "bob@local" not found`)

	err = AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, jc.ErrorIsNil)

	admin, err := s.state.EnvironmentUser(s.owner)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(admin.UserTag().Canonical(), gc.DeepEquals, s.owner.Canonical())
	bob, err := s.state.EnvironmentUser(bobTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bob.UserTag().Canonical(), gc.DeepEquals, bobTag.Canonical())
}

func (s *upgradesSuite) TestAddStateUsersToEnvironIdempotent(c *gc.C) {
	stateBob, err := s.state.AddUser("bob", "notused", "notused", "bob")
	c.Assert(err, jc.ErrorIsNil)
	bobTag := stateBob.UserTag()

	err = AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, jc.ErrorIsNil)

	err = AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, jc.ErrorIsNil)

	admin, err := s.state.EnvironmentUser(s.owner)
	c.Assert(admin.UserTag().Canonical(), gc.DeepEquals, s.owner.Canonical())
	bob, err := s.state.EnvironmentUser(bobTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bob.UserTag().Canonical(), gc.DeepEquals, bobTag.Canonical())
}

func (s *upgradesSuite) TestAddEnvironmentUUIDToStateServerDoc(c *gc.C) {
	info, err := s.state.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	tag := info.EnvironmentTag

	// force remove the uuid.
	ops := []txn.Op{{
		C:      stateServersC,
		Id:     environGlobalKey,
		Assert: txn.DocExists,
		Update: bson.D{{"$unset", bson.D{
			{"env-uuid", nil},
		}}},
	}}
	err = s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
	// Make sure it has gone.
	stateServers, closer := s.state.getRawCollection(stateServersC)
	defer closer()
	var doc stateServersDoc
	err = stateServers.Find(bson.D{{"_id", environGlobalKey}}).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doc.EnvUUID, gc.Equals, "")

	// Run the upgrade step
	err = AddEnvironmentUUIDToStateServerDoc(s.state)
	c.Assert(err, jc.ErrorIsNil)
	// Make sure it is there now
	info, err = s.state.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.EnvironmentTag, gc.Equals, tag)
}

func (s *upgradesSuite) TestAddEnvironmentUUIDToStateServerDocIdempotent(c *gc.C) {
	info, err := s.state.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	tag := info.EnvironmentTag

	err = AddEnvironmentUUIDToStateServerDoc(s.state)
	c.Assert(err, jc.ErrorIsNil)

	info, err = s.state.StateServerInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.EnvironmentTag, gc.Equals, tag)
}

func (s *upgradesSuite) TestAddCharmStoragePaths(c *gc.C) {
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)

	bundleSHA256 := "dummy-1-sha256"
	dummyCharm, err := s.state.AddCharm(ch, curl, "", bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)
	SetCharmBundleURL(c, s.state, curl, "http://anywhere.com")
	dummyCharm, err = s.state.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummyCharm.BundleURL(), gc.NotNil)
	c.Assert(dummyCharm.BundleURL().String(), gc.Equals, "http://anywhere.com")
	c.Assert(dummyCharm.StoragePath(), gc.Equals, "")

	storagePaths := map[*charm.URL]string{curl: "/some/where"}
	err = AddCharmStoragePaths(s.state, storagePaths)
	c.Assert(err, jc.ErrorIsNil)

	dummyCharm, err = s.state.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummyCharm.BundleURL(), gc.IsNil)
	c.Assert(dummyCharm.StoragePath(), gc.Equals, "/some/where")
}

func (s *upgradesSuite) TestEnvUUIDMigrationWithIdAlreadyPrefixed(c *gc.C) {
	s.checkEnvUUID(c, AddEnvUUIDToServices, servicesC,
		[]bson.M{
			{"_id": s.state.docID("mysql")},
			{"_id": s.state.docID("mediawiki")},
		}, false)
}

func (s *upgradesSuite) TestAddEnvUUIDToServices(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToServices, servicesC,
		bson.M{
			"_id":    "mysql",
			"series": "quantal",
			"life":   Dead,
		},
		bson.M{
			"_id":    "mediawiki",
			"series": "precise",
			"life":   Alive,
		},
	)

	var newDoc serviceDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Name, gc.Equals, "mysql")
	c.Assert(newDoc.Series, gc.Equals, "quantal")
	c.Assert(newDoc.Life, gc.Equals, Dead)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Name, gc.Equals, "mediawiki")
	c.Assert(newDoc.Series, gc.Equals, "precise")
	c.Assert(newDoc.Life, gc.Equals, Alive)
}

func (s *upgradesSuite) TestAddEnvUUIDToServicesIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToServices, servicesC)
}

func (s *upgradesSuite) TestAddEnvUUIDToUnits(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToUnits, unitsC,
		bson.M{
			"_id":    "mysql/0",
			"series": "trusty",
			"life":   Alive,
		},
		bson.M{
			"_id":    "nounforge/0",
			"series": "utopic",
			"life":   Dead,
		},
	)

	var newDoc unitDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Name, gc.Equals, "mysql/0")
	c.Assert(newDoc.Series, gc.Equals, "trusty")
	c.Assert(newDoc.Life, gc.Equals, Alive)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Name, gc.Equals, "nounforge/0")
	c.Assert(newDoc.Series, gc.Equals, "utopic")
	c.Assert(newDoc.Life, gc.Equals, Dead)
}

func (s *upgradesSuite) TestAddEnvUUIDToUnitsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToUnits, unitsC)
}

func (s *upgradesSuite) TestAddEnvUUIDToEnvUsers(c *gc.C) {
	uuid := s.state.EnvironUUID()
	coll, newIDs, count := s.checkEnvUUID(c, AddEnvUUIDToEnvUsersDoc, envUsersC,
		[]bson.M{{
			"_id":         uuid + ":sam@local",
			"createdby":   "test-admin@local",
			"displayname": "sam",
			"envuuid":     uuid,
			"user":        "sam@local",
		}, {
			"_id":         uuid + ":ralph@local",
			"createdby":   "test-admin@local",
			"displayname": "ralph",
			"envuuid":     uuid,
			"user":        "ralph@local",
		}},
		false,
	)
	// This test expects 3 docs to account for the test-admin user doc.
	c.Assert(count, gc.Equals, 3)

	var newDoc envUserDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.UserName, gc.Equals, "sam@local")
	c.Assert(newDoc.CreatedBy, gc.Equals, "test-admin@local")
	c.Assert(newDoc.DisplayName, gc.Equals, "sam")

	var newBsonDoc bson.M
	s.FindId(c, coll, newIDs[0], &newBsonDoc)
	_, ok := newBsonDoc["envuuid"]
	c.Assert(ok, jc.IsFalse)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.UserName, gc.Equals, "ralph@local")
	c.Assert(newDoc.CreatedBy, gc.Equals, "test-admin@local")
	c.Assert(newDoc.DisplayName, gc.Equals, "ralph")

	s.FindId(c, coll, newIDs[1], &newBsonDoc)
	_, ok = newBsonDoc["envuuid"]
	c.Assert(ok, jc.IsFalse)
}

func (s *upgradesSuite) TestAddEnvUUIDToEnvUsersIdempotent(c *gc.C) {
	uuid := s.state.EnvironUUID()
	oldID := uuid + ":bob@local"

	s.addLegacyDoc(c, envUsersC, bson.M{"_id": oldID, "envuuid": uuid})
	err := AddEnvUUIDToEnvUsersDoc(s.state)
	c.Assert(err, jc.ErrorIsNil)
	err = AddEnvUUIDToEnvUsersDoc(s.state)
	c.Assert(err, jc.ErrorIsNil)
	coll, closer := s.state.getRawCollection(envUsersC)
	defer closer()

	var docs []map[string]string
	err = coll.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(docs, gc.HasLen, 2)
	c.Assert(docs[0]["user"], gc.Equals, "test-admin@local")
	c.Assert(docs[1]["_id"], gc.Equals, oldID)
	c.Assert(docs[1]["env-uuid"], gc.Equals, uuid)
	if oldUuid, ok := docs[1]["envuuid"]; ok {
		c.Fatalf("expected nil found %q", oldUuid)
	}
}

func (s *upgradesSuite) TestAddEnvUUIDToMachines(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToMachines, machinesC,
		bson.M{
			"_id":    "0",
			"series": "trusty",
			"life":   Alive,
		},
		bson.M{
			"_id":    "1",
			"series": "utopic",
			"life":   Dead,
		},
	)

	var newDoc machineDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Id, gc.Equals, "0")
	c.Assert(newDoc.Series, gc.Equals, "trusty")
	c.Assert(newDoc.Life, gc.Equals, Alive)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Id, gc.Equals, "1")
	c.Assert(newDoc.Series, gc.Equals, "utopic")
	c.Assert(newDoc.Life, gc.Equals, Dead)
}

func (s *upgradesSuite) TestAddEnvUUIDToMachinesIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToMachines, machinesC)
}

func (s *upgradesSuite) TestAddEnvUUIDToOpenPorts(c *gc.C) {
	range1 := []PortRange{{
		FromPort: 100,
		ToPort:   200,
		UnitName: "wordpress/0",
		Protocol: "TCP",
	}, {
		FromPort: 300,
		ToPort:   400,
		UnitName: "mysql/1",
		Protocol: "UDP",
	}}

	range2 := []PortRange{{
		FromPort: 800,
		ToPort:   900,
		UnitName: "ghost/1",
		Protocol: "UDP",
	}, {
		FromPort: 500,
		ToPort:   600,
		UnitName: "mongo/0",
		Protocol: "TCP",
	}}

	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToOpenPorts, openedPortsC,
		bson.M{
			"_id":   "m#2#n#juju-public",
			"ports": range1,
		},
		bson.M{
			"_id":   "m#1#n#net3",
			"ports": range2,
		},
	)

	var newDoc portsDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	p := Ports{s.state, newDoc, false}
	c.Assert(p.GlobalKey(), gc.Equals, "m#2#n#juju-public")
	c.Assert(newDoc.Ports, gc.DeepEquals, range1)
	c.Assert(newDoc.MachineID, gc.Equals, "2")
	c.Assert(newDoc.NetworkName, gc.Equals, "juju-public")

	s.FindId(c, coll, newIDs[1], &newDoc)
	p = Ports{s.state, newDoc, false}
	c.Assert(p.GlobalKey(), gc.Equals, "m#1#n#net3")
	c.Assert(newDoc.Ports, gc.DeepEquals, range2)
	c.Assert(newDoc.MachineID, gc.Equals, "1")
	c.Assert(newDoc.NetworkName, gc.Equals, "net3")
}

func (s *upgradesSuite) TestAddEnvUUIDToOpenPortsIdempotent(c *gc.C) {
	oldID := "m#0#n#net1"
	docs := s.checkEnvUUIDIdempotent(c, oldID, AddEnvUUIDToOpenPorts, openedPortsC)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0]["_id"], gc.Equals, s.state.docID(fmt.Sprint(oldID)))
}

func (s *upgradesSuite) TestAddEnvUUIDToAnnotations(c *gc.C) {
	annotations := map[string]string{"foo": "bar", "arble": "baz"}
	annotations2 := map[string]string{"foo": "bar", "arble": "baz"}
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToAnnotations, annotationsC,
		bson.M{
			"_id":         "m#0",
			"tag":         "machine-0",
			"annotations": annotations,
		},
		bson.M{
			"_id":         "m#1",
			"tag":         "machine-1",
			"annotations": annotations2,
		},
	)

	var newDoc annotatorDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.GlobalKey, gc.Equals, "m#0")
	c.Assert(newDoc.Tag, gc.Equals, "machine-0")
	c.Assert(newDoc.Annotations, gc.DeepEquals, annotations)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.GlobalKey, gc.Equals, "m#1")
	c.Assert(newDoc.Tag, gc.Equals, "machine-1")
	c.Assert(newDoc.Annotations, gc.DeepEquals, annotations2)
}

func (s *upgradesSuite) TestAddEnvUUIDToAnnotationsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToAnnotations, annotationsC)
}

func (s *upgradesSuite) TestAddEnvUUIDToNetworks(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToNetworks, networksC,
		bson.M{
			"_id":        "net1",
			"providerid": "net1",
			"cidr":       "0.1.2.0/24",
		},
		bson.M{
			"_id":        "net2",
			"providerid": "net2",
			"cidr":       "0.2.2.0/24",
		},
	)

	var newDoc networkDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(network.Id(newDoc.ProviderId), gc.Equals, network.Id("net1"))
	c.Assert(newDoc.CIDR, gc.Equals, "0.1.2.0/24")

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(network.Id(newDoc.ProviderId), gc.Equals, network.Id("net2"))
	c.Assert(newDoc.CIDR, gc.Equals, "0.2.2.0/24")
}

func (s *upgradesSuite) TestAddEnvUUIDToNetworksIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToNetworks, networksC)
}

func (s *upgradesSuite) TestAddEnvUUIDToRequestedNetworks(c *gc.C) {
	reqNetworks1 := []string{"net1", "net2"}
	reqNetworks2 := []string{"net3", "net4"}
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToRequestedNetworks, requestedNetworksC,
		bson.M{
			"_id":      "0",
			"networks": reqNetworks1,
		},
		bson.M{
			"_id":      "1",
			"networks": reqNetworks2,
		},
	)

	var newDoc requestedNetworksDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Networks, gc.DeepEquals, reqNetworks1)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Networks, gc.DeepEquals, reqNetworks2)
}

func (s *upgradesSuite) TestAddEnvUUIDToRequestedNetworksIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToRequestedNetworks, requestedNetworksC)
}

func (s *upgradesSuite) TestAddEnvUUIDToNetworkInterfaces(c *gc.C) {
	coll, newIDs, count := s.checkEnvUUID(c, AddEnvUUIDToNetworkInterfaces, networkInterfacesC,
		[]bson.M{
			{
				"_id":         bson.NewObjectId(),
				"machineid":   "2",
				"networkname": "net1",
			}, {

				"_id":         bson.NewObjectId(),
				"machineid":   "4",
				"networkname": "net2",
			}},
		false)
	c.Assert(count, gc.Equals, 2)

	var newDoc networkInterfaceDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.NetworkName, gc.Equals, "net1")
	c.Assert(newDoc.MachineId, gc.Equals, "2")

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.NetworkName, gc.Equals, "net2")
	c.Assert(newDoc.MachineId, gc.Equals, "4")
}

func (s *upgradesSuite) TestAddEnvUUIDToNetworkInterfacesIdempotent(c *gc.C) {
	oldID := "foo"
	docs := s.checkEnvUUIDIdempotent(c, oldID, AddEnvUUIDToNetworkInterfaces, networkInterfacesC)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0]["_id"], gc.Equals, oldID)
	c.Assert(docs[0]["env-uuid"], gc.Equals, s.state.EnvironUUID())
}

func (s *upgradesSuite) TestAddEnvUUIDToMinUnits(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToMinUnits, minUnitsC,
		bson.M{
			"_id":   "wordpress",
			"revno": 1,
		},
		bson.M{
			"_id":   "mediawiki",
			"revno": 2,
		},
	)

	var newDoc minUnitsDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.ServiceName, gc.Equals, "wordpress")
	c.Assert(newDoc.Revno, gc.Equals, 1)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.ServiceName, gc.Equals, "mediawiki")
	c.Assert(newDoc.Revno, gc.Equals, 2)
}

func (s *upgradesSuite) TestAddEnvUUIDToMinUnitsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToMinUnits, minUnitsC)
}

func (s *upgradesSuite) TestAddEnvUUIDToCleanups(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToCleanups, cleanupsC,
		bson.M{
			"_id":    bson.NewObjectId(),
			"kind":   "units",
			"prefix": "mysql",
		},
		bson.M{
			"_id":    bson.NewObjectId(),
			"kind":   "service",
			"prefix": "mediawiki",
		},
	)

	var newDoc cleanupDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(string(newDoc.Kind), gc.Equals, "units")

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(string(newDoc.Kind), gc.Equals, "service")
}

func (s *upgradesSuite) TestAddEnvUUIDToCleanupsIdempotent(c *gc.C) {
	oldID := bson.NewObjectId()
	docs := s.checkEnvUUIDIdempotent(c, oldID, AddEnvUUIDToCleanups, cleanupsC)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0]["_id"], gc.Equals, s.state.docID(fmt.Sprint(oldID)))
}

func (s *upgradesSuite) TestAddEnvUUIDToConstraints(c *gc.C) {
	networks1 := []string{"net1", "net2"}
	networks2 := []string{"net3", "net4"}
	coll, newIDs, count := s.checkEnvUUID(c, AddEnvUUIDToConstraints, constraintsC,
		[]bson.M{
			{
				"_id":      "s#wordpress",
				"cpucores": 4,
				"networks": networks1,
			},
			{
				"_id":      "s#mediawiki",
				"cpucores": 8,
				"networks": networks2,
			},
		},
		true)
	// The test expects three records because there is a preexisting
	// environment constraints doc in mongo.
	c.Assert(count, gc.Equals, 3)

	var newDoc constraintsDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(*newDoc.CpuCores, gc.Equals, uint64(4))
	c.Assert(*newDoc.Networks, jc.DeepEquals, networks1)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(*newDoc.CpuCores, gc.Equals, uint64(8))
	c.Assert(*newDoc.Networks, jc.DeepEquals, networks2)
}

func (s *upgradesSuite) TestAddEnvUUIDToConstraintsIdempotent(c *gc.C) {
	oldID := "s#ghost"
	docs := s.checkEnvUUIDIdempotent(c, oldID, AddEnvUUIDToConstraints, constraintsC)
	c.Assert(docs, gc.HasLen, 2)
	c.Assert(docs[0]["_id"], gc.Equals, s.state.docID(fmt.Sprint("e")))
	c.Assert(docs[1]["_id"], gc.Equals, s.state.docID(fmt.Sprint(oldID)))
}

func (s *upgradesSuite) TestAddEnvUUIDToStatuses(c *gc.C) {
	statusData := map[string]interface{}{
		"1st-key": "one",
		"2nd-key": 2,
		"3rd-key": true,
	}

	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToStatuses, statusesC,
		bson.M{
			"_id":    "u#wordpress/0",
			"status": StatusActive,
		},
		bson.M{
			"_id":        "m#0",
			"status":     StatusError,
			"statusdata": statusData,
		},
	)

	var newDoc statusDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Status, gc.Equals, StatusActive)
	c.Assert(newDoc.StatusData, gc.IsNil)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Status, gc.Equals, StatusError)
	c.Assert(newDoc.StatusData, gc.DeepEquals, statusData)
}

func (s *upgradesSuite) TestAddEnvUUIDToStatusesIdempotent(c *gc.C) {
	oldID := "m#1"
	docs := s.checkEnvUUIDIdempotent(c, oldID, AddEnvUUIDToCleanups, cleanupsC)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0]["_id"], gc.Equals, s.state.docID(oldID))
}

func (s *upgradesSuite) TestAddEnvUUIDToSettingsRefs(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToSettingsRefs, settingsrefsC,
		bson.M{
			"_id":      "something",
			"refcount": 3,
		},
		bson.M{
			"_id":      "config",
			"refcount": 8,
		},
	)

	var newDoc bson.M
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc["refcount"], gc.Equals, 3)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc["refcount"], gc.Equals, 8)
}

func (s *upgradesSuite) TestAddEnvUUIDToSettingsRefsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToSettingsRefs, settingsrefsC)
}

func (s *upgradesSuite) TestAddEnvUUIDToSettings(c *gc.C) {
	coll, newIDs, count := s.checkEnvUUID(c, AddEnvUUIDToSettings, settingsC,
		[]bson.M{
			{
				"_id":  "something",
				"key2": "value2",
			},
			{
				"_id":  "config",
				"key3": "value3",
			}},
		true)
	c.Assert(count, gc.Equals, 3)

	var newDoc bson.M
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc["key2"], gc.Equals, "value2")

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc["key3"], gc.Equals, "value3")
}

func (s *upgradesSuite) TestAddEnvUUIDToSettingsIdempotent(c *gc.C) {
	oldID := "foo"
	docs := s.checkEnvUUIDIdempotent(c, oldID, AddEnvUUIDToSettings, settingsC)
	c.Assert(docs, gc.HasLen, 2)
	c.Assert(docs[0]["_id"], gc.Equals, s.state.docID(fmt.Sprint("e")))
	c.Assert(docs[1]["_id"], gc.Equals, s.state.docID(fmt.Sprint(oldID)))
}

func (s *upgradesSuite) TestAddEnvUUIDToReboots(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToReboots, rebootC,
		bson.M{
			"_id": "0",
		},
		bson.M{
			"_id": "1",
		},
	)

	var newDoc rebootDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Id, gc.Equals, "0")

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Id, gc.Equals, "1")
}

func (s *upgradesSuite) TestAddEnvUUIDToRebootsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToReboots, rebootC)
}

func (s *upgradesSuite) TestAddEnvUUIDToCharms(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToCharms, charmsC,
		bson.M{
			"_id":          "local:series/dummy-1",
			"bundlesha256": "series-dummy-1-sha256",
		},
		bson.M{
			"_id":          "local:anotherseries/dummy-2",
			"bundlesha256": "anotherseries-dummy-2-sha256",
		},
	)

	var newDoc charmDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.URL.String(), gc.Equals, "local:series/dummy-1")
	c.Assert(newDoc.BundleSha256, gc.Equals, "series-dummy-1-sha256")

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.URL.String(), gc.Equals, "local:anotherseries/dummy-2")
	c.Assert(newDoc.BundleSha256, gc.Equals, "anotherseries-dummy-2-sha256")
}

func (s *upgradesSuite) TestAddEnvUUIDToCharmsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToCharms, charmsC)
}

func (s *upgradesSuite) TestAddEnvUUIDToSequences(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToSequences, sequenceC,
		bson.M{
			"_id":     "0",
			"counter": 10,
		},
		bson.M{
			"_id":     "1",
			"counter": 15,
		},
	)

	var newDoc sequenceDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Name, gc.Equals, "0")
	c.Assert(newDoc.Counter, gc.Equals, 10)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Name, gc.Equals, "1")
	c.Assert(newDoc.Counter, gc.Equals, 15)
}

func (s *upgradesSuite) TestAddEnvUUIDToSequenceIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToSequences, sequenceC)
}

func (s *upgradesSuite) TestAddEnvUUIDToInstanceData(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToInstanceData, instanceDataC,
		bson.M{
			"_id":    "0",
			"status": "alive",
		},
		bson.M{
			"_id":    "1",
			"status": "dead",
		},
	)

	var newDoc instanceData
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.MachineId, gc.Equals, "0")
	c.Assert(newDoc.Status, gc.Equals, "alive")

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.MachineId, gc.Equals, "1")
	c.Assert(newDoc.Status, gc.Equals, "dead")
}

func (s *upgradesSuite) TestAddEnvUUIDToInstanceDatasIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToInstanceData, instanceDataC)
}

func (s *upgradesSuite) TestAddEnvUUIDToContainerRef(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToContainerRefs, containerRefsC,
		bson.M{
			"_id":      "0",
			"children": []string{"1", "2"},
		},
		bson.M{
			"_id":      "1",
			"children": []string{"3", "4"},
		},
	)

	var newDoc machineContainers
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Id, gc.Equals, "0")
	c.Assert(newDoc.Children, gc.DeepEquals, []string{"1", "2"})

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Id, gc.Equals, "1")
	c.Assert(newDoc.Children, gc.DeepEquals, []string{"3", "4"})
}

func (s *upgradesSuite) TestAddEnvUUIDToContainerRefsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToContainerRefs, containerRefsC)
}

func (s *upgradesSuite) TestAddEnvUUIDToRelations(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToRelations, relationsC,
		bson.M{
			"_id": "foo:db bar:db",
			"id":  1,
		},
		bson.M{
			"_id": "foo:http bar:http",
			"id":  3,
		},
	)

	var newDoc relationDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Key, gc.Equals, "foo:db bar:db")
	c.Assert(newDoc.Id, gc.Equals, 1)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Key, gc.Equals, "foo:http bar:http")
	c.Assert(newDoc.Id, gc.Equals, 3)
}

func (s *upgradesSuite) TestAddEnvUUIDToRelationsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToRelations, relationsC)
}

func (s *upgradesSuite) TestAddEnvUUIDToRelationScopes(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToRelationScopes, relationScopesC,
		bson.M{
			"_id":       "r#0#peer#foo/0",
			"departing": false,
		},
		bson.M{
			"_id":       "r#1#provider#bar/0",
			"departing": true,
		},
	)

	var newDoc relationScopeDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Key, gc.Equals, "r#0#peer#foo/0")
	c.Assert(newDoc.Departing, jc.IsFalse)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Key, gc.Equals, "r#1#provider#bar/0")
	c.Assert(newDoc.Departing, jc.IsTrue)
}

func (s *upgradesSuite) TestAddEnvUUIDToRelationScopesIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToRelationScopes, relationScopesC)
}

func (s *upgradesSuite) TestAddEnvUUIDToMeterStatus(c *gc.C) {
	coll, newIDs := s.checkAddEnvUUIDToCollection(c, AddEnvUUIDToMeterStatus, meterStatusC,
		bson.M{
			"_id":  "u#foo/0",
			"code": MeterGreen.String(),
		},
		bson.M{
			"_id":  "u#bar/0",
			"code": MeterRed.String(),
		},
	)

	var newDoc meterStatusDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Code, gc.Equals, MeterGreen.String())

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Code, gc.Equals, MeterRed.String())
}

func (s *upgradesSuite) TestAddEnvUUIDToMeterStatusIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, AddEnvUUIDToMeterStatus, meterStatusC)
}

func (s *upgradesSuite) checkAddEnvUUIDToCollection(
	c *gc.C,
	upgradeStep func(*State) error,
	collName string,
	oldDocs ...bson.M,
) (*mgo.Collection, []interface{}) {
	coll, ids, count := s.checkEnvUUID(c, upgradeStep, collName, oldDocs, true)
	c.Assert(count, gc.Equals, len(oldDocs))
	return coll, ids
}

func (s *upgradesSuite) checkEnvUUID(
	c *gc.C,
	upgradeStep func(*State) error,
	collName string,
	oldDocs []bson.M,
	idUpdated bool,
) (*mgo.Collection, []interface{}, int) {
	c.Assert(len(oldDocs) >= 2, jc.IsTrue)
	for _, oldDoc := range oldDocs {
		s.addLegacyDoc(c, collName, oldDoc)
	}

	err := upgradeStep(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// For each old document check that _id has been migrated and that
	// env-uuid has been added correctly.
	coll, closer := s.state.getRawCollection(collName)
	s.AddCleanup(func(*gc.C) { closer() })
	var d map[string]string
	var ids []interface{}
	envTag := s.state.EnvironUUID()
	for _, oldDoc := range oldDocs {
		id := oldDoc["_id"]
		if idUpdated {
			id = s.state.docID(fmt.Sprint(oldDoc["_id"]))
			err = coll.FindId(oldDoc["_id"]).One(&d)
			c.Assert(err, gc.Equals, mgo.ErrNotFound)
		}

		err = coll.FindId(id).One(&d)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(d["env-uuid"], gc.Equals, envTag)

		ids = append(ids, id)
	}
	count, err := coll.Find(nil).Count()
	c.Assert(err, jc.ErrorIsNil)
	return coll, ids, count
}

func (s *upgradesSuite) checkAddEnvUUIDToCollectionIdempotent(
	c *gc.C,
	upgradeStep func(*State) error,
	collName string,
) {
	oldID := "foo"
	docs := s.checkEnvUUIDIdempotent(c, oldID, upgradeStep, collName)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0]["_id"], gc.Equals, s.state.docID(fmt.Sprint(oldID)))
}

func (s *upgradesSuite) checkEnvUUIDIdempotent(
	c *gc.C,
	oldID interface{},
	upgradeStep func(*State) error,
	collName string,
) (docs []map[string]string) {
	s.addLegacyDoc(c, collName, bson.M{"_id": oldID})

	err := upgradeStep(s.state)
	c.Assert(err, jc.ErrorIsNil)

	err = upgradeStep(s.state)
	c.Assert(err, jc.ErrorIsNil)

	coll, closer := s.state.getRawCollection(collName)
	defer closer()
	err = coll.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	return docs
}

func (s *upgradesSuite) addLegacyDoc(c *gc.C, collName string, legacyDoc bson.M) {
	ops := []txn.Op{{
		C:      collName,
		Id:     legacyDoc["_id"],
		Assert: txn.DocMissing,
		Insert: legacyDoc,
	}}
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) FindId(c *gc.C, coll *mgo.Collection, id interface{}, doc interface{}) {
	err := coll.FindId(id).One(doc)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestAddCharmStoragePathsAllOrNothing(c *gc.C) {
	ch := testcharms.Repo.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)

	bundleSHA256 := "dummy-1-sha256"
	dummyCharm, err := s.state.AddCharm(ch, curl, "", bundleSHA256)
	c.Assert(err, jc.ErrorIsNil)

	curl2 := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()+1),
	)
	storagePaths := map[*charm.URL]string{
		curl:  "/some/where",
		curl2: "/some/where/else",
	}
	err = AddCharmStoragePaths(s.state, storagePaths)
	c.Assert(err, gc.ErrorMatches, "charms not found")

	// The charm entry for "curl" should not have been touched.
	dummyCharm, err = s.state.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dummyCharm.StoragePath(), gc.Equals, "")
}

func (s *upgradesSuite) TestSetOwnerAndServerUUIDForEnvironment(c *gc.C) {
	env, err := s.state.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// force remove the server-uuid and owner
	ops := []txn.Op{{
		C:      environmentsC,
		Id:     env.UUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$unset", bson.D{
			{"server-uuid", nil}, {"owner", nil},
		}}},
	}}
	err = s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
	// Make sure it has gone.
	environments, closer := s.state.getRawCollection(environmentsC)
	defer closer()

	var envDoc environmentDoc
	err = environments.FindId(env.UUID()).One(&envDoc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envDoc.ServerUUID, gc.Equals, "")
	c.Assert(envDoc.Owner, gc.Equals, "")

	// Run the upgrade step
	err = SetOwnerAndServerUUIDForEnvironment(s.state)
	c.Assert(err, jc.ErrorIsNil)
	// Make sure it is there now
	env, err = s.state.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.ControllerTag().Id(), gc.Equals, env.UUID())
	c.Assert(env.Owner().Id(), gc.Equals, "admin@local")
}

func (s *upgradesSuite) TestSetOwnerAndServerUUIDForEnvironmentIdempotent(c *gc.C) {
	// Run the upgrade step
	err := SetOwnerAndServerUUIDForEnvironment(s.state)
	c.Assert(err, jc.ErrorIsNil)
	// Run the upgrade step gagain
	err = SetOwnerAndServerUUIDForEnvironment(s.state)
	c.Assert(err, jc.ErrorIsNil)
	// Check as expected
	env, err := s.state.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.ControllerTag().Id(), gc.Equals, env.UUID())
	c.Assert(env.Owner().Id(), gc.Equals, "admin@local")
}

func openLegacyPort(c *gc.C, unit *Unit, number int, proto string) {
	port := network.Port{Protocol: proto, Number: number}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     unit.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$addToSet", bson.D{{"ports", port}}}},
	}}
	err := unit.st.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func openLegacyRange(c *gc.C, unit *Unit, from, to int, proto string) {
	c.Assert(from <= to, jc.IsTrue, gc.Commentf("expected %d <= %d", from, to))
	for port := from; port <= to; port++ {
		openLegacyPort(c, unit, port, proto)
	}
}

func (s *upgradesSuite) setUpPortsMigration(c *gc.C) ([]*Machine, map[int][]*Unit) {

	// Setup the test scenario by creating 3 services with 1, 2, and 3
	// units respectively, and 3 machines. Then assign the units like
	// this:
	//
	// (services[0]) units[0][0] -> machines[0]
	// (services[1]) units[1][0] -> machines[1]
	// (services[1]) units[1][1] -> machines[0] (co-located with units[0][0])
	// (services[2]) units[2][0] -> machines[2]
	// (services[2]) units[2][1] -> machines[1] (co-located with units[1][0])
	// (services[2]) units[2][2] -> unassigned
	//
	// Finally, open some ports on the units using the legacy method
	// (only on the unitDoc.Ports) and a new-style port range on
	// machines[2] to test all the relevant cases during the
	// migration.

	// Add the machines.
	machines, err := s.state.AddMachines([]MachineTemplate{
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
	}...)
	c.Assert(err, jc.ErrorIsNil)

	// Add the charm, services and units, assign to machines.
	services := make([]*Service, 3)
	units := make(map[int][]*Unit)
	networks := []string{network.DefaultPublic}
	charm := AddTestingCharm(c, s.state, "wordpress")
	stateOwner, err := s.state.AddUser("bob", "notused", "notused", "bob")
	c.Assert(err, jc.ErrorIsNil)
	ownerTag := stateOwner.UserTag()
	_, err = s.state.AddEnvironmentUser(ownerTag, ownerTag, "")
	c.Assert(err, jc.ErrorIsNil)

	for i := range services {
		name := fmt.Sprintf("wp%d", i)
		services[i] = AddTestingServiceWithNetworks(
			c, s.state, name, charm, ownerTag, networks,
		)
		numUnits := i + 1
		units[i] = make([]*Unit, numUnits)
		for j := 0; j < numUnits; j++ {
			unit, err := services[i].AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			switch {
			case j == 0:
				// The first unit of each service goes to a machine
				// with the same index as the service.
				err = unit.AssignToMachine(machines[i])
				c.Assert(err, jc.ErrorIsNil)
			case j == 1 && i >= 1:
				// Co-locate the second unit of each service. Leave
				// units[2][2] unassigned.
				err = unit.AssignToMachine(machines[i-1])
				c.Assert(err, jc.ErrorIsNil)
			}
			units[i][j] = unit
		}
	}

	// Open ports on units using the legacy method, covering all
	// cases below:
	// - invalid port (0 <= port || port > 65535) (not validated before)
	// - invalid proto (i.e. 42/invalid) (not validated before)
	// - mixed case proto (i.e. 443/tCp), still valid (but saved as-is in state)
	// - valid port and proto (i.e. 80/tcp)
	// - overlapping (legacy) ranges; 4 sub-cases here:
	//   - complete overlap (i.e. 10-20/tcp and 10-20/tcp)
	//   - left-bound overlap (i.e. 10-20/tcp and 10-30/tcp)
	//   - right-bound overlap (i.e. 30-40/tcp and 20-40/tcp)
	//   - complete inclusion (i.e. 10-50/tcp and 20-30/tcp or vice versa)
	// - mixed case proto range (i.e. 10-20/tCp), valid when not overlapping
	// - invalid proto range (i.e. 10-20/invalid)
	// - valid, non-overlapping ranges (i.e. 10-20/tcp and 30-40/tcp)
	// - overlapping ranges, different proto (i.e. 10-20/tcp and 10-20/udp), valid
	//
	// NOTE: When talking about a (legacy) range here, we mean opening
	// each individual port separately on the unit, using the same
	// protocol (using the openLegacyRange helper below). Also, by
	// "overlapping ranges" we mean overlapping both on the same unit
	// and on different units assigned to the same machine.
	//
	// Distribute the cases described above like this:
	//
	// machines[2] (new-style port ranges):
	// - 100-110/tcp (simulate a pre-existing range for units[2][1],
	//   without opening the ports on the unit itself)
	//
	// This is done "manually" because doing it conventionally using
	// State methods will mean that the document ID used will be env
	// UUID prefixed and this migration runs before the openedPorts
	// collection is migrated to using environment UUIDs. The
	// migration step expects the openedPorts collection to be pre-env
	// UUID migration.
	err = s.state.runRawTransaction([]txn.Op{{
		C:      openedPortsC,
		Id:     portsGlobalKey("2", network.DefaultPublic),
		Assert: txn.DocMissing,
		Insert: bson.M{
			"machine-id":   "2",
			"network-name": network.DefaultPublic,
			"ports": []bson.M{{
				"unitname": units[2][1].Name(),
				"fromport": 100,
				"toport":   110,
				"protocol": "tcp",
			}},
		},
	}})
	c.Assert(err, jc.ErrorIsNil)

	// units[0][0] (on machines[0]):
	// - no ports opened
	//
	// units[1][0] (on machines[1]):
	// - 10-20/invalid (invalid; won't be migrated and a warning will
	//   be logged instead)
	openLegacyRange(c, units[1][0], 10, 20, "invalid")
	// - -10-5/tcp (invalid; will be migrated partially, as 1-5/tcp
	//   (wp1/0), logging a warning)
	openLegacyRange(c, units[1][0], -10, 5, "tcp")
	// - 443/tCp, 63/UDP (all valid, will be migrated and the protocol
	//   will be lower-cased, i.e. 443-443/tcp (wp1/0) and 63-63/udp
	//   (wp1/0).)
	openLegacyPort(c, units[1][0], 443, "tCp")
	openLegacyPort(c, units[1][0], 63, "UDP")
	// - 100-110/tcp (valid, but overlapping with units[2][1]; will
	//   be migrated as 100-110/tcp because it appears first)
	openLegacyRange(c, units[1][0], 100, 110, "tcp")
	// - 80-85/tcp (valid; migrated as 80-85/tcp (wp1/0).)
	openLegacyRange(c, units[1][0], 80, 85, "tcp")
	// - 22/tcp (valid, but overlapping with units[2][1]; will be
	//   migrated as 22-22/tcp (wp1/0), because it appears first)
	openLegacyPort(c, units[1][0], 22, "tcp")
	// - 4000-4010/tcp (valid, not overlapping with units[2][1],
	//   because the protocol is different; migrated as 4000-4010/tcp
	//   (wp1/0).)
	openLegacyRange(c, units[1][0], 4000, 4010, "tcp")
	// - add the same range twice, to ensure the duplicates will be
	//   ignored, so this will not migrated, but a warning logged
	//   instead).
	openLegacyRange(c, units[1][0], 4000, 4010, "tcp")
	//
	// units[1][1] (on machine[0]):
	// - 10/tcp, 11/tcp, 12/tcp, 13/tcp, 14/udp (valid; will be
	//   migrated as 10-13/tcp (wp1/1) and 14-14/udp (wp1/1)).
	openLegacyPort(c, units[1][1], 10, "tcp")
	openLegacyPort(c, units[1][1], 11, "tcp")
	openLegacyPort(c, units[1][1], 12, "tcp")
	openLegacyPort(c, units[1][1], 13, "tcp")
	openLegacyPort(c, units[1][1], 14, "udp")
	// - 42/ (empty protocol; invalid, won't be migrated, but a
	//   warning will be logged instead)
	openLegacyPort(c, units[1][1], 42, "")
	//
	// units[2][0] (on machines[2]):
	// - 90-120/tcp (valid, but overlapping with the new-style port
	//   range 100-110/tcp opened on machines[2] earlier; will be
	//   skipped with a warning, as 100-110/tcp existed already and
	//   90-120/tcp conflicts with it).
	openLegacyRange(c, units[2][0], 90, 120, "tcp")
	// - 10-20/tcp (valid; migrated as expected)
	openLegacyRange(c, units[2][0], 10, 20, "tcp")
	// - 65530-65540/udp (invalid; will be partially migrated as
	//   65530-65535/udp, logging warnings for the rest).
	openLegacyRange(c, units[2][0], 65530, 65540, "udp")
	//
	// units[2][1] (on machines[1]):
	// - 90-105/tcp (valid, but overlapping with units[1][0]; won't
	//   be migrated, logging a warning for the conflict).
	openLegacyRange(c, units[2][1], 90, 105, "tcp")
	// - 100-110/udp (valid, overlapping with units[1][0] but the
	//   protocol is different, so will be migrated as expected)
	openLegacyRange(c, units[2][1], 100, 110, "udp")
	// - 22/tcp (valid, but overlapping with units[1][0]; won't be
	//   migrated, as it appears later, but logged as a warning)
	openLegacyPort(c, units[2][1], 22, "tcp")
	// - 8080/udp (valid and migrated as 8080-8080/udp (wp2/1).)
	openLegacyPort(c, units[2][1], 8080, "udp")
	// - 1234/tcp (valid, but due to the initial collapsing of ports
	//   into ranges, it will be migrated as 1234-1235/tcp (wp2/1)
	//   along with the next one.
	openLegacyPort(c, units[2][1], 1234, "tcp")
	// - adding the same port twice to ensure duplicated legacy ports
	//   for the same unit are ignored (this will be collapsed with
	//   the previous).
	openLegacyRange(c, units[2][1], 1234, 1235, "tcp")
	// - 4000-4010/udp (valid, not overlapping with units[1][0],
	//   because the protocol is different; migrated as 4000-4010/udp
	//   (wp2/1).)
	openLegacyRange(c, units[2][1], 4000, 4010, "udp")
	//
	// units[2][2] (unassigned):
	// - 80/tcp (valid, but won't be migrated as the unit's unassigned)
	openLegacyPort(c, units[2][2], 80, "tcp")

	return machines, units
}

func (s *upgradesSuite) newRange(from, to int, proto string) network.PortRange {
	return network.PortRange{from, to, proto}
}

func (s *upgradesSuite) assertInitialMachinePorts(c *gc.C, machines []*Machine, units map[int][]*Unit) {
	for i := range machines {
		ports, err := GetPorts(s.state, machines[i].Id(), network.DefaultPublic)
		if i != 2 {
			c.Assert(err, jc.Satisfies, errors.IsNotFound)
			c.Assert(ports, gc.IsNil)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			allRanges := ports.AllPortRanges()
			c.Assert(allRanges, jc.DeepEquals, map[network.PortRange]string{
				s.newRange(100, 110, "tcp"): units[2][1].Name(),
			})
		}
	}
}

func (s *upgradesSuite) assertUnitPortsPostMigration(c *gc.C, units map[int][]*Unit) {
	for _, serviceUnits := range units {
		for _, unit := range serviceUnits {
			err := unit.Refresh()
			c.Assert(err, jc.ErrorIsNil)
			if unit.Name() == units[2][2].Name() {
				// Only units[2][2] will have ports on its doc, as
				// it's not assigned to a machine.
				c.Assert(networkPorts(unit.doc.Ports), jc.DeepEquals, []network.Port{
					{Protocol: "tcp", Number: 80},
				})
			} else {
				c.Assert(unit.doc.Ports, gc.HasLen, 0, gc.Commentf("unit %q has unexpected ports %v", unit, unit.doc.Ports))
			}
		}
	}
}

func (s *upgradesSuite) assertFinalMachinePorts(c *gc.C, machines []*Machine, units map[int][]*Unit) {
	for i := range machines {
		c.Assert(machines[i].Refresh(), gc.IsNil)
		allMachinePorts, err := machines[i].AllPorts()
		c.Assert(err, jc.ErrorIsNil)
		for _, ports := range allMachinePorts {
			allPortRanges := ports.AllPortRanges()
			switch i {
			case 0:
				c.Assert(allPortRanges, jc.DeepEquals, map[network.PortRange]string{
					// 10/tcp..13/tcp were merged and migrated as
					// 10-13/tcp (wp1/1).
					s.newRange(10, 13, "tcp"): units[1][1].Name(),
					// 14/udp was migrated ok as 14-14/udp (wp1/1).
					s.newRange(14, 14, "udp"): units[1][1].Name(),
				})
			case 1:
				c.Assert(allPortRanges, jc.DeepEquals, map[network.PortRange]string{
					// 63/UDP was migrated ok as 63-63/udp (wp1/0).
					s.newRange(63, 63, "udp"): units[1][0].Name(),
					// 443/tCp was migrated ok as 443-443/tcp (wp1/0).
					s.newRange(443, 443, "tcp"): units[1][0].Name(),
					// -1/tcp..5/tcp was merged, sanitized, and
					// migrated ok as 1-5/tcp (wp1/0).
					s.newRange(1, 5, "tcp"): units[1][0].Name(),
					// 22/tcp was migrated ok as 22-22/tcp (wp1/0).
					s.newRange(22, 22, "tcp"): units[1][0].Name(),
					// 80/tcp..85/tcp were merged and migrated ok as
					// 80-85/tcp (wp1/0).
					s.newRange(80, 85, "tcp"): units[1][0].Name(),
					// 100/tcp..110/tcp were merged and migrated ok as
					// 100-110/tcp (wp1/0).
					s.newRange(100, 110, "tcp"): units[1][0].Name(),
					// 4000/tcp..4010/tcp were merged and migrated ok
					// as 4000-4010/tcp (wp1/0).
					s.newRange(4000, 4010, "tcp"): units[1][0].Name(),
					// 1234/tcp,1234/tcp..1235/tcp were merged,
					// duplicates ignored, and migrated ok as
					// 1234-1235/tcp (wp2/1).
					s.newRange(1234, 1235, "tcp"): units[2][1].Name(),
					// 100/udp..110/udp were merged and migrated ok as
					// 100-110/udp (wp2/1).
					s.newRange(100, 110, "udp"): units[2][1].Name(),
					// 4000/udp..4010/udp were merged and migrated ok
					// as 4000-4010/udp (wp2/1).
					s.newRange(4000, 4010, "udp"): units[2][1].Name(),
					// 8080/udp was migrated ok as 8080-8080/udp (wp2/1).
					s.newRange(8080, 8080, "udp"): units[2][1].Name(),
				})
			case 2:
				c.Assert(allPortRanges, jc.DeepEquals, map[network.PortRange]string{
					// 100-110/tcp (wp2/1) existed before migration.
					s.newRange(100, 110, "tcp"): units[2][1].Name(),
					// 10/tcp..20/tcp were merged and migrated ok as
					// 10-20/tcp (wp2/0).
					s.newRange(10, 20, "tcp"): units[2][0].Name(),
					// 65530/udp..65540/udp were merged, sanitized,
					// and migrated ok as 65530-65535/udp (wp2/0).
					s.newRange(65530, 65535, "udp"): units[2][0].Name(),
				})
			}
		}
	}
}

func (s *upgradesSuite) TestMigrateUnitPortsToOpenedPorts(c *gc.C) {
	s.patchPortOptFuncs()

	machines, units := s.setUpPortsMigration(c)

	// Ensure there are no new-style port ranges before the migration,
	// except for machines[2].
	s.assertInitialMachinePorts(c, machines, units)

	err := MigrateUnitPortsToOpenedPorts(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure there are no ports on the migrated units' documents,
	// except for units[2][2].
	s.assertUnitPortsPostMigration(c, units)

	// Ensure new-style port ranges are migrated as expected.
	s.assertFinalMachinePorts(c, machines, units)
}

func (s *upgradesSuite) TestMigrateUnitPortsToOpenedPortsIdempotent(c *gc.C) {
	s.patchPortOptFuncs()

	machines, units := s.setUpPortsMigration(c)

	// Ensure there are no new-style port ranges before the migration,
	// except for machines[2].
	s.assertInitialMachinePorts(c, machines, units)

	err := MigrateUnitPortsToOpenedPorts(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure there are no ports on the migrated units' documents,
	// except for units[2][2].
	s.assertUnitPortsPostMigration(c, units)

	// Ensure new-style port ranges are migrated as expected.
	s.assertFinalMachinePorts(c, machines, units)

	// Migrate and check again, should work fine.
	err = MigrateUnitPortsToOpenedPorts(s.state)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUnitPortsPostMigration(c, units)
	s.assertFinalMachinePorts(c, machines, units)
}

// patchPortOptsFuncs patches addPortsDocOps, updatePortsDocOps and setPortsDocOps
// to accommodate pre 1.22 schema during multihop upgrades. It returns a func
// which restores original behaviour.
func (s *upgradesSuite) patchPortOptFuncs() {
	s.PatchValue(
		&GetPorts,
		func(st *State, machineId, networkName string) (*Ports, error) {
			openedPorts, closer := st.getRawCollection(openedPortsC)
			defer closer()

			var doc portsDoc
			key := portsGlobalKey(machineId, networkName)
			err := openedPorts.FindId(key).One(&doc)
			if err != nil {
				doc.MachineID = machineId
				doc.NetworkName = networkName
				p := Ports{st, doc, false}
				if err == mgo.ErrNotFound {
					return nil, errors.NotFoundf(p.String())
				}
				return nil, errors.Annotatef(err, "cannot get %s", p.String())
			}

			return &Ports{st, doc, false}, nil
		})

	s.PatchValue(
		&GetOrCreatePorts,
		func(st *State, machineId, networkName string) (*Ports, error) {
			ports, err := GetPorts(st, machineId, networkName)
			if errors.IsNotFound(err) {
				doc := portsDoc{
					MachineID:   machineId,
					NetworkName: networkName,
				}
				ports = &Ports{st, doc, true}
				upgradesLogger.Debugf(
					"created ports for machine %q, network %q",
					machineId, networkName,
				)
			} else if err != nil {
				return nil, errors.Trace(err)
			}
			return ports, nil
		})

	s.PatchValue(
		&addPortsDocOps,
		func(st *State, pDoc *portsDoc, portsAssert interface{}, ports ...PortRange) []txn.Op {
			pDoc.Ports = ports
			return []txn.Op{{
				C:      machinesC,
				Id:     st.docID(pDoc.MachineID),
				Assert: notDeadDoc,
			}, {
				C:      openedPortsC,
				Id:     portsGlobalKey(pDoc.MachineID, pDoc.NetworkName),
				Assert: portsAssert,
				Insert: pDoc,
			}}
		})

	s.PatchValue(
		&updatePortsDocOps,
		func(st *State, pDoc portsDoc, portsAssert interface{}, portRange PortRange) []txn.Op {
			return []txn.Op{{
				C:      machinesC,
				Id:     st.docID(pDoc.MachineID),
				Assert: notDeadDoc,
			}, {
				C:      unitsC,
				Id:     portRange.UnitName,
				Assert: notDeadDoc,
			}, {
				C:      openedPortsC,
				Id:     portsGlobalKey(pDoc.MachineID, pDoc.NetworkName),
				Assert: portsAssert,
				Update: bson.D{{"$addToSet", bson.D{{"ports", portRange}}}},
			}}
		},
	)

	s.PatchValue(
		&setPortsDocOps,
		func(st *State, pDoc portsDoc, portsAssert interface{}, ports ...PortRange) []txn.Op {
			return []txn.Op{{
				C:      machinesC,
				Id:     st.docID(pDoc.MachineID),
				Assert: notDeadDoc,
			}, {
				C:      openedPortsC,
				Id:     portsGlobalKey(pDoc.MachineID, pDoc.NetworkName),
				Assert: portsAssert,
				Update: bson.D{{"$set", bson.D{{"ports", ports}}}},
			}}
		})
}

func (s *upgradesSuite) setUpMeterStatusCreation(c *gc.C) []*Unit {
	// Set up the test scenario with several units that have no meter status docs
	// associated with them.
	units := make([]*Unit, 9)
	charm := AddTestingCharm(c, s.state, "wordpress")
	stateOwner, err := s.state.AddUser("bob", "notused", "notused", "bob")
	c.Assert(err, jc.ErrorIsNil)
	ownerTag := stateOwner.UserTag()
	_, err = s.state.AddEnvironmentUser(ownerTag, ownerTag, "")
	c.Assert(err, jc.ErrorIsNil)

	for i := 0; i < 3; i++ {
		svc := AddTestingService(c, s.state, fmt.Sprintf("service%d", i), charm, ownerTag)

		for j := 0; j < 3; j++ {
			name, err := svc.newUnitName()
			c.Assert(err, jc.ErrorIsNil)
			docID := s.state.docID(name)
			udoc := &unitDoc{
				DocID:     docID,
				Name:      name,
				EnvUUID:   svc.doc.EnvUUID,
				Service:   svc.doc.Name,
				Series:    svc.doc.Series,
				Life:      Alive,
				Principal: "",
			}
			ops := []txn.Op{
				{
					C:      unitsC,
					Id:     docID,
					Assert: txn.DocMissing,
					Insert: udoc,
				},
				{
					C:      servicesC,
					Id:     svc.doc.DocID,
					Assert: isAliveDoc,
					Update: bson.D{{"$inc", bson.D{{"unitcount", 1}}}},
				}}
			err = s.state.runRawTransaction(ops)
			c.Assert(err, jc.ErrorIsNil)
			units[i*3+j], err = s.state.Unit(name)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
	return units
}

func (s *upgradesSuite) TestCreateMeterStatuses(c *gc.C) {
	units := s.setUpMeterStatusCreation(c)

	// assert the units do not have meter status documents
	for _, unit := range units {
		_, err := unit.GetMeterStatus()
		c.Assert(err, gc.ErrorMatches, "cannot retrieve meter status for unit .*: not found")
	}

	// run meter status upgrade
	err := CreateUnitMeterStatus(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// assert the units do not have meter status documents
	for _, unit := range units {
		status, err := unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status, gc.DeepEquals, MeterStatus{MeterNotSet, ""})
	}

	// run migration again to make sure it's idempotent
	err = CreateUnitMeterStatus(s.state)
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		status, err := unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status, gc.DeepEquals, MeterStatus{MeterNotSet, ""})
	}
}

func (s *upgradesSuite) TestMigrateMachineInstanceIdToInstanceData(c *gc.C) {
	machineID := "0"
	var instID instance.Id = "1"
	s.instanceIdSetUp(c, machineID, instID)

	err := MigrateMachineInstanceIdToInstanceData(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.instanceIdAssertMigration(c, machineID, instID)
}

func (s *upgradesSuite) TestMigrateMachineInstanceIdToInstanceDataIdempotent(c *gc.C) {
	machineID := "0"
	var instID instance.Id = "1"
	s.instanceIdSetUp(c, machineID, instID)

	err := MigrateMachineInstanceIdToInstanceData(s.state)
	c.Assert(err, jc.ErrorIsNil)

	err = MigrateMachineInstanceIdToInstanceData(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.instanceIdAssertMigration(c, machineID, instID)
}

func (s *upgradesSuite) TestMigrateMachineInstanceIdNoIdLogsWarning(c *gc.C) {
	machineID := "0"
	var instID instance.Id = ""
	s.instanceIdSetUp(c, machineID, instID)

	MigrateMachineInstanceIdToInstanceData(s.state)
	c.Assert(c.GetTestLog(), jc.Contains, `WARNING juju.state.upgrade machine "0" doc has no instanceid`)
}

func (s *upgradesSuite) instanceIdSetUp(c *gc.C, machineID string, instID instance.Id) {
	mDoc := bson.M{
		"_id":        machineID,
		"instanceid": instID,
	}
	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     machineID,
			Assert: txn.DocMissing,
			Insert: mDoc,
		},
	}
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) instanceIdAssertMigration(c *gc.C, machineID string, instID instance.Id) {
	// check to see if instanceid is in instance
	var instanceMap bson.M
	insts, closer := s.state.getRawCollection(instanceDataC)
	defer closer()
	err := insts.FindId(machineID).One(&instanceMap)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceMap["instanceid"], gc.Equals, string(instID))

	// check to see if instanceid field is removed
	var machineMap bson.M
	machines, closer := s.state.getRawCollection(machinesC)
	defer closer()
	err = machines.FindId(machineID).One(&machineMap)
	c.Assert(err, jc.ErrorIsNil)
	_, keyExists := machineMap["instanceid"]
	c.Assert(keyExists, jc.IsFalse)
}

func (s *upgradesSuite) TestAddAvailabilityZoneToInstanceData(c *gc.C) {
	foundInstZoneFunc := func(*State, instance.Id) (string, error) {
		return "a_zone", nil
	}

	for i, test := range []azoneTest{
		{
			about:        "Update an environ level machine's availability zone",
			id:           "9999",
			azfunc:       foundInstZoneFunc,
			expectedZone: "a_zone",
		}, {
			about:  "Don't update a container's availability zone",
			id:     "1/lxc/0",
			azfunc: foundInstZoneFunc,
		},
		{
			about: "Skip missing instance",
			id:    "8888",
			azfunc: func(*State, instance.Id) (string, error) {
				return "", errors.NotFoundf("instances")
			},
		},
	} {
		c.Logf("%d: %s", i, test.about)
		machineID := test.id
		instID := instance.Id(test.id)
		s.azSetUp(c, machineID, instID)

		err := AddAvailabilityZoneToInstanceData(s.state, test.azfunc)
		c.Assert(err, jc.ErrorIsNil)

		s.checkAvailabilityZone(c, machineID, test.expectedZone)
	}
}

type azoneTest struct {
	about        string
	id           string
	azfunc       func(*State, instance.Id) (string, error)
	expectedZone string
}

func (s *upgradesSuite) azSetUp(c *gc.C, machineID string, instID instance.Id) {
	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     machineID,
			Assert: txn.DocMissing,
			Insert: bson.M{
				"_id":        machineID,
				"instanceid": instID,
			},
		},
		{
			C:      instanceDataC,
			Id:     machineID,
			Assert: txn.DocMissing,
			Insert: bson.M{
				"_id":        machineID,
				"machineid":  machineID,
				"instanceid": instID,
				// We do *not* set AvailZone.
			},
		},
	}
	err := s.state.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure "availzone" isn't set.
	var instanceMap bson.M
	insts, closer := s.state.getCollection(instanceDataC)
	defer closer()
	err = insts.FindId(machineID).One(&instanceMap)
	c.Assert(err, jc.ErrorIsNil)
	_, ok := instanceMap["availzone"]
	c.Assert(ok, jc.IsFalse)
}

// checkAvailabilityZone checks to see if the availability zone is set
// for the instance data associated with the machine.
func (s *upgradesSuite) checkAvailabilityZone(c *gc.C, machineID string, expectedZone string) {
	var instanceMap bson.M
	insts, closer := s.state.getCollection(instanceDataC)
	defer closer()
	err := insts.FindId(machineID).One(&instanceMap)
	c.Assert(err, jc.ErrorIsNil)

	if expectedZone == "" {
		_, ok := instanceMap["availzone"]
		c.Check(ok, jc.IsFalse)
	} else {
		zone, ok := instanceMap["availzone"]
		c.Check(ok, jc.IsTrue)
		c.Check(zone, gc.Equals, expectedZone)
	}
}

// setUpJobManageNetworking prepares the test environment for the JobManageNetworking tests.
func (s *upgradesSuite) setUpJobManageNetworking(c *gc.C, provider string, manual bool) {
	// Set provider type.
	settings, err := readSettings(s.state, environGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	settings.Set("type", provider)
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)
	// Add machines.
	machines, err := s.state.AddMachines([]MachineTemplate{
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
	}...)
	c.Assert(err, jc.ErrorIsNil)
	ops := []txn.Op{}
	if manual {
		mdoc := machines[2].doc
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     mdoc.DocID,
			Update: bson.D{{"$set", bson.D{{"nonce", "manual:" + mdoc.Nonce}}}},
		})
	}
	// Run transaction.
	err = s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

// checkJobManageNetworking tests if the machine withe the given id has the
// JobManageNetworking if hasJob shows that it should.
func (s *upgradesSuite) checkJobManageNetworking(c *gc.C, id string, hasJob bool) {
	machine, err := s.state.Machine(id)
	c.Assert(err, jc.ErrorIsNil)
	jobs := machine.Jobs()
	foundJob := false
	for _, job := range jobs {
		if job == JobManageNetworking {
			foundJob = true
			break
		}
	}
	c.Assert(foundJob, gc.Equals, hasJob)
}

// tearDownJobManageNetworking cleans the test environment for the following tests.
func (s *upgradesSuite) tearDownJobManageNetworking(c *gc.C) {
	// Remove machines.
	machines, err := s.state.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	for _, machine := range machines {
		err = machine.ForceDestroy()
		c.Assert(err, jc.ErrorIsNil)
		err = machine.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = machine.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}
	// Reset machine sequence.
	sequences, closer := s.state.getCollection(sequenceC)
	defer closer()
	query := sequences.FindId(s.state.docID("machine"))
	set := mgo.Change{
		Update: bson.M{"$set": bson.M{"counter": 0}},
		Upsert: true,
	}
	result := &sequenceDoc{}
	_, err = query.Apply(set, result)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestJobManageNetworking(c *gc.C) {
	tests := []struct {
		description string
		provider    string
		manual      bool
		hasJob      []bool
	}{{
		description: "azure provider, no manual provisioned machines",
		provider:    "azure",
		manual:      false,
		hasJob:      []bool{true, true, true},
	}, {
		description: "azure provider, one manual provisioned machine",
		provider:    "azure",
		manual:      true,
		hasJob:      []bool{true, true, false},
	}, {
		description: "ec2 provider, no manual provisioned machines",
		provider:    "ec2",
		manual:      false,
		hasJob:      []bool{true, true, true},
	}, {
		description: "ec2 provider, one manual provisioned machine",
		provider:    "ec2",
		manual:      true,
		hasJob:      []bool{true, true, false},
	}, {
		description: "joyent provider, no manual provisioned machines",
		provider:    "joyent",
		manual:      false,
		hasJob:      []bool{false, false, false},
	}, {
		description: "joyent provider, one manual provisioned machine",
		provider:    "joyent",
		manual:      true,
		hasJob:      []bool{false, false, false},
	}, {
		description: "local provider, no manual provisioned machines",
		provider:    "local",
		manual:      false,
		hasJob:      []bool{false, true, true},
	}, {
		description: "maas provider, no manual provisioned machines",
		provider:    "maas",
		manual:      false,
		hasJob:      []bool{false, false, false},
	}, {
		description: "maas provider, one manual provisioned machine",
		provider:    "maas",
		manual:      true,
		hasJob:      []bool{false, false, false},
	}, {
		description: "manual provider, only manual provisioned machines",
		provider:    "manual",
		manual:      false,
		hasJob:      []bool{false, false, false},
	}, {
		description: "openstack provider, no manual provisioned machines",
		provider:    "openstack",
		manual:      false,
		hasJob:      []bool{true, true, true},
	}, {
		description: "openstack provider, one manual provisioned machine",
		provider:    "openstack",
		manual:      true,
		hasJob:      []bool{true, true, false},
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.description)
		s.setUpJobManageNetworking(c, test.provider, test.manual)

		err := MigrateJobManageNetworking(s.state)
		c.Assert(err, jc.ErrorIsNil)

		s.checkJobManageNetworking(c, "0", test.hasJob[0])
		s.checkJobManageNetworking(c, "1", test.hasJob[1])
		s.checkJobManageNetworking(c, "2", test.hasJob[2])

		s.tearDownJobManageNetworking(c)
	}
}

func (s *upgradesSuite) TestFixMinUnitsEnvUUID(c *gc.C) {
	minUnits, closer := s.state.getRawCollection(minUnitsC)
	defer closer()

	uuid := s.state.EnvironUUID()

	err := minUnits.Insert(
		// This record should be left untouched.
		bson.D{
			{"_id", uuid + ":bar"},
			{"servicename", "bar"},
			{"env-uuid", uuid},
		},
		// This record should have its env-uuid field set.
		bson.D{
			{"_id", uuid + ":foo"},
			{"servicename", "foo"},
			{"env-uuid", ""},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = FixMinUnitsEnvUUID(s.state)
	c.Assert(err, jc.ErrorIsNil)

	var docs []minUnitsDoc
	err = minUnits.Find(nil).Sort("_id").All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, jc.DeepEquals, []minUnitsDoc{{
		DocID:       uuid + ":bar",
		ServiceName: "bar",
		EnvUUID:     uuid,
	}, {
		DocID:       uuid + ":foo",
		ServiceName: "foo",
		EnvUUID:     uuid,
	}})

}

func (s *upgradesSuite) TestFixSequenceFields(c *gc.C) {
	sequence, closer := s.state.getRawCollection(sequenceC)
	defer closer()

	uuid := s.state.EnvironUUID()

	err := sequence.Insert(
		// This record should be left untouched.
		bson.D{
			{"_id", uuid + ":ok"},
			{"name", "ok"},
			{"env-uuid", uuid},
			{"counter", 1},
		},
		// This record should have its env-uuid and name fields set.
		bson.D{
			{"_id", uuid + ":foobar"},
			{"name", ""},
			{"env-uuid", ""},
			{"counter", 2},
		},
		// This record should have its env-uuid field set.
		bson.D{
			{"_id", uuid + ":foo"},
			{"name", "foo"},
			{"env-uuid", ""},
			{"counter", 3},
		},
		// This record should have its name field set.
		bson.D{
			{"_id", uuid + ":bar"},
			{"name", ""},
			{"env-uuid", uuid},
			{"counter", 4},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = FixSequenceFields(s.state)
	c.Assert(err, jc.ErrorIsNil)

	var docs []sequenceDoc
	err = sequence.Find(nil).Sort("counter").All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, jc.DeepEquals, []sequenceDoc{{
		DocID:   uuid + ":ok",
		Name:    "ok",
		EnvUUID: uuid,
		Counter: 1,
	}, {
		DocID:   uuid + ":foobar",
		Name:    "foobar",
		EnvUUID: uuid,
		Counter: 2,
	}, {
		DocID:   uuid + ":foo",
		Name:    "foo",
		EnvUUID: uuid,
		Counter: 3,
	}, {
		DocID:   uuid + ":bar",
		Name:    "bar",
		EnvUUID: uuid,
		Counter: 4,
	}})
}

func (s *upgradesSuite) TestDropOldIndexesv123(c *gc.C) {

	var expectedOldIndexes = map[string]int{
		relationsC:         2,
		unitsC:             3,
		networksC:          1,
		networkInterfacesC: 4,
		blockDevicesC:      1,
		subnetsC:           1,
		ipaddressesC:       2,
	}

	// setup state
	for collName, indexes := range oldIndexesv123 {
		var i int
		func() {
			coll, closer := s.state.getRawCollection(collName)
			defer closer()

			// create the old indexes
			for _, oldIndex := range indexes {
				index := mgo.Index{Key: oldIndex}
				err := coll.EnsureIndex(index)
				c.Assert(err, jc.ErrorIsNil)
				i++
			}

			// check that the old indexes are there
			foundCount, oldCount := countOldIndexes(c, coll)
			c.Assert(foundCount, gc.Equals, oldCount)
		}()

		// check that the expected number of old indexes was added to guard
		// against accidental edits of oldIndexesv123.
		c.Assert(i, gc.Equals, expectedOldIndexes[collName])
	}

	// run upgrade step
	DropOldIndexesv123(s.state)

	// check that all old indexes are now missing
	for collName := range oldIndexesv123 {
		func() {
			coll, closer := s.state.getRawCollection(collName)
			defer closer()
			foundCount, _ := countOldIndexes(c, coll)
			c.Assert(foundCount, gc.Equals, 0)
		}()
	}
}

func countOldIndexes(c *gc.C, coll *mgo.Collection) (foundCount, oldCount int) {
	old := oldIndexesv123[coll.Name]
	oldCount = len(old)
	indexes, err := coll.Indexes()
	c.Assert(err, jc.ErrorIsNil)

	for _, collIndex := range indexes {
		for _, oldIndex := range old {
			if reflect.DeepEqual(collIndex.Key, oldIndex) {
				foundCount++
			}
		}
	}
	return
}

func (s *upgradesSuite) addMachineWithLife(c *gc.C, machineID string, life Life) {
	mDoc := bson.M{
		"_id":        s.state.docID(machineID),
		"instanceid": "foobar",
		"life":       life,
	}
	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     machineID,
			Assert: txn.DocMissing,
			Insert: mDoc,
		},
	}
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) removePreferredAddressFields(c *gc.C, machine *Machine) {
	machinesCol, closer := s.state.getRawCollection(machinesC)
	defer closer()

	err := machinesCol.Update(
		bson.D{{"_id", s.state.docID(machine.Id())}},
		bson.D{{"$unset", bson.D{{"preferredpublicaddress", ""}}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machinesCol.Update(
		bson.D{{"_id", s.state.docID(machine.Id())}},
		bson.D{{"$unset", bson.D{{"preferredprivateaddress", ""}}}},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) setPreferredAddressFields(c *gc.C, machine *Machine, addr string) {
	machinesCol, closer := s.state.getRawCollection(machinesC)
	defer closer()

	stateAddr := fromNetworkAddress(network.NewAddress(addr), OriginUnknown)
	err := machinesCol.Update(
		bson.D{{"_id", s.state.docID(machine.Id())}},
		bson.D{{"$set", bson.D{{"preferredpublicaddress", stateAddr}}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machinesCol.Update(
		bson.D{{"_id", s.state.docID(machine.Id())}},
		bson.D{{"$set", bson.D{{"preferredprivateaddress", stateAddr}}}},
	)
	c.Assert(err, jc.ErrorIsNil)
}

func assertMachineAddresses(c *gc.C, machine *Machine, publicAddress, privateAddress string) {
	err := machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	addr, err := machine.PublicAddress()
	if publicAddress != "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.Satisfies, network.IsNoAddress)
	}
	c.Assert(addr.Value, gc.Equals, publicAddress)
	privAddr, err := machine.PrivateAddress()
	if privateAddress != "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.Satisfies, network.IsNoAddress)
	}
	c.Assert(privAddr.Value, gc.Equals, privateAddress)
}

func (s *upgradesSuite) createMachinesWithAddresses(c *gc.C) []*Machine {
	_, err := s.state.AddMachine("quantal", JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.AddMachines([]MachineTemplate{
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
		{Series: "quantal", Jobs: []MachineJob{JobHostUnits}},
	}...)
	c.Assert(err, jc.ErrorIsNil)
	machines, err := s.state.AllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 4)

	m1 := machines[0]
	m2 := machines[1]
	m4 := machines[3]
	err = m1.SetProviderAddresses(network.NewAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)
	err = m2.SetMachineAddresses(network.NewAddress("10.0.0.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = m2.SetProviderAddresses(network.NewAddress("10.0.0.2"), network.NewAddress("8.8.4.4"))
	c.Assert(err, jc.ErrorIsNil)

	// Attempting to set the addresses of a dead machine will fail, so we
	// include a dead machine to make sure the upgrade step can cope.
	err = m4.SetProviderAddresses(network.NewAddress("8.8.8.8"))
	c.Assert(err, jc.ErrorIsNil)
	err = m4.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Delete the preferred address fields.
	for _, machine := range machines {
		s.removePreferredAddressFields(c, machine)
	}
	return machines
}

func (s *upgradesSuite) TestAddPreferredAddressesToMachines(c *gc.C) {
	machines := s.createMachinesWithAddresses(c)
	m1 := machines[0]
	m2 := machines[1]
	m3 := machines[2]

	err := AddPreferredAddressesToMachines(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertMachineAddresses(c, m1, "8.8.8.8", "8.8.8.8")
	assertMachineAddresses(c, m2, "8.8.4.4", "10.0.0.2")
	assertMachineAddresses(c, m3, "", "")
}

func (s *upgradesSuite) TestAddPreferredAddressesToMachinesIdempotent(c *gc.C) {
	machines := s.createMachinesWithAddresses(c)
	m1 := machines[0]
	m2 := machines[1]
	m3 := machines[2]

	err := AddPreferredAddressesToMachines(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertMachineAddresses(c, m1, "8.8.8.8", "8.8.8.8")
	assertMachineAddresses(c, m2, "8.8.4.4", "10.0.0.2")
	assertMachineAddresses(c, m3, "", "")

	err = AddPreferredAddressesToMachines(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertMachineAddresses(c, m1, "8.8.8.8", "8.8.8.8")
	assertMachineAddresses(c, m2, "8.8.4.4", "10.0.0.2")
	assertMachineAddresses(c, m3, "", "")
}

func (s *upgradesSuite) TestAddPreferredAddressesToMachinesUpdatesExistingFields(c *gc.C) {
	machines := s.createMachinesWithAddresses(c)
	m1 := machines[0]
	m2 := machines[1]
	m3 := machines[2]
	s.setPreferredAddressFields(c, m1, "1.1.2.2")
	s.setPreferredAddressFields(c, m2, "1.1.2.2")
	s.setPreferredAddressFields(c, m3, "1.1.2.2")

	assertMachineInitial := func(m *Machine) {
		err := m.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		addr, err := m.PublicAddress()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(addr.Value, gc.Equals, "1.1.2.2")
		addr, err = m.PrivateAddress()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(addr.Value, gc.Equals, "1.1.2.2")
	}
	assertMachineInitial(m1)
	assertMachineInitial(m2)
	assertMachineInitial(m3)

	err := AddPreferredAddressesToMachines(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertMachineAddresses(c, m1, "8.8.8.8", "8.8.8.8")
	assertMachineAddresses(c, m2, "8.8.4.4", "10.0.0.2")
	assertMachineAddresses(c, m3, "", "")
}

func (s *upgradesSuite) TestIPAddressesLife(c *gc.C) {
	addresses, closer := s.state.getRawCollection(ipaddressesC)
	defer closer()

	s.addMachineWithLife(c, "1", Alive)
	s.addMachineWithLife(c, "2", Alive)
	s.addMachineWithLife(c, "3", Dead)

	uuid := s.state.EnvironUUID()

	err := addresses.Insert(
		// this one should have Life set to Alive
		bson.D{
			{"_id", uuid + ":0.1.2.3"},
			{"env-uuid", uuid},
			{"subnetid", "foo"},
			{"machineid", s.state.docID("1")},
			{"interfaceid", "bam"},
			{"value", "0.1.2.3"},
			{"state", ""},
		},
		// this one should be untouched
		bson.D{
			{"_id", uuid + ":0.1.2.4"},
			{"env-uuid", uuid},
			{"life", Dead},
			{"subnetid", "foo"},
			{"machineid", s.state.docID("2")},
			{"interfaceid", "bam"},
			{"value", "0.1.2.4"},
			{"state", ""},
		},
		// this one should be set to Dead as the machine is Dead
		bson.D{
			{"_id", uuid + ":0.1.2.5"},
			{"env-uuid", uuid},
			{"subnetid", "foo"},
			{"machineid", s.state.docID("3")},
			{"interfaceid", "bam"},
			{"value", "0.1.2.5"},
			{"state", AddressStateAllocated},
		},
		// this one should be set to Dead as the machine is missing
		bson.D{
			{"_id", uuid + ":0.1.2.6"},
			{"env-uuid", uuid},
			{"subnetid", "foo"},
			{"machineid", s.state.docID("4")},
			{"interfaceid", "bam"},
			{"value", "0.1.2.6"},
			{"state", AddressStateAllocated},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = AddLifeFieldOfIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)

	ipAddr, err := s.state.IPAddress("0.1.2.4")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.Life(), gc.Equals, Dead)

	ipAddr, err = s.state.IPAddress("0.1.2.5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.Life(), gc.Equals, Dead)

	ipAddr, err = s.state.IPAddress("0.1.2.6")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.Life(), gc.Equals, Dead)

	doc := ipaddressDoc{}
	err = addresses.FindId(uuid + ":0.1.2.3").One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doc.Life, gc.Equals, Alive)
}

func (s *upgradesSuite) TestIPAddressLifeIdempotent(c *gc.C) {
	addresses, closer := s.state.getRawCollection(ipaddressesC)
	defer closer()

	s.addMachineWithLife(c, "1", Alive)
	uuid := s.state.EnvironUUID()

	err := addresses.Insert(
		// this one should have Life set to Alive
		bson.D{
			{"_id", uuid + ":0.1.2.3"},
			{"env-uuid", uuid},
			{"subnetid", "foo"},
			{"machineid", s.state.docID("1")},
			{"interfaceid", "bam"},
			{"value", "0.1.2.3"},
			{"state", ""},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = AddInstanceIdFieldOfIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)
	before, err := s.state.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)

	err = AddInstanceIdFieldOfIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)
	after, err := s.state.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(after, jc.DeepEquals, before)
}

func (s *upgradesSuite) TestIPAddressesInstanceId(c *gc.C) {
	addresses, closer := s.state.getRawCollection(ipaddressesC)
	defer closer()
	instances, closer2 := s.state.getRawCollection(instanceDataC)
	defer closer2()

	s.addMachineWithLife(c, "1", Alive)
	s.addMachineWithLife(c, "2", Alive)

	uuid := s.state.EnvironUUID()

	err := instances.Insert(
		bson.D{
			{"_id", uuid + ":1"},
			{"env-uuid", uuid},
			{"machineid", s.state.docID("1")},
			{"instanceid", "instance"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = addresses.Insert(
		// This address should have the instance ID set.
		bson.D{
			{"_id", uuid + ":0.1.2.3"},
			{"env-uuid", uuid},
			{"life", Alive},
			{"subnetid", "foo"},
			{"machineid", s.state.docID("1")},
			{"interfaceid", "bam"},
			{"value", "0.1.2.3"},
			{"state", AddressStateAllocated},
		},
		// This address won't have the instance ID set as there is no
		// instance for machine 2.
		bson.D{
			{"_id", uuid + ":0.1.2.4"},
			{"env-uuid", uuid},
			{"life", Alive},
			{"subnetid", "foo"},
			{"machineid", s.state.docID("2")},
			{"interfaceid", "bam"},
			{"value", "0.1.2.4"},
			{"state", AddressStateAllocated},
		},
		// This address won't have the instance ID set because it isn't
		// allocated.
		bson.D{
			{"_id", uuid + ":0.1.2.5"},
			{"env-uuid", uuid},
			{"life", Alive},
			{"subnetid", "foo"},
			{"interfaceid", "bam"},
			{"value", "0.1.2.5"},
			{"state", ""},
		},
		// This address won't have the instance ID set because the
		// machine referenced doesn't exist.
		bson.D{
			{"_id", uuid + ":0.1.2.6"},
			{"env-uuid", uuid},
			{"life", Alive},
			{"machineid", s.state.docID("3")},
			{"subnetid", "foo"},
			{"interfaceid", "bam"},
			{"value", "0.1.2.6"},
			{"state", AddressStateAllocated},
		},
	)

	c.Assert(err, jc.ErrorIsNil)

	err = AddInstanceIdFieldOfIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)

	ipAddr, err := s.state.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.Id("instance"))

	ipAddr, err = s.state.IPAddress("0.1.2.4")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.UnknownId)

	ipAddr, err = s.state.IPAddress("0.1.2.5")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.UnknownId)

	ipAddr, err = s.state.IPAddress("0.1.2.6")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.UnknownId)

	doc := ipaddressDoc{}
	err = addresses.FindId(uuid + ":0.1.2.3").One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(doc.InstanceId, gc.Equals, "instance")
}

func (s *upgradesSuite) TestIPAddressesInstanceIdIdempotent(c *gc.C) {
	addresses, closer := s.state.getRawCollection(ipaddressesC)
	defer closer()
	instances, closer2 := s.state.getRawCollection(instanceDataC)
	defer closer2()

	s.addMachineWithLife(c, "1", Alive)
	s.addMachineWithLife(c, "2", Alive)

	uuid := s.state.EnvironUUID()

	err := instances.Insert(
		bson.D{
			{"_id", uuid + ":1"},
			{"env-uuid", uuid},
			{"machineid", s.state.docID("1")},
			{"instanceid", "instance"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = addresses.Insert(
		// This address should have the instance ID set.
		bson.D{
			{"_id", uuid + ":0.1.2.3"},
			{"env-uuid", uuid},
			{"life", Alive},
			{"subnetid", "foo"},
			{"machineid", s.state.docID("1")},
			{"interfaceid", "bam"},
			{"value", "0.1.2.3"},
			{"state", AddressStateAllocated},
		},
	)

	c.Assert(err, jc.ErrorIsNil)

	err = AddInstanceIdFieldOfIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// and repeat
	err = AddInstanceIdFieldOfIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)

	ipAddr, err := s.state.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ipAddr.InstanceId(), gc.Equals, instance.Id("instance"))
}

func (s *upgradesSuite) TestIPAddressAddUUID(c *gc.C) {
	addresses, addrCloser := s.state.getRawCollection(ipaddressesC)
	defer addrCloser()
	instances, instanceCloser := s.state.getRawCollection(instanceDataC)
	defer instanceCloser()

	s.addMachineWithLife(c, "1", Alive)

	envUUID := s.state.EnvironUUID()

	err := instances.Insert(
		bson.D{
			{"_id", envUUID + ":1"},
			{"env-uuid", envUUID},
			{"machineid", "1"},
			{"instanceid", "instance"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = addresses.Insert(
		// Two addresses without UUID.
		bson.D{
			{"_id", envUUID + ":0.1.2.3"},
			{"env-uuid", envUUID},
			{"life", Alive},
			{"subnetid", "foo"},
			{"machineid", "1"},
			{"interfaceid", "bam"},
			{"value", "0.1.2.3"},
			{"state", AddressStateAllocated},
		},
		bson.D{
			{"_id", envUUID + ":0.1.2.4"},
			{"env-uuid", envUUID},
			{"life", Alive},
			{"subnetid", "foo"},
			{"machineid", "1"},
			{"interfaceid", "bam"},
			{"value", "0.1.2.4"},
			{"state", AddressStateAllocated},
		},
		// Two addresses with UUID.
		bson.D{
			{"_id", envUUID + ":0.1.2.5"},
			{"env-uuid", envUUID},
			{"uuid", "42424242-1111-2222-3333-0123456789ab"},
			{"life", Alive},
			{"subnetid", "foo"},
			{"machineid", "1"},
			{"interfaceid", "bam"},
			{"value", "0.1.2.5"},
			{"state", AddressStateAllocated},
		},
		bson.D{
			{"_id", envUUID + ":0.1.2.6"},
			{"env-uuid", envUUID},
			{"uuid", "42424242-4444-5555-6666-0123456789ab"},
			{"life", Alive},
			{"subnetid", "foo"},
			{"machineid", "1"},
			{"interfaceid", "bam"},
			{"value", "0.1.2.6"},
			{"state", AddressStateAllocated},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = AddUUIDToIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)

	ipAddr, err := s.state.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)
	uuid, err := ipAddr.UUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid.String(), jc.Satisfies, utils.IsValidUUIDString)

	ipAddr, err = s.state.IPAddress("0.1.2.4")
	c.Assert(err, jc.ErrorIsNil)
	uuid, err = ipAddr.UUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid.String(), jc.Satisfies, utils.IsValidUUIDString)

	ipAddr, err = s.state.IPAddress("0.1.2.5")
	c.Assert(err, jc.ErrorIsNil)
	uuid, err = ipAddr.UUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid.String(), gc.Equals, "42424242-1111-2222-3333-0123456789ab")

	ipAddr, err = s.state.IPAddress("0.1.2.6")
	c.Assert(err, jc.ErrorIsNil)
	uuid, err = ipAddr.UUID()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid.String(), gc.Equals, "42424242-4444-5555-6666-0123456789ab")
}

func (s *upgradesSuite) TestIPAddressAddUUIDIdempotent(c *gc.C) {
	addresses, closer := s.state.getRawCollection(ipaddressesC)
	defer closer()

	s.addMachineWithLife(c, "1", Alive)
	envUUID := s.state.EnvironUUID()

	err := addresses.Insert(
		bson.D{
			{"_id", envUUID + ":0.1.2.3"},
			{"env-uuid", envUUID},
			{"subnetid", "foo"},
			{"machineid", 1},
			{"interfaceid", "bam"},
			{"value", "0.1.2.3"},
			{"state", ""},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = AddUUIDToIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)
	before, err := s.state.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)

	err = AddUUIDToIPAddresses(s.state)
	c.Assert(err, jc.ErrorIsNil)
	after, err := s.state.IPAddress("0.1.2.3")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(after, jc.DeepEquals, before)
}

func (s *upgradesSuite) prepareEnvsForLeadership(c *gc.C, envs map[string][]string) []string {
	environments, closer := s.state.getRawCollection(environmentsC)
	defer closer()
	addEnvironment := func(envUUID string) {
		err := environments.Insert(bson.M{
			"_id": envUUID,
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	var expectedDocIDs []string
	services, closer := s.state.getRawCollection(servicesC)
	defer closer()
	addService := func(envUUID, name string) {
		err := services.Insert(bson.M{
			"_id":      envUUID + ":" + name,
			"env-uuid": envUUID,
			"name":     name,
		})
		c.Assert(err, jc.ErrorIsNil)
		expectedDocIDs = append(expectedDocIDs, envUUID+":"+leadershipSettingsKey(name))
	}

	// Use the helpers to set up the environments.
	for envUUID, svcs := range envs {
		if envUUID == "" {
			envUUID = s.state.EnvironUUID()
		} else {
			addEnvironment(envUUID)
		}
		for _, svc := range svcs {
			addService(envUUID, svc)
		}
	}

	return expectedDocIDs
}

func (s *upgradesSuite) readDocIDs(c *gc.C, coll, regex string) []string {
	settings, closer := s.state.getRawCollection(coll)
	defer closer()
	var docs []bson.M
	err := settings.Find(bson.D{{"_id", bson.D{{"$regex", regex}}}}).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	var actualDocIDs []string
	for _, doc := range docs {
		actualDocIDs = append(actualDocIDs, doc["_id"].(string))
	}
	return actualDocIDs
}

func (s *upgradesSuite) TestAddLeadershipSettingsDocs(c *gc.C) {
	expectedDocIDs := s.prepareEnvsForLeadership(c, map[string][]string{
		"": []string{"mediawiki", "postgresql"},
		"6983ac70-b0aa-45c5-80fe-9f207bbb18d9": []string{"foobar"},
		"7983ac70-b0aa-45c5-80fe-9f207bbb18d9": []string{"mysql"},
	})

	err := AddLeadershipSettingsDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)

	actualDocIDs := s.readDocIDs(c, settingsC, ".+#leader$")
	c.Assert(actualDocIDs, jc.SameContents, expectedDocIDs)
}

func (s *upgradesSuite) TestAddLeadershipSettingsFresh(c *gc.C) {
	err := AddLeadershipSettingsDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)

	actualDocIDs := s.readDocIDs(c, settingsC, ".+#leader$")
	c.Assert(actualDocIDs, gc.HasLen, 0)
}

func (s *upgradesSuite) TestAddLeadershipSettingsMultipleEmpty(c *gc.C) {
	s.prepareEnvsForLeadership(c, map[string][]string{
		"6983ac70-b0aa-45c5-80fe-9f207bbb18d9": nil,
		"7983ac70-b0aa-45c5-80fe-9f207bbb18d9": nil,
	})

	err := AddLeadershipSettingsDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)

	actualDocIDs := s.readDocIDs(c, settingsC, ".+#leader$")
	c.Assert(actualDocIDs, gc.HasLen, 0)
}

func (s *upgradesSuite) TestAddLeadershipSettingsIdempotent(c *gc.C) {
	s.prepareEnvsForLeadership(c, map[string][]string{
		"": []string{"mediawiki", "postgresql"},
		"6983ac70-b0aa-45c5-80fe-9f207bbb18d9": []string{"foobar"},
		"7983ac70-b0aa-45c5-80fe-9f207bbb18d9": []string{"mysql"},
	})

	originalIDs := s.readDocIDs(c, settingsC, ".+#leader$")
	c.Assert(originalIDs, gc.HasLen, 0)

	err := AddLeadershipSettingsDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)
	firstPassIDs := s.readDocIDs(c, settingsC, ".+#leader$")

	err = AddLeadershipSettingsDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)
	secondPassIDs := s.readDocIDs(c, settingsC, ".+#leader$")

	c.Check(firstPassIDs, jc.SameContents, secondPassIDs)
}

func (s *upgradesSuite) prepareEnvsForMachineBlockDevices(c *gc.C, envs map[string][]string) []string {
	environments, closer := s.state.getRawCollection(environmentsC)
	defer closer()
	addEnvironment := func(envUUID string) {
		err := environments.Insert(bson.M{
			"_id": envUUID,
		})
		c.Assert(err, jc.ErrorIsNil)
	}

	var expectedDocIDs []string
	machines, closer := s.state.getRawCollection(machinesC)
	defer closer()
	addMachine := func(envUUID, id string) {
		err := machines.Insert(bson.M{
			"_id":       envUUID + ":" + id,
			"env-uuid":  envUUID,
			"machineid": id,
		})
		c.Assert(err, jc.ErrorIsNil)
		expectedDocIDs = append(expectedDocIDs, envUUID+":"+id)
	}

	// Use the helpers to set up the environments.
	for envUUID, machines := range envs {
		if envUUID == "" {
			envUUID = s.state.EnvironUUID()
		} else {
			addEnvironment(envUUID)
		}
		for _, mId := range machines {
			addMachine(envUUID, mId)
		}
	}

	return expectedDocIDs
}

func (s *upgradesSuite) TestAddBlockDevicesDocs(c *gc.C) {
	expectedDocIDs := s.prepareEnvsForMachineBlockDevices(c, map[string][]string{
		"": []string{"1", "2"},
		"6983ac70-b0aa-45c5-80fe-9f207bbb18d9": []string{"1"},
		"7983ac70-b0aa-45c5-80fe-9f207bbb18d9": []string{"1"},
	})

	err := AddDefaultBlockDevicesDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)

	actualDocIDs := s.readDocIDs(c, blockDevicesC, "")
	c.Assert(actualDocIDs, jc.SameContents, expectedDocIDs)
}

func (s *upgradesSuite) TestAddBlockDevicesDocsFresh(c *gc.C) {
	err := AddDefaultBlockDevicesDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)

	actualDocIDs := s.readDocIDs(c, blockDevicesC, "")
	c.Assert(actualDocIDs, gc.HasLen, 0)
}

func (s *upgradesSuite) TestAddBlockDevicesDocsMultipleEmpty(c *gc.C) {
	s.prepareEnvsForMachineBlockDevices(c, map[string][]string{
		"6983ac70-b0aa-45c5-80fe-9f207bbb18d9": nil,
		"7983ac70-b0aa-45c5-80fe-9f207bbb18d9": nil,
	})

	err := AddDefaultBlockDevicesDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)

	actualDocIDs := s.readDocIDs(c, blockDevicesC, "")
	c.Assert(actualDocIDs, gc.HasLen, 0)
}

func (s *upgradesSuite) TestAddBlockDevicesDocsIdempotent(c *gc.C) {
	s.prepareEnvsForMachineBlockDevices(c, map[string][]string{
		"": []string{"1", "2"},
		"6983ac70-b0aa-45c5-80fe-9f207bbb18d9": []string{"1"},
		"7983ac70-b0aa-45c5-80fe-9f207bbb18d9": []string{"1"},
	})

	originalIDs := s.readDocIDs(c, blockDevicesC, "")
	c.Assert(originalIDs, gc.HasLen, 0)

	err := AddDefaultBlockDevicesDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)
	firstPassIDs := s.readDocIDs(c, blockDevicesC, "")

	err = AddDefaultBlockDevicesDocs(s.state)
	c.Assert(err, jc.ErrorIsNil)
	secondPassIDs := s.readDocIDs(c, blockDevicesC, "")

	c.Assert(firstPassIDs, jc.SameContents, secondPassIDs)
}

func (s *upgradesSuite) TestEnvUUIDMigrationFieldOrdering(c *gc.C) {
	// This tests a DB migration regression triggered by Go 1.3+'s
	// randomised map iteration feature. See LP #1451674.
	//
	// Here we ensure that the addEnvUUIDToEntityCollection helper
	// doesn't change the order of document fields. This is important
	// because MongoDB comparisons and txn assertions will not work as
	// expected if document field orders don't match.
	//
	// Several documents, each containing other documents in an array,
	// are inserted and then read back out to ensure that field
	// ordering hasn't changed.

	type address struct {
		Value       string `bson:"value"`
		AddressType string `bson:"addresstype"`
		NetworkName string `bson:"networkname"`
		Scope       string `bson:"networkscope"`
	}

	type fakeMachineDoc struct {
		DocID     string    `bson:"_id"`
		Series    string    `bson:"series"`
		Addresses []address `bson:"addresses"`
	}

	mdoc := fakeMachineDoc{
		Series: "foo",
		Addresses: []address{
			{
				Value:       "1.2.3.4",
				AddressType: "local",
				NetworkName: "foo",
				Scope:       "bar",
			},
			{
				Value:       "5.4.3.2",
				AddressType: "meta",
				NetworkName: "brie",
				Scope:       "cheese",
			},
		},
	}

	machines, close := s.state.getRawCollection(machinesC)
	defer close()
	for i := 0; i < 20; i++ {
		mdoc.DocID = fmt.Sprintf("%d", i)
		err := machines.Insert(mdoc)
		c.Assert(err, jc.ErrorIsNil)
	}

	err := addEnvUUIDToEntityCollection(s.state, machinesC, setOldID("machineid"))
	c.Assert(err, jc.ErrorIsNil)

	var outDocs []bson.D
	err = machines.Find(nil).All(&outDocs)
	c.Assert(err, jc.ErrorIsNil)

	expectedMachineFields := []string{"_id", "series", "addresses", "machineid", "env-uuid"}
	expectedAddressFields := []string{"value", "addresstype", "networkname", "networkscope"}
	for _, doc := range outDocs {
		for i, fieldName := range expectedMachineFields {
			c.Assert(doc[i].Name, gc.Equals, fieldName)
		}

		addresses := doc[2].Value.([]interface{})
		c.Assert(addresses, gc.HasLen, 2)
		for _, addressElem := range addresses {
			address := addressElem.(bson.D)
			for i, fieldName := range expectedAddressFields {
				c.Assert(address[i].Name, gc.Equals, fieldName)
			}
		}
	}
}

func (s *upgradesSuite) TestMoveServiceUnitSeqToSequence(c *gc.C) {
	svcC, closer := s.state.getRawCollection(servicesC)
	defer closer()

	err := svcC.Insert(
		bson.D{
			{"_id", s.state.docID("my-service")},
			{"unitseq", 7},
			{"env-uuid", s.state.EnvironUUID()},
			{"name", "my-service"},
		})
	c.Assert(err, jc.ErrorIsNil)
	err = MoveServiceUnitSeqToSequence(s.state)
	c.Assert(err, jc.ErrorIsNil)
	count, err := s.state.sequence("service-my-service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 7)

	var result map[string]interface{}
	err = svcC.Find(nil).Select(bson.M{"unitseq": 1}).One(&result)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result["unitseq"], gc.Equals, nil)
}

func (s *upgradesSuite) TestMoveServiceNotUnitSeq(c *gc.C) {
	svcC, closer := s.state.getRawCollection(servicesC)
	defer closer()

	err := svcC.Insert(
		bson.D{
			{"env-uuid", s.state.EnvironUUID()},
			{"name", "my-service"},
		})
	c.Assert(err, jc.ErrorIsNil)
	err = MoveServiceUnitSeqToSequence(s.state)
	c.Assert(err, jc.ErrorIsNil)
	count, err := s.state.sequence("service-my-service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 0)
}

func (s *upgradesSuite) TestMoveServiceUnitSeqToSequenceWithPreExistingSequence(c *gc.C) {
	_, err := s.state.sequence("service-my-service")

	svcC, closer := s.state.getRawCollection(servicesC)
	defer closer()

	err = svcC.Insert(
		bson.D{
			{"unitseq", 7},
			{"env-uuid", s.state.EnvironUUID()},
			{"name", "my-service"},
		})
	c.Assert(err, jc.ErrorIsNil)
	err = MoveServiceUnitSeqToSequence(s.state)
	c.Assert(err, jc.ErrorIsNil)
	count, err := s.state.sequence("service-my-service")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 7)
}

func (s *upgradesSuite) TestSetHostedEnvironCount(c *gc.C) {
	s.removeEnvCountDoc(c)

	s.makeEnvironment(c)
	s.makeEnvironment(c)
	s.makeEnvironment(c)
	SetHostedEnvironCount(s.state)

	//While there are 4 environments, the system environment should not be
	//counted.
	c.Assert(HostedEnvironCount(c, s.state), gc.Equals, 3)
}

func (s *upgradesSuite) TestSetHostedEnvironCountIdempotent(c *gc.C) {
	s.removeEnvCountDoc(c)

	s.makeEnvironment(c)
	s.makeEnvironment(c)
	s.makeEnvironment(c)
	SetHostedEnvironCount(s.state)
	SetHostedEnvironCount(s.state)

	c.Assert(HostedEnvironCount(c, s.state), gc.Equals, 3)
}

var index uint32

// We can't use factory.MakeEnvironment due to an import cycle.
func (s *upgradesSuite) makeEnvironment(c *gc.C) string {
	st := s.state

	env, err := st.Environment()
	c.Assert(err, jc.ErrorIsNil)

	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	ops := []txn.Op{createEnvironmentOp(
		s.state, env.Owner(),
		fmt.Sprintf("envname-%d", int(atomic.AddUint32(&index, 1))),
		uuid.String(),
		env.UUID())}

	err = st.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
	return uuid.String()
}

func (s *upgradesSuite) removeEnvCountDoc(c *gc.C) {
	err := s.state.runTransaction([]txn.Op{{
		C:      stateServersC,
		Id:     hostedEnvCountKey,
		Assert: txn.DocExists,
		Remove: true,
	}})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestMigrateLastLoginAndLastConnection(c *gc.C) {
	t := time.Now().Round(time.Second)
	fooUser := names.NewUserTag("foobar")
	barUser := names.NewUserTag("barfoo")

	s.addUsersForLastLoginAndConnection(c, &t, fooUser)
	s.addUsersForLastLoginAndConnection(c, nil, barUser)

	err := MigrateLastLoginAndLastConnection(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.assertLastLoginAndConnectionMigration(c, t, fooUser)

	// assert that no documents were added for a user who has never logged in
	// or connected.
	_, err = s.getDocMap(c, barUser.Id(), "userLastLogin")
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
	_, err = s.getDocMap(c, envUserID(barUser), "envUserLastConnection")
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func (s *upgradesSuite) TestMigrateLastLoginAndLastConnectionIdempotent(c *gc.C) {
	t := time.Now().Round(time.Second)
	fooUser := names.NewUserTag("foobar")
	barUser := names.NewUserTag("barfoo")

	s.addUsersForLastLoginAndConnection(c, &t, fooUser)
	s.addUsersForLastLoginAndConnection(c, nil, barUser)

	err := MigrateLastLoginAndLastConnection(s.state)
	c.Assert(err, jc.ErrorIsNil)
	err = MigrateLastLoginAndLastConnection(s.state)
	c.Assert(err, jc.ErrorIsNil)

	s.assertLastLoginAndConnectionMigration(c, t, fooUser)

	// assert that no documents were added for a user who has never logged in
	// or connected.
	_, err = s.getDocMap(c, barUser.Id(), "userLastLogin")
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
	_, err = s.getDocMap(c, envUserID(barUser), "envUserLastConnection")
	c.Assert(err, gc.Equals, mgo.ErrNotFound)
}

func (s *upgradesSuite) addUsersForLastLoginAndConnection(c *gc.C, t *time.Time, user names.UserTag) {
	userID := user.Id()
	oldEnvUserID := envUserID(user)

	oldUserDoc := bson.M{
		"_id":          userID,
		"env-uuid":     "envuuid456",
		"displayname":  "foo bar",
		"deactivated":  false,
		"passwordhash": "hash",
		"passwordsalt": "salt",
		"createdby":    "creator",
		"datecreated":  t,
		"lastlogin":    t,
	}

	oldEnvUserDoc := bson.M{
		"_id":            oldEnvUserID,
		"env-uuid":       "envuuid123",
		"user":           "username@local",
		"displayname":    "ignored",
		"createdby":      "ignored@local",
		"datecreated":    t,
		"lastconnection": t,
	}

	ops := []txn.Op{
		{
			C:      "users",
			Id:     userID,
			Assert: txn.DocMissing,
			Insert: oldUserDoc,
		},
		{
			C:      "envusers",
			Id:     oldEnvUserID,
			Assert: txn.DocMissing,
			Insert: oldEnvUserDoc,
		},
	}
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) assertLastLoginAndConnectionMigration(c *gc.C, t time.Time, user names.UserTag) {
	userID := user.Id()
	oldEnvUserID := envUserID(user)

	// check to see if lastlogin field is removed
	userMap, err := s.getDocMap(c, userID, "users")
	c.Assert(err, jc.ErrorIsNil)
	_, keyExists := userMap["lastlogin"]
	c.Assert(keyExists, jc.IsFalse)

	// check to see if lastconnection field is removed
	envUserMap, err := s.getDocMap(c, oldEnvUserID, "envusers")
	c.Assert(err, jc.ErrorIsNil)
	_, keyExists = envUserMap["lastconnection"]
	c.Assert(keyExists, jc.IsFalse)

	// check to see if lastlogin doc is added
	lastLoginMap, err := s.getDocMap(c, userID, "userLastLogin")
	c.Assert(err, jc.ErrorIsNil)
	lastLogin, keyExists := lastLoginMap["last-login"]
	c.Assert(keyExists, jc.IsTrue)
	c.Assert(lastLogin.(time.Time).UTC(), gc.Equals, t.UTC())
	envUUID, keyExists := lastLoginMap["env-uuid"]
	c.Assert(keyExists, jc.IsTrue)
	c.Assert(envUUID, gc.Equals, "envuuid456")

	// check to see if lastconnection doc is added
	lastConnMap, err := s.getDocMap(c, oldEnvUserID, "envUserLastConnection")
	c.Assert(err, jc.ErrorIsNil)
	lastConn, keyExists := lastConnMap["last-connection"]
	c.Assert(keyExists, jc.IsTrue)
	c.Assert(lastConn.(time.Time).UTC(), gc.Equals, t.UTC())
	envUUID, keyExists = lastConnMap["env-uuid"]
	c.Assert(keyExists, jc.IsTrue)
	c.Assert(envUUID, gc.Equals, "envuuid123")
	username, keyExists := lastConnMap["user"]
	c.Assert(keyExists, jc.IsTrue)
	c.Assert(username, gc.Equals, "username@local")
}

func (s *upgradesSuite) getDocMap(c *gc.C, docID, collection string) (map[string]interface{}, error) {
	docMap := map[string]interface{}{}
	coll, closer := s.state.getRawCollection(collection)
	defer closer()
	err := coll.Find(bson.D{{"_id", docID}}).One(&docMap)
	return docMap, err
}

func (s *upgradesSuite) TestAddMissingEnvUUIDOnStatuses(c *gc.C) {
	statuses, closer := s.state.getRawCollection(statusesC)
	defer closer()

	err := statuses.Insert(
		// This record should be left untouched.
		bson.D{
			{"_id", "uuid0:bar"},
			{"env-uuid", "uuid0"},
		},
		// These records should have their env-uuid fields set.
		bson.D{
			{"_id", "uuid0:foo"},
			{"env-uuid", ""},
		},
		bson.D{
			{"_id", "uuid1:foo"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = AddMissingEnvUUIDOnStatuses(s.state)
	c.Assert(err, jc.ErrorIsNil)

	var docs []statusDoc
	err = statuses.Find(nil).Sort("_id").All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, jc.DeepEquals, []statusDoc{
		{EnvUUID: "uuid0"},
		{EnvUUID: "uuid0"},
		{EnvUUID: "uuid1"},
	})
}

func (s *upgradesSuite) TestAddMissingServiceStatuses(c *gc.C) {

	// Add two environments.
	uuid0 := utils.MustNewUUID().String()
	uuid1 := utils.MustNewUUID().String()
	environments, closer := s.state.getRawCollection(environmentsC)
	defer closer()
	err := environments.Insert(
		bson.D{
			{"_id", uuid0},
		},
		bson.D{
			{"_id", uuid1},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Create four services across those environments.
	services, closer := s.state.getRawCollection(servicesC)
	defer closer()
	err = services.Insert(
		bson.D{
			{"_id", uuid0 + ":foo"},
			{"name", "foo"},
			{"env-uuid", uuid0},
		},
		bson.D{
			{"_id", uuid0 + ":bar"},
			{"name", "bar"},
			{"env-uuid", uuid0},
		},
		bson.D{
			{"_id", uuid1 + ":ping"},
			{"name", "ping"},
			{"env-uuid", uuid1},
		},
		bson.D{
			{"_id", uuid1 + ":pong"},
			{"name", "pong"},
			{"env-uuid", uuid1},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Create status for one service in each environment.
	statuses, closer := s.state.getRawCollection(statusesC)
	defer closer()
	err = statuses.Insert(
		bson.D{
			{"_id", uuid0 + ":s#foo"},
			{"env-uuid", uuid0},
			{"status", "untouched"},
		},
		bson.D{
			{"_id", uuid1 + ":s#pong"},
			{"env-uuid", uuid1},
			{"status", "untouched"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Run the upgrade.
	err = AddMissingServiceStatuses(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// Check we now have the expected number of statuses.
	count, err := statuses.Find(nil).Count()
	c.Check(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 4)

	// Check the original statuses were preserved.
	checkUntouched := func(id string) {
		var doc statusDoc
		err = statuses.FindId(id).One(&doc)
		c.Check(err, jc.ErrorIsNil)
		c.Check(doc.Status, gc.Equals, Status("untouched"))
	}
	checkUntouched(uuid0 + ":s#foo")
	checkUntouched(uuid1 + ":s#pong")

	// Check new statuses were inserted.
	checkStatusDoc := func(doc statusDoc, expectNeverSet bool) {
		c.Check(doc.Status, gc.Equals, StatusUnknown)
		c.Check(doc.StatusInfo, gc.Equals, "Waiting for agent initialization to finish")
		c.Check(doc.StatusData, gc.DeepEquals, map[string]interface{}{})
		c.Check(doc.Updated, gc.NotNil)
		c.Check(doc.NeverSet, gc.Equals, expectNeverSet)
	}

	checkStatusInserted := func(id string) {
		var doc statusDoc
		err = statuses.FindId(id).One(&doc)
		c.Check(err, jc.ErrorIsNil)
		checkStatusDoc(doc, true)
	}
	checkStatusInserted(uuid0 + ":s#bar")
	checkStatusInserted(uuid1 + ":s#ping")

	// Check status history docs were inserted.
	history, closer := s.state.getRawCollection(statusesHistoryC)
	defer closer()

	checkHistoryInserted := func(envUUID, entityid string) {
		var doc statusDoc
		err = history.Find(bson.D{{
			"globalkey", entityid,
		}, {
			"env-uuid", envUUID,
		}}).One(&doc)
		c.Check(err, jc.ErrorIsNil)
		checkStatusDoc(doc, false)
	}
	checkHistoryInserted(uuid0, "s#bar")
	checkHistoryInserted(uuid1, "s#ping")
}

func unsetField(st *State, id, collection, field string) error {
	return st.runTransaction(
		[]txn.Op{{
			C:      collection,
			Id:     id,
			Update: bson.D{{"$unset", bson.D{{field, nil}}}},
		},
		})
}

func setupForStorageTesting(s *upgradesSuite, c *gc.C, kind, pool string) func() error {
	pm := poolmanager.New(NewStateSettings(s.state))
	_, err := pm.Create(pool, provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)

	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := AddTestingCharm(c, s.state, "storage-"+kind)
	storage := map[string]StorageConstraints{
		"data": StorageConstraints{Pool: pool, Size: 1024, Count: 1},
	}
	service := AddTestingServiceWithStorage(c, s.state, "storage-"+kind, ch, s.owner, storage)

	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.state.AssignUnit(unit, AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	return service.Destroy
}

func assertVolumeAttachments(s *State, c *gc.C, expected int) *volume {
	volumes, err := s.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes, gc.HasLen, 1)
	vol := volumes[0].(*volume)
	attCount := vol.doc.AttachmentCount
	c.Assert(attCount, gc.Equals, expected)
	return vol
}

func (s *upgradesSuite) TestAddAttachmentToVolumes(c *gc.C) {
	cleanup := setupForStorageTesting(s, c, "block", "loop")
	defer cleanup()
	vol := assertVolumeAttachments(s.state, c, 1)

	id := vol.doc.DocID
	err := unsetField(s.state, id, volumesC, "attachmentcount")
	c.Assert(err, jc.ErrorIsNil)
	assertVolumeAttachments(s.state, c, 0)

	err = AddVolumeAttachmentCount(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertVolumeAttachments(s.state, c, 1)
}

func assertFilesystemAttachments(s *State, c *gc.C, expected int) *filesystem {
	filesystems, err := s.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystems, gc.HasLen, 1)
	fs := filesystems[0].(*filesystem)
	attCount := fs.doc.AttachmentCount
	c.Assert(attCount, gc.Equals, expected)
	return fs
}

func (s *upgradesSuite) TestAddAttachmentToFilesystems(c *gc.C) {
	cleanup := setupForStorageTesting(s, c, "filesystem", "loop")
	defer cleanup()

	fs := assertFilesystemAttachments(s.state, c, 1)

	id := fs.doc.DocID

	err := unsetField(s.state, id, filesystemsC, "attachmentcount")
	c.Assert(err, jc.ErrorIsNil)

	assertFilesystemAttachments(s.state, c, 0)

	err = AddFilesystemsAttachmentCount(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertFilesystemAttachments(s.state, c, 1)
}

func assertVolumeBinding(s *State, c *gc.C, expected string) *volume {
	volumes, err := s.AllVolumes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumes, gc.HasLen, 1)
	vol := volumes[0].(*volume)
	binding := vol.doc.Binding
	c.Assert(binding, gc.Equals, expected)
	return vol
}

func setupMachineBoundStorageTests(c *gc.C, st *State) (*Machine, Volume, Filesystem, func() error) {
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType, provider.RootfsProviderType)
	// Make an unprovisioned machine with storage for tests to use.
	// TODO(axw) extend testing/factory to allow creating unprovisioned
	// machines.
	m, err := st.AddOneMachine(MachineTemplate{
		Series: "quantal",
		Jobs:   []MachineJob{JobHostUnits},
		Volumes: []MachineVolumeParams{
			{Volume: VolumeParams{Pool: "loop", Size: 2048}},
		},
		Filesystems: []MachineFilesystemParams{
			{Filesystem: FilesystemParams{Pool: "rootfs", Size: 2048}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	va, err := m.VolumeAttachments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(va, gc.HasLen, 1)
	v, err := st.Volume(va[0].Volume())
	c.Assert(err, jc.ErrorIsNil)

	fa, err := st.MachineFilesystemAttachments(m.MachineTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fa, gc.HasLen, 1)
	f, err := st.Filesystem(fa[0].Filesystem())
	c.Assert(err, jc.ErrorIsNil)

	return m, v, f, m.Destroy
}

func (s *upgradesSuite) TestAddBindingToVolumesFilesystemBound(c *gc.C) {
	cleanup := setupForStorageTesting(s, c, "filesystem", "loop")
	defer cleanup()
	vol := assertVolumeBinding(s.state, c, "filesystem-0-0")

	id := vol.doc.DocID
	err := unsetField(s.state, id, volumesC, "binding")
	c.Assert(err, jc.ErrorIsNil)

	assertVolumeBinding(s.state, c, "")

	err = AddBindingToVolumes(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertVolumeBinding(s.state, c, "filesystem-0-0")
}

func (s *upgradesSuite) TestAddBindingToVolumesStorageBound(c *gc.C) {
	cleanup := setupForStorageTesting(s, c, "block", "loop")
	defer cleanup()
	vol := assertVolumeBinding(s.state, c, "storage-data-0")

	id := vol.doc.DocID
	err := unsetField(s.state, id, volumesC, "binding")
	c.Assert(err, jc.ErrorIsNil)

	assertVolumeBinding(s.state, c, "")

	err = AddBindingToVolumes(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertVolumeBinding(s.state, c, "storage-data-0")
}

func (s *upgradesSuite) TestAddBindingToVolumesMachineBound(c *gc.C) {
	_, _, _, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()
	vol := assertVolumeBinding(s.state, c, "machine-0")

	id := vol.doc.DocID
	err := unsetField(s.state, id, volumesC, "binding")
	c.Assert(err, jc.ErrorIsNil)

	assertVolumeBinding(s.state, c, "")

	err = AddBindingToVolumes(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertVolumeBinding(s.state, c, "machine-0")
}

func assertFilesystemBinding(s *State, c *gc.C, expected string) *filesystem {
	filesystems, err := s.AllFilesystems()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(filesystems, gc.HasLen, 1)
	fs := filesystems[0].(*filesystem)
	binding := fs.doc.Binding
	c.Assert(binding, gc.Equals, expected)
	return fs
}

func (s *upgradesSuite) TestAddBindingToFilesystemsStorageBound(c *gc.C) {
	cleanup := setupForStorageTesting(s, c, "filesystem", "loop")
	defer cleanup()
	fs := assertFilesystemBinding(s.state, c, "storage-data-0")

	id := fs.doc.DocID
	err := unsetField(s.state, id, filesystemsC, "binding")
	c.Assert(err, jc.ErrorIsNil)

	assertFilesystemBinding(s.state, c, "")

	err = AddBindingToFilesystems(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertFilesystemBinding(s.state, c, "storage-data-0")
}

func (s *upgradesSuite) TestAddVolumeStatus(c *gc.C) {
	_, volume, _, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	removeStatusDoc(c, s.state, volume)
	_, err := volume.Status()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	s.assertAddVolumeStatus(c, volume, StatusPending)
}

func (s *upgradesSuite) TestAddVolumeStatusDoesNotOverwrite(c *gc.C) {
	_, volume, _, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	err := volume.SetStatus(StatusDestroying, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAddVolumeStatus(c, volume, StatusDestroying)
}

func (s *upgradesSuite) TestAddVolumeStatusProvisioned(c *gc.C) {
	_, volume, _, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	err := s.state.SetVolumeInfo(volume.VolumeTag(), VolumeInfo{
		VolumeId: "vol",
	})
	c.Assert(err, jc.ErrorIsNil)
	removeStatusDoc(c, s.state, volume)
	s.assertAddVolumeStatus(c, volume, StatusAttaching)
}

func (s *upgradesSuite) TestAddVolumeStatusAttached(c *gc.C) {
	machine, volume, _, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	err := machine.SetProvisioned("fake", "fake", nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetVolumeInfo(volume.VolumeTag(), VolumeInfo{
		VolumeId: "vol",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetVolumeAttachmentInfo(
		machine.MachineTag(),
		volume.VolumeTag(),
		VolumeAttachmentInfo{},
	)
	c.Assert(err, jc.ErrorIsNil)

	removeStatusDoc(c, s.state, volume)
	s.assertAddVolumeStatus(c, volume, StatusAttached)
}

func (s *upgradesSuite) TestAddFilesystemStatus(c *gc.C) {
	_, _, filesystem, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	removeStatusDoc(c, s.state, filesystem)
	_, err := filesystem.Status()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	s.assertAddFilesystemStatus(c, filesystem, StatusPending)
}

func (s *upgradesSuite) TestAddFilesystemStatusDoesNotOverwrite(c *gc.C) {
	_, _, filesystem, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	err := filesystem.SetStatus(StatusDestroying, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAddFilesystemStatus(c, filesystem, StatusDestroying)
}

func (s *upgradesSuite) TestAddFilesystemStatusProvisioned(c *gc.C) {
	_, _, filesystem, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	err := s.state.SetFilesystemInfo(filesystem.FilesystemTag(), FilesystemInfo{
		FilesystemId: "fs",
	})
	c.Assert(err, jc.ErrorIsNil)
	removeStatusDoc(c, s.state, filesystem)
	s.assertAddFilesystemStatus(c, filesystem, StatusAttaching)
}

func (s *upgradesSuite) TestAddFilesystemStatusAttached(c *gc.C) {
	machine, _, filesystem, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	err := machine.SetProvisioned("fake", "fake", nil)
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetFilesystemInfo(filesystem.FilesystemTag(), FilesystemInfo{
		FilesystemId: "fs",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.state.SetFilesystemAttachmentInfo(
		machine.MachineTag(),
		filesystem.FilesystemTag(),
		FilesystemAttachmentInfo{},
	)
	c.Assert(err, jc.ErrorIsNil)

	removeStatusDoc(c, s.state, filesystem)
	s.assertAddFilesystemStatus(c, filesystem, StatusAttached)
}

func (s *upgradesSuite) assertAddVolumeStatus(c *gc.C, volume Volume, expect Status) {
	err := AddVolumeStatus(s.state)
	c.Assert(err, jc.ErrorIsNil)

	info, err := volume.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Status, gc.Equals, expect)
}

func (s *upgradesSuite) assertAddFilesystemStatus(c *gc.C, filesystem Filesystem, expect Status) {
	err := AddFilesystemStatus(s.state)
	c.Assert(err, jc.ErrorIsNil)

	info, err := filesystem.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info.Status, gc.Equals, expect)
}

func removeStatusDoc(c *gc.C, st *State, g GlobalEntity) {
	op := removeStatusOp(st, g.globalKey())
	err := st.runTransaction([]txn.Op{op})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestChangeIdsFromSeqToAuto(c *gc.C) {
	// Crate a new environment
	uuid0 := s.makeEnvironment(c)
	// Insert basic test data
	sHistory, closer := s.state.getRawCollection(statusesHistoryC)
	defer closer()
	err := sHistory.Insert(
		bson.D{
			{"_id", "1"},
			{"env-uuid", uuid0},
			{"status", "status 1"},
		},
		bson.D{
			{"_id", "2"},
			{"env-uuid", uuid0},
			{"status", "status 2"},
		},
		bson.D{
			{"_id", "3"},
			{"env-uuid", uuid0},
			{"status", "status 3"},
		},
		bson.D{
			{"_id", "4"},
			{"env-uuid", uuid0},
			{"status", "status 4"},
		},
		bson.D{
			{"_id", "this is not a seq number"},
			{"env-uuid", uuid0},
			{"status", "status not sequence"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// assert that the ids are in fact ints.
	for i := 1; i < 5; i++ {
		logger.Infof("checking that id %d is correctly in status history", i)
		n, err := sHistory.FindId(fmt.Sprintf("%d", i)).Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 1)
	}

	// Get State for a particular env simulating runForAllEnvStates
	envSt, err := s.state.ForEnviron(names.NewEnvironTag(uuid0))
	defer envSt.Close()
	c.Assert(err, jc.ErrorIsNil)

	err = changeIdsFromSeqToAuto(envSt)
	c.Assert(err, jc.ErrorIsNil)

	// assert that there are no more int ids.
	for i := 1; i < 5; i++ {
		logger.Infof("checking that id %d is no longer in status history", i)
		n, err := sHistory.FindId(fmt.Sprintf("%d", i)).Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 0)
	}
	// assert that the non int id was left untouched.
	var doc bson.M
	err = sHistory.FindId("this is not a seq number").One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	status := doc["status"].(string)
	c.Assert(status, gc.Equals, "status not sequence")

	// assert that the statuses left are correct.
	var docs []bson.M
	err = sHistory.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 5)
	statuses := make([]string, len(docs))
	for i, d := range docs {
		statuses[i] = d["status"].(string)
	}

	c.Assert(statuses, jc.SameContents, []string{"status 1", "status 2", "status 3", "status 4", "status not sequence"})
}

func (s *upgradesSuite) TestChangeStatusHistoryUpdatedFromTimeToInt64(c *gc.C) {
	uuid0 := s.makeEnvironment(c)
	c.Logf("UUID0 : %v", uuid0)
	uuid1 := s.makeEnvironment(c)
	c.Logf("UUID1 : %v", uuid1)
	sHistory, closer := s.state.getRawCollection(statusesHistoryC)
	defer closer()
	epoch := time.Unix(0, 0).UTC()
	// lets create a time with fine precision to check its left
	// untouched.
	nanoTime := epoch.Add(3 * time.Hour).Add(4 * time.Nanosecond).UTC().UnixNano()
	err := sHistory.Insert(
		bson.D{
			{"_id", "0"},
			{"test_id", "0"},
			{"env-uuid", uuid0},
			{"updated", epoch.Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", "1"},
			{"test_id", "1"},
			{"env-uuid", uuid0},
			{"updated", epoch.Add(1 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", "2"},
			{"test_id", "2"},
			{"env-uuid", uuid0},
			{"updated", epoch.Add(2 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", "3"},
			{"test_id", "3"},
			{"env-uuid", uuid1},
			{"updated", epoch.Add(3 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", "4"},
			{"test_id", "4"},
			{"env-uuid", uuid1},
			{"updated", epoch.Add(4 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", "5"},
			{"test_id", "5"},
			{"env-uuid", uuid1},
			{"updated", epoch.Add(5 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", "6"},
			{"test_id", "6"},
			{"env-uuid", uuid0},
			{"updated", nanoTime},
		},
		bson.D{
			{"_id", "7"},
			{"test_id", "7"},
			{"env-uuid", uuid1},
			{"updated", nanoTime},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = ChangeStatusHistoryUpdatedType(s.state)
	c.Assert(err, jc.ErrorIsNil)
	var docs []bson.M
	sHistory.Find(nil).All(&docs)
	logger.Debugf("%v", docs)

	var doc bson.M
	for i := 0; i < 6; i++ {
		logger.Debugf("checking entry %d", i)
		err := sHistory.Find(bson.M{"test_id": fmt.Sprintf("%d", i)}).One(&doc)
		c.Assert(err, jc.ErrorIsNil)

		logger.Debugf("checking doc: %v", doc)
		updatedTime, ok := doc["updated"].(int64)
		c.Assert(ok, jc.IsTrue)
		// time stored in mongo native format will have lost precision
		c.Assert(updatedTime, gc.Equals, epoch.Add(time.Duration(i)*time.Hour).UTC().UnixNano())

		newId, ok := doc["_id"].(bson.ObjectId)
		c.Assert(ok, jc.IsTrue)
		c.Assert(newId, gc.Not(gc.Equals), fmt.Sprintf("%d", i))
	}
	sHistory.Find(bson.M{"test_id": "6"}).One(&doc)
	updatedTime, ok := doc["updated"].(int64)
	c.Assert(ok, jc.IsTrue)
	c.Assert(updatedTime, gc.Equals, nanoTime)
	newId, ok := doc["_id"].(bson.ObjectId)
	c.Assert(ok, jc.IsTrue)
	c.Assert(newId, gc.Not(gc.Equals), "6")

	sHistory.Find(bson.M{"test_id": "7"}).One(&doc)
	updatedTime, ok = doc["updated"].(int64)
	c.Assert(ok, jc.IsTrue)
	c.Assert(updatedTime, gc.Equals, nanoTime)
	newId, ok = doc["_id"].(bson.ObjectId)
	c.Assert(ok, jc.IsTrue)
	c.Assert(newId, gc.Not(gc.Equals), "7")

}

func (s *upgradesSuite) TestChangeStatusUpdatedFromTimeToInt64(c *gc.C) {
	uuid0 := s.makeEnvironment(c)
	uuid1 := s.makeEnvironment(c)
	statuses, closer := s.state.getRawCollection(statusesC)
	defer closer()
	epoch := time.Unix(0, 0).UTC()
	// lets create a time with fine precision to check its left
	// untouched.
	nanoTime := epoch.Add(3 * time.Hour).Add(4 * time.Nanosecond).UTC().UnixNano()
	err := statuses.Insert(
		bson.D{
			{"_id", uuid0 + ":0"},
			{"test_id", "0"},
			{"env-uuid", uuid0},
			{"updated", epoch.Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", uuid0 + ":1"},
			{"test_id", "1"},
			{"env-uuid", uuid0},
			{"updated", epoch.Add(1 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", uuid0 + ":2"},
			{"test_id", "2"},
			{"env-uuid", uuid0},
			{"updated", epoch.Add(2 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", uuid1 + ":3"},
			{"test_id", "3"},
			{"env-uuid", uuid1},
			{"updated", epoch.Add(3 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", uuid1 + ":4"},
			{"test_id", "4"},
			{"env-uuid", uuid1},
			{"updated", epoch.Add(4 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", uuid1 + ":5"},
			{"test_id", "5"},
			{"env-uuid", uuid1},
			{"updated", epoch.Add(5 * time.Hour).Add(2 * time.Nanosecond)},
		},
		bson.D{
			{"_id", uuid0 + ":6"},
			{"test_id", "6"},
			{"env-uuid", uuid0},
			{"updated", nanoTime},
		},
		bson.D{
			{"_id", uuid1 + ":7"},
			{"test_id", "7"},
			{"env-uuid", uuid1},
			{"updated", nanoTime},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = ChangeStatusUpdatedType(s.state)
	c.Assert(err, jc.ErrorIsNil)

	var doc bson.M
	for i := 0; i < 6; i++ {
		logger.Debugf("checking entry %d", i)
		err := statuses.Find(bson.M{"test_id": fmt.Sprintf("%d", i)}).One(&doc)
		c.Assert(err, jc.ErrorIsNil)

		logger.Debugf("checking doc: %v", doc)
		updatedTime, ok := doc["updated"].(int64)
		c.Assert(ok, jc.IsTrue)
		// time stored in mongo native format will have lost precision
		c.Assert(updatedTime, gc.Equals, epoch.Add(time.Duration(i)*time.Hour).UTC().UnixNano())
	}
	statuses.Find(bson.M{"test_id": "6"}).One(&doc)
	updatedTime, ok := doc["updated"].(int64)
	c.Assert(ok, jc.IsTrue)
	c.Assert(updatedTime, gc.Equals, nanoTime)

	statuses.Find(bson.M{"test_id": "7"}).One(&doc)
	updatedTime, ok = doc["updated"].(int64)
	c.Assert(ok, jc.IsTrue)
	c.Assert(updatedTime, gc.Equals, nanoTime)
}

func (s *upgradesSuite) TestChangeEntityIdToGlobalKey(c *gc.C) {
	uuid0 := s.makeEnvironment(c)
	uuid1 := s.makeEnvironment(c)
	sHistory, closer := s.state.getRawCollection(statusesHistoryC)
	defer closer()

	err := sHistory.Insert(
		bson.D{
			{"_id", "0"},
			{"env-uuid", uuid0},
			{"entityid", "global0"},
		},
		bson.D{
			{"_id", uuid0 + ":1"},
			{"env-uuid", uuid0},
			{"entityid", "global1"},
		},
		bson.D{
			{"_id", uuid0 + ":2"},
			{"env-uuid", uuid0},
			{"globalkey", "global2"},
		},
		bson.D{
			{"_id", uuid0 + ":3"},
			{"env-uuid", uuid0},
			{"globalkey", "global3"},
		},
		bson.D{
			{"_id", uuid1 + ":4"},
			{"env-uuid", uuid1},
			{"entityid", "global4"},
		},
		bson.D{
			{"_id", uuid1 + ":5"},
			{"env-uuid", uuid1},
			{"entityid", "global5"},
		},
		bson.D{
			{"_id", uuid1 + ":6"},
			{"env-uuid", uuid1},
			{"globalkey", "global6"},
		},
		bson.D{
			{"_id", uuid1 + ":7"},
			{"env-uuid", uuid1},
			{"globalkey", "global7"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	var docs []bson.M
	err = sHistory.Find(bson.D{{
		"entityid", bson.D{{"$exists", true}}}}).All(&docs)
	c.Assert(docs, gc.HasLen, 4)

	err = ChangeStatusHistoryEntityId(s.state)
	c.Assert(err, jc.ErrorIsNil)

	err = sHistory.Find(bson.D{{
		"entityid", bson.D{{"$exists", true}}}}).All(&docs)
	c.Assert(docs, gc.HasLen, 0)

	var doc bson.M

	logger.Debugf("checking global key 0")
	err = sHistory.FindId(fmt.Sprintf("%s:0", uuid0)).One(&doc)
	c.Assert(err, gc.ErrorMatches, "not found")
	err = sHistory.Find(bson.M{"globalkey": "global0"}).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	ID, ok := doc["_id"].(bson.ObjectId)
	c.Assert(ok, jc.IsTrue)
	c.Assert(ID, gc.Not(gc.Equals), "0")

	for i := 1; i < 8; i++ {
		u := uuid0
		if i > 3 {
			u = uuid1
		}
		logger.Debugf("checking global key %d", i)
		err := sHistory.FindId(fmt.Sprintf("%s:%d", u, i)).One(&doc)
		c.Assert(err, jc.ErrorIsNil)
		globalKey, ok := doc["globalkey"].(string)
		c.Assert(ok, jc.IsTrue)
		c.Assert(globalKey, gc.Equals, fmt.Sprintf("global%d", i))
	}
}

func (s *upgradesSuite) TestMigrateSettingsSchema(c *gc.C) {
	// Insert test documents.
	settingsColl, closer := s.state.getRawCollection(settingsC)
	defer closer()
	err := settingsColl.Insert(
		bson.D{
			// Post-env-uuid migration, with no settings.
			{"_id", "1"},
			{"env-uuid", "env-uuid"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
		},
		bson.D{
			// Post-env-uuid migration, with settings. One
			// of the settings is called "settings", and
			// one "version".
			{"_id", "2"},
			{"env-uuid", "env-uuid"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
			{"settings", int64(123)},
			{"version", "onetwothree"},
		},
		bson.D{
			// Pre-env-uuid migration, with no settings.
			{"_id", "3"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
		},
		bson.D{
			// Pre-env-uuid migration, with settings.
			{"_id", "4"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
			{"settings", int64(123)},
			{"version", "onetwothree"},
		},
		bson.D{
			// Already migrated, with no settings.
			{"_id", "5"},
			{"env-uuid", "env-uuid"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
			{"version", int64(98)},
			{"settings", map[string]interface{}{}},
		},
		bson.D{
			// Already migrated, with settings.
			{"_id", "6"},
			{"env-uuid", "env-uuid"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
			{"version", int64(98)},
			{"settings", bson.D{
				{"settings", int64(123)},
				{"version", "onetwothree"},
			}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Expected docs, excluding txn-queu which we cannot predict.
	expected := []bson.M{{
		"_id":       "1",
		"env-uuid":  "env-uuid",
		"txn-revno": int64(100),
		"settings":  bson.M{},
		"version":   int64(99),
	}, {
		"_id":       "2",
		"env-uuid":  "env-uuid",
		"txn-revno": int64(101),
		"settings": bson.M{
			"settings": int64(123),
			"version":  "onetwothree",
		},
		"version": int64(99),
	}, {
		"_id":       "3",
		"txn-revno": int64(100),
		"settings":  bson.M{},
		"version":   int64(99),
	}, {
		"_id":       "4",
		"txn-revno": int64(101),
		"settings": bson.M{
			"settings": int64(123),
			"version":  "onetwothree",
		},
		"version": int64(99),
	}, {
		"_id":       "5",
		"env-uuid":  "env-uuid",
		"txn-revno": int64(99),
		"version":   int64(98),
		"settings":  bson.M{},
	}, {
		"_id":       "6",
		"env-uuid":  "env-uuid",
		"txn-revno": int64(99),
		"version":   int64(98),
		"settings": bson.M{
			"settings": int64(123),
			"version":  "onetwothree",
		},
	}}

	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		err = MigrateSettingsSchema(s.state)
		c.Assert(err, jc.ErrorIsNil)

		var docs []bson.M
		err = settingsColl.Find(
			bson.D{{"env-uuid", bson.D{{"$ne", s.state.EnvironUUID()}}}},
		).Sort("_id").Select(bson.M{"txn-queue": 0}).All(&docs)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(docs, jc.DeepEquals, expected)
	}
}

func (s *upgradesSuite) setupAddDefaultEndpointBindingsToServices(c *gc.C) []*Service {
	// Add an owner user.
	stateOwner, err := s.state.AddUser("bob", "notused", "notused", "bob")
	c.Assert(err, jc.ErrorIsNil)
	ownerTag := stateOwner.UserTag()
	_, err = s.state.AddEnvironmentUser(ownerTag, ownerTag, "")
	c.Assert(err, jc.ErrorIsNil)

	// Add a coule of test spaces, but notably NOT the default one.
	_, err = s.state.AddSpace("db", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.AddSpace("apps", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	// Add some testing charms for the services.
	charms := []*Charm{
		AddTestingCharm(c, s.state, "wordpress"),
		AddTestingCharm(c, s.state, "mysql"),
	}

	// Add a few services using the charms above: with no bindings, with just
	// defaults, and with explicitly given bindings. For the first case we need
	// to manually remove the added default bindings.
	wpBindings := map[string]string{
		"db":  "db",
		"url": "apps",
	}
	msBindings := map[string]string{
		"server": "db",
	}
	services := []*Service{
		AddTestingService(c, s.state, "wp-no-bindings", charms[0], ownerTag),
		AddTestingService(c, s.state, "ms-no-bindings", charms[1], ownerTag),

		AddTestingService(c, s.state, "wp-default-bindings", charms[0], ownerTag),
		AddTestingService(c, s.state, "ms-default-bindings", charms[1], ownerTag),

		AddTestingServiceWithBindings(c, s.state, "wp-given-bindings", charms[0], ownerTag, wpBindings),
		AddTestingServiceWithBindings(c, s.state, "ms-given-bindings", charms[1], ownerTag, msBindings),
	}

	// Drop the added endpoint bindings doc directly for the first two services.
	ops := []txn.Op{
		removeEndpointBindingsOp(services[0].globalKey()),
		removeEndpointBindingsOp(services[1].globalKey()),
	}
	err = s.state.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	return services
}

func (s *upgradesSuite) getServicesBindings(c *gc.C, services []*Service) map[string]map[string]string {
	currentBindings := make(map[string]map[string]string, len(services))
	for i := range services {
		serviceName := services[i].Name()
		readBindings, err := services[i].EndpointBindings()
		if err != nil && !errors.IsNotFound(err) {
			c.Fatalf("unexpected error getting service %q bindings: %v", serviceName, err)
		}
		serviceBindings := make(map[string]string, len(readBindings))
		for key, value := range readBindings {
			serviceBindings[key] = value
		}
		currentBindings[serviceName] = serviceBindings
	}
	return currentBindings
}

func (s *upgradesSuite) testAddDefaultEndpointBindingsToServices(c *gc.C, runTwice bool) {
	services := s.setupAddDefaultEndpointBindingsToServices(c)
	initialBindings := s.getServicesBindings(c, services)
	wpAllDefaults := map[string]string{
		"url":             network.DefaultSpace,
		"logging-dir":     network.DefaultSpace,
		"monitoring-port": network.DefaultSpace,
		"db":              network.DefaultSpace,
		"cache":           network.DefaultSpace,
	}
	msAllDefaults := map[string]string{
		"server": network.DefaultSpace,
	}
	c.Assert(initialBindings, jc.DeepEquals, map[string]map[string]string{
		"wp-no-bindings":      map[string]string{},
		"wp-default-bindings": wpAllDefaults,
		"wp-given-bindings": map[string]string{
			"url":             "apps",
			"logging-dir":     network.DefaultSpace,
			"monitoring-port": network.DefaultSpace,
			"db":              "db",
			"cache":           network.DefaultSpace,
		},

		"ms-no-bindings":      map[string]string{},
		"ms-default-bindings": msAllDefaults,
		"ms-given-bindings": map[string]string{
			"server": "db",
		},
	})

	assertFinalBindings := func() {
		finalBindings := s.getServicesBindings(c, services)
		c.Assert(finalBindings, jc.DeepEquals, map[string]map[string]string{
			"wp-no-bindings":      wpAllDefaults,
			"wp-default-bindings": wpAllDefaults,
			"wp-given-bindings": map[string]string{
				"url":             "apps",
				"logging-dir":     network.DefaultSpace,
				"monitoring-port": network.DefaultSpace,
				"db":              "db",
				"cache":           network.DefaultSpace,
			},

			"ms-no-bindings":      msAllDefaults,
			"ms-default-bindings": msAllDefaults,
			"ms-given-bindings": map[string]string{
				"server": "db",
			},
		})
	}
	err := AddDefaultEndpointBindingsToServices(s.state)
	c.Assert(err, jc.ErrorIsNil)
	assertFinalBindings()

	if runTwice {
		err = AddDefaultEndpointBindingsToServices(s.state)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("idempotency check failed!"))
		assertFinalBindings()
	}
}

func (s *upgradesSuite) TestAddDefaultEndpointBindingsToServices(c *gc.C) {
	s.testAddDefaultEndpointBindingsToServices(c, false)
}

func (s *upgradesSuite) TestAddDefaultEndpointBindingsToServicesIdempotent(c *gc.C) {
	s.testAddDefaultEndpointBindingsToServices(c, true)
}
