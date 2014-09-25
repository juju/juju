// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	charmtesting "gopkg.in/juju/charm.v4/testing"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/testing"
)

type upgradesSuite struct {
	testing.BaseSuite
	gitjujutesting.MgoSuite
	state *State
	owner names.UserTag
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
	s.owner = names.NewLocalUserTag("upgrade-admin")
	s.state, err = Initialize(s.owner, TestingMongoInfo(), testing.EnvironConfig(c), TestingDialOpts(), Policy(nil))
	c.Assert(err, gc.IsNil)
}

func (s *upgradesSuite) TearDownTest(c *gc.C) {
	if s.state != nil {
		s.state.Close()
	}
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
	user, err := s.state.User(names.NewLocalUserTag(userId))
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
	stateBob, err := s.state.AddUser("bob", "notused", "notused", "bob")
	c.Assert(err, gc.IsNil)
	bobTag := stateBob.UserTag()

	_, err = s.state.EnvironmentUser(bobTag)
	c.Assert(err, gc.ErrorMatches, `environment user "bob@local" not found`)

	err = AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, gc.IsNil)

	admin, err := s.state.EnvironmentUser(s.owner)
	c.Assert(err, gc.IsNil)
	c.Assert(admin.UserTag().Username(), gc.DeepEquals, s.owner.Username())
	bob, err := s.state.EnvironmentUser(bobTag)
	c.Assert(err, gc.IsNil)
	c.Assert(bob.UserTag().Username(), gc.DeepEquals, bobTag.Username())
}

func (s *upgradesSuite) TestAddStateUsersToEnvironIdempotent(c *gc.C) {
	stateBob, err := s.state.AddUser("bob", "notused", "notused", "bob")
	c.Assert(err, gc.IsNil)
	bobTag := stateBob.UserTag()

	err = AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, gc.IsNil)

	err = AddStateUsersAsEnvironUsers(s.state)
	c.Assert(err, gc.IsNil)

	admin, err := s.state.EnvironmentUser(s.owner)
	c.Assert(admin.UserTag().Username(), gc.DeepEquals, s.owner.Username())
	bob, err := s.state.EnvironmentUser(bobTag)
	c.Assert(err, gc.IsNil)
	c.Assert(bob.UserTag().Username(), gc.DeepEquals, bobTag.Username())
}

func (s *upgradesSuite) TestAddEnvironmentUUIDToStateServerDoc(c *gc.C) {
	info, err := s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
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
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)
	// Make sure it has gone.
	stateServers, closer := s.state.getCollection(stateServersC)
	defer closer()
	var doc stateServersDoc
	err = stateServers.Find(bson.D{{"_id", environGlobalKey}}).One(&doc)
	c.Assert(err, gc.IsNil)
	c.Assert(doc.EnvUUID, gc.Equals, "")

	// Run the upgrade step
	err = AddEnvironmentUUIDToStateServerDoc(s.state)
	c.Assert(err, gc.IsNil)
	// Make sure it is there now
	info, err = s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(info.EnvironmentTag, gc.Equals, tag)
}

func (s *upgradesSuite) TestAddEnvironmentUUIDToStateServerDocIdempotent(c *gc.C) {
	info, err := s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
	tag := info.EnvironmentTag

	err = AddEnvironmentUUIDToStateServerDoc(s.state)
	c.Assert(err, gc.IsNil)

	info, err = s.state.StateServerInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(info.EnvironmentTag, gc.Equals, tag)
}

func (s *upgradesSuite) TestAddCharmStoragePaths(c *gc.C) {
	ch := charmtesting.Charms.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)

	bundleSHA256 := "dummy-1-sha256"
	dummyCharm, err := s.state.AddCharm(ch, curl, "", bundleSHA256)
	c.Assert(err, gc.IsNil)
	SetCharmBundleURL(c, s.state, curl, "http://anywhere.com")
	dummyCharm, err = s.state.Charm(curl)
	c.Assert(err, gc.IsNil)
	c.Assert(dummyCharm.BundleURL(), gc.NotNil)
	c.Assert(dummyCharm.BundleURL().String(), gc.Equals, "http://anywhere.com")
	c.Assert(dummyCharm.StoragePath(), gc.Equals, "")

	storagePaths := map[*charm.URL]string{curl: "/some/where"}
	err = AddCharmStoragePaths(s.state, storagePaths)
	c.Assert(err, gc.IsNil)

	dummyCharm, err = s.state.Charm(curl)
	c.Assert(err, gc.IsNil)
	c.Assert(dummyCharm.BundleURL(), gc.IsNil)
	c.Assert(dummyCharm.StoragePath(), gc.Equals, "/some/where")
}

