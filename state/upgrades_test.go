// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type upgradesSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&upgradesSuite{})

func (s *upgradesSuite) TestStripLocalUserDomainCredentials(c *gc.C) {
	coll, closer := s.state.getRawCollection(cloudCredentialsC)
	defer closer()
	err := coll.Insert(
		cloudCredentialDoc{
			DocID:      "aws#admin@local#default",
			Owner:      "user-admin@local",
			Name:       "default",
			Cloud:      "cloud-aws",
			AuthType:   "userpass",
			Attributes: map[string]string{"user": "fred"},
		},
		cloudCredentialDoc{
			DocID:      "aws#fred#default",
			Owner:      "user-mary@external",
			Name:       "default",
			Cloud:      "cloud-aws",
			AuthType:   "userpass",
			Attributes: map[string]string{"user": "fred"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	expected := []bson.M{{
		"_id":        "aws#admin#default",
		"owner":      "user-admin",
		"cloud":      "cloud-aws",
		"name":       "default",
		"revoked":    false,
		"auth-type":  "userpass",
		"attributes": bson.M{"user": "fred"},
	}, {
		"_id":        "aws#fred#default",
		"owner":      "user-mary@external",
		"cloud":      "cloud-aws",
		"name":       "default",
		"revoked":    false,
		"auth-type":  "userpass",
		"attributes": bson.M{"user": "fred"},
	}}
	s.assertStrippedUserData(c, coll, expected)
}

func (s *upgradesSuite) TestStripLocalUserDomainModels(c *gc.C) {
	coll, closer := s.state.getRawCollection(modelsC)
	defer closer()

	var initialModels []bson.M
	err := coll.Find(nil).Sort("_id").All(&initialModels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(initialModels, gc.HasLen, 1)

	err = coll.Insert(
		modelDoc{
			UUID:            "0000-dead-beaf-0001",
			Owner:           "user-admin@local",
			Name:            "controller",
			ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			Cloud:           "cloud-aws",
			CloudRegion:     "us-west-1",
			CloudCredential: "aws#fred@local#default",
		},
		modelDoc{
			UUID:            "0000-dead-beaf-0002",
			Owner:           "user-mary@external",
			Name:            "default",
			ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			Cloud:           "cloud-aws",
			CloudRegion:     "us-west-1",
			CloudCredential: "aws#mary@external#default",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	initialModel := initialModels[0]
	delete(initialModel, "txn-queue")
	delete(initialModel, "txn-revno")
	initialModel["owner"] = "test-admin"

	expected := []bson.M{{
		"_id":              "0000-dead-beaf-0001",
		"owner":            "user-admin",
		"cloud":            "cloud-aws",
		"name":             "controller",
		"cloud-region":     "us-west-1",
		"cloud-credential": "aws#fred#default",
		"controller-uuid":  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		"life":             0,
		"migration-mode":   "",
	}, {
		"_id":              "0000-dead-beaf-0002",
		"owner":            "user-mary@external",
		"cloud":            "cloud-aws",
		"name":             "default",
		"cloud-region":     "us-west-1",
		"cloud-credential": "aws#mary@external#default",
		"controller-uuid":  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		"life":             0,
		"migration-mode":   "",
	},
		initialModel,
	}

	s.assertStrippedUserData(c, coll, expected)
}

func (s *upgradesSuite) TestStripLocalUserDomainModelNames(c *gc.C) {
	coll, closer := s.state.getRawCollection(usermodelnameC)
	defer closer()

	err := coll.Insert(
		bson.M{"_id": "fred@local:test"},
		bson.M{"_id": "mary@external:test2"},
	)
	c.Assert(err, jc.ErrorIsNil)

	expected := []bson.M{{
		"_id": "fred:test",
	}, {
		"_id": "mary@external:test2",
	}, {
		"_id": "test-admin:testenv",
	}}

	s.assertStrippedUserData(c, coll, expected)
}

func (s *upgradesSuite) TestStripLocalUserDomainControllerUser(c *gc.C) {
	s.assertStripLocalUserDomainUserAccess(c, controllerUsersC)
}

func (s *upgradesSuite) TestStripLocalUserDomainModelUser(c *gc.C) {
	s.assertStripLocalUserDomainUserAccess(c, modelUsersC)
}

func (s *upgradesSuite) assertStripLocalUserDomainUserAccess(c *gc.C, collName string) {
	coll, closer := s.state.getRawCollection(collName)
	defer closer()

	var initialUsers []bson.M
	err := coll.Find(nil).Sort("_id").All(&initialUsers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(initialUsers, gc.HasLen, 1)

	now := time.Now()
	err = coll.Insert(
		userAccessDoc{
			ID:          "zfred@local",
			ObjectUUID:  "uuid1",
			UserName:    "fred@local",
			DisplayName: "Fred",
			CreatedBy:   "admin@local",
			DateCreated: now,
		},
		userAccessDoc{
			ID:          "zmary@external",
			ObjectUUID:  "uuid2",
			UserName:    "mary@external",
			DisplayName: "Mary",
			CreatedBy:   "admin@local",
			DateCreated: now,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	initialUser := initialUsers[0]
	delete(initialUser, "txn-queue")
	delete(initialUser, "txn-revno")
	initialCreated := initialUser["datecreated"].(time.Time)
	initialUser["datecreated"] = initialCreated.Truncate(time.Millisecond)

	roundedNow := now.Truncate(time.Millisecond)
	expected := []bson.M{
		initialUser,
		{
			"_id":         "zfred",
			"object-uuid": "uuid1",
			"user":        "fred",
			"displayname": "Fred",
			"createdby":   "admin",
			"datecreated": roundedNow,
		}, {
			"_id":         "zmary@external",
			"object-uuid": "uuid2",
			"user":        "mary@external",
			"displayname": "Mary",
			"createdby":   "admin",
			"datecreated": roundedNow,
		},
	}
	s.assertStrippedUserData(c, coll, expected)
}

func (s *upgradesSuite) TestStripLocalUserDomainPermissions(c *gc.C) {
	coll, closer := s.state.getRawCollection(permissionsC)
	defer closer()

	var initialPermissions []bson.M
	err := coll.Find(nil).Sort("_id").All(&initialPermissions)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(initialPermissions, gc.HasLen, 2)

	err = coll.Insert(
		permissionDoc{
			ID:               "uuid#fred@local",
			ObjectGlobalKey:  "c#uuid",
			SubjectGlobalKey: "fred@local",
			Access:           "addmodel",
		},
		permissionDoc{
			ID:               "uuid#mary@external",
			ObjectGlobalKey:  "c#uuid",
			SubjectGlobalKey: "mary@external",
			Access:           "addmodel",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	for i, inital := range initialPermissions {
		perm := inital
		delete(perm, "txn-queue")
		delete(perm, "txn-revno")
		initialPermissions[i] = perm
	}

	expected := []bson.M{initialPermissions[0], initialPermissions[1], {
		"_id":                "uuid#fred",
		"object-global-key":  "c#uuid",
		"subject-global-key": "fred",
		"access":             "addmodel",
	}, {
		"_id":                "uuid#mary@external",
		"object-global-key":  "c#uuid",
		"subject-global-key": "mary@external",
		"access":             "addmodel",
	}}
	s.assertStrippedUserData(c, coll, expected)
}

func (s *upgradesSuite) TestStripLocalUserDomainLastConnection(c *gc.C) {
	coll, closer := s.state.getRawCollection(modelUserLastConnectionC)
	defer closer()

	now := time.Now()
	err := coll.Insert(
		modelUserLastConnectionDoc{
			ID:             "fred@local",
			ModelUUID:      "uuid",
			UserName:       "fred@local",
			LastConnection: now,
		},
		modelUserLastConnectionDoc{
			ID:             "mary@external",
			ModelUUID:      "uuid",
			UserName:       "mary@external",
			LastConnection: now,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	roundedNow := now.Truncate(time.Millisecond)
	expected := []bson.M{{
		"_id":             "fred",
		"model-uuid":      "uuid",
		"user":            "fred",
		"last-connection": roundedNow,
	}, {
		"_id":             "mary@external",
		"model-uuid":      "uuid",
		"user":            "mary@external",
		"last-connection": roundedNow,
	}}
	s.assertStrippedUserData(c, coll, expected)
}

func (s *upgradesSuite) assertStrippedUserData(c *gc.C, coll *mgo.Collection, expected []bson.M) {
	s.assertUpgradedData(c, StripLocalUserDomain, coll, expected)
}

func (s *upgradesSuite) assertUpgradedData(c *gc.C, upgrade func(*State) error, coll *mgo.Collection, expected []bson.M) {
	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		err := upgrade(s.state)
		c.Assert(err, jc.ErrorIsNil)

		var docs []bson.M
		err = coll.Find(nil).Sort("_id").All(&docs)
		c.Assert(err, jc.ErrorIsNil)
		for i, d := range docs {
			doc := d
			delete(doc, "txn-queue")
			delete(doc, "txn-revno")
			docs[i] = doc
		}
		c.Assert(docs, jc.DeepEquals, expected)
	}
}

func (s *upgradesSuite) TestRenameAddModelPermission(c *gc.C) {
	coll, closer := s.state.getRawCollection(permissionsC)
	defer closer()

	var initialPermissions []bson.M
	err := coll.Find(nil).Sort("_id").All(&initialPermissions)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(initialPermissions, gc.HasLen, 2)

	err = coll.Insert(
		permissionDoc{
			ID:               "uuid#fred",
			ObjectGlobalKey:  "c#uuid",
			SubjectGlobalKey: "fred",
			Access:           "superuser",
		},
		permissionDoc{
			ID:               "uuid#mary@external",
			ObjectGlobalKey:  "c#uuid",
			SubjectGlobalKey: "mary@external",
			Access:           "addmodel",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	for i, inital := range initialPermissions {
		perm := inital
		delete(perm, "txn-queue")
		delete(perm, "txn-revno")
		initialPermissions[i] = perm
	}

	expected := []bson.M{initialPermissions[0], initialPermissions[1], {
		"_id":                "uuid#fred",
		"object-global-key":  "c#uuid",
		"subject-global-key": "fred",
		"access":             "superuser",
	}, {
		"_id":                "uuid#mary@external",
		"object-global-key":  "c#uuid",
		"subject-global-key": "mary@external",
		"access":             "add-model",
	}}
	s.assertUpgradedData(c, RenameAddModelPermission, coll, expected)
}

func (s *upgradesSuite) TestDropOldLogIndex(c *gc.C) {
	coll := s.state.MongoSession().DB(logsDB).C(logsC)
	err := coll.EnsureIndexKey("e", "t")
	c.Assert(err, jc.ErrorIsNil)
	err = DropOldLogIndex(s.state)
	c.Assert(err, jc.ErrorIsNil)

	exists, err := hasIndex(coll, []string{"e", "t"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsFalse)

	// Sanity check for idempotency.
	err = DropOldLogIndex(s.state)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestDropOldIndexWhenNoIndex(c *gc.C) {
	coll := s.state.MongoSession().DB(logsDB).C(logsC)
	exists, err := hasIndex(coll, []string{"e", "t"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsFalse)

	err = DropOldLogIndex(s.state)
	c.Assert(err, jc.ErrorIsNil)
}

func hasIndex(coll *mgo.Collection, key []string) (bool, error) {
	indexes, err := coll.Indexes()
	if err != nil {
		return false, err
	}
	for _, index := range indexes {
		if reflect.DeepEqual(index.Key, key) {
			return true, nil
		}
	}
	return false, nil
}