func (s *upgradesSuite) TestAddEnvUUIDToServicesID(c *gc.C) {
	serviceName := "wordpress"
	s.addServiceNoEnvID(c, serviceName)

	var service serviceDoc
	services, closer := s.state.getCollection(servicesC)
	defer closer()

	err := AddEnvUUIDToServicesID(s.state)
	c.Assert(err, gc.IsNil)

	err = services.Find(bson.D{{"_id", serviceName}}).One(&service)
	c.Assert(err, gc.ErrorMatches, "not found")

	err = services.Find(bson.D{{"_id", s.state.docID(serviceName)}}).One(&service)
	c.Assert(err, gc.IsNil)
	c.Assert(service.Name, gc.Equals, serviceName)
	c.Assert(service.EnvUUID, gc.Equals, s.state.EnvironTag().Id())
}

func (s *upgradesSuite) TestAddEnvUUIDToServicesIDIdempotent(c *gc.C) {
	serviceName := "wordpress"
	s.addServiceNoEnvID(c, serviceName)

	var serviceResults []serviceDoc
	services, closer := s.state.getCollection(servicesC)
	defer closer()

	err := AddEnvUUIDToServicesID(s.state)
	c.Assert(err, gc.IsNil)

	err = AddEnvUUIDToServicesID(s.state)
	c.Assert(err, gc.IsNil)

	err = services.Find(nil).All(&serviceResults)
	c.Assert(err, gc.IsNil)
	c.Assert(serviceResults, gc.HasLen, 1)

	serviceResults[0].DocID = s.state.docID(serviceName)
}

func (s *upgradesSuite) addServiceNoEnvID(c *gc.C, name string) {
	// Bare minimum service document as of 1.21-alpha1
	oldService := struct {
		Name string `bson:"_id"`
	}{Name: name}

	ops := []txn.Op{{
		C:      servicesC,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: oldService,
	}}
	err := s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)
}

func (s *upgradesSuite) TestAddCharmStoragePathsAllOrNothing(c *gc.C) {
	ch := charmtesting.Charms.CharmDir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)

	bundleSHA256 := "dummy-1-sha256"
	dummyCharm, err := s.state.AddCharm(ch, curl, "", bundleSHA256)
	c.Assert(err, gc.IsNil)

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
	c.Assert(err, gc.IsNil)
	c.Assert(dummyCharm.StoragePath(), gc.Equals, "")
}

func (s *upgradesSuite) TestSetOwnerAndServerUUIDForEnvironment(c *gc.C) {
	env, err := s.state.Environment()
	c.Assert(err, gc.IsNil)

	// force remove the server-uuid and owner
	ops := []txn.Op{{
		C:      environmentsC,
		Id:     env.UUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$unset", bson.D{
			{"server-uuid", nil}, {"owner", nil},
		}}},
	}}
	err = s.state.runTransaction(ops)
	c.Assert(err, gc.IsNil)
	// Make sure it has gone.
	environments, closer := s.state.getCollection(environmentsC)
	defer closer()

	var envDoc environmentDoc
	err = environments.FindId(env.UUID()).One(&envDoc)
	c.Assert(err, gc.IsNil)
	c.Assert(envDoc.ServerUUID, gc.Equals, "")
	c.Assert(envDoc.Owner, gc.Equals, "")

	// Run the upgrade step
	err = SetOwnerAndServerUUIDForEnvironment(s.state)
	c.Assert(err, gc.IsNil)
	// Make sure it is there now
	env, err = s.state.Environment()
	c.Assert(err, gc.IsNil)
	c.Assert(env.ServerTag().Id(), gc.Equals, env.UUID())
	c.Assert(env.Owner().Id(), gc.Equals, "admin@local")
}

func (s *upgradesSuite) TestSetOwnerAndServerUUIDForEnvironmentIdempotent(c *gc.C) {
	// Run the upgrade step
	err := SetOwnerAndServerUUIDForEnvironment(s.state)
	c.Assert(err, gc.IsNil)
	// Run the upgrade step gagain
	err = SetOwnerAndServerUUIDForEnvironment(s.state)
	c.Assert(err, gc.IsNil)
	// Check as expected
	env, err := s.state.Environment()
	c.Assert(err, gc.IsNil)
	c.Assert(env.ServerTag().Id(), gc.Equals, env.UUID())
	c.Assert(env.Owner().Id(), gc.Equals, "admin@local")
}
