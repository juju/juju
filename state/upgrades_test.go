// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
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
		"sla":              bson.M{"level": "", "credentials": []uint8{}},
		"meter-status":     bson.M{"code": "", "info": ""},
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
		"sla":              bson.M{"level": "", "credentials": []uint8{}},
		"meter-status":     bson.M{"code": "", "info": ""},
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
	s.assertUpgradedData(c, StripLocalUserDomain, expectUpgradedData{coll, expected})
}

type expectUpgradedData struct {
	coll     *mgo.Collection
	expected []bson.M
}

func (s *upgradesSuite) assertUpgradedData(c *gc.C, upgrade func(*State) error, expect ...expectUpgradedData) {
	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		err := upgrade(s.state)
		c.Assert(err, jc.ErrorIsNil)

		for _, expect := range expect {
			var docs []bson.M
			err = expect.coll.Find(nil).Sort("_id").All(&docs)
			c.Assert(err, jc.ErrorIsNil)
			for i, d := range docs {
				doc := d
				delete(doc, "txn-queue")
				delete(doc, "txn-revno")
				delete(doc, "version")
				docs[i] = doc
			}
			c.Assert(docs, jc.DeepEquals, expect.expected)
		}
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
	s.assertUpgradedData(c, RenameAddModelPermission, expectUpgradedData{coll, expected})
}

func (s *upgradesSuite) TestAddMigrationAttempt(c *gc.C) {
	coll, closer := s.state.getRawCollection(migrationsC)
	defer closer()

	err := coll.Insert(
		bson.M{"_id": "uuid:1"},
		bson.M{"_id": "uuid:11"},
		bson.M{
			"_id":     "uuid:2",
			"attempt": 2,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	expected := []bson.M{
		bson.M{
			"_id":     "uuid:1",
			"attempt": 1,
		},
		bson.M{
			"_id":     "uuid:11",
			"attempt": 11,
		},
		bson.M{
			"_id":     "uuid:2",
			"attempt": 2,
		},
	}
	s.assertUpgradedData(c, AddMigrationAttempt, expectUpgradedData{coll, expected})
}

func (s *upgradesSuite) TestAddLocalCharmSequences(c *gc.C) {
	uuid0 := s.state.ModelUUID()
	st1 := s.newState(c)
	uuid1 := st1.ModelUUID()
	// Sort model UUIDs so that result ordering matches expected test
	// results.
	if uuid0 > uuid1 {
		uuid0, uuid1 = uuid1, uuid0
	}

	mkInput := func(uuid, curl string, life Life) bson.M {
		return bson.M{
			"_id":  uuid + ":" + curl,
			"url":  curl,
			"life": life,
		}
	}

	charms, closer := s.state.getRawCollection(charmsC)
	defer closer()
	err := charms.Insert(
		mkInput(uuid0, "local:trusty/bar-2", Alive),
		mkInput(uuid0, "local:trusty/bar-1", Dead),
		mkInput(uuid0, "local:xenial/foo-1", Alive),
		mkInput(uuid0, "cs:xenial/moo-2", Alive), // Should be ignored
		mkInput(uuid1, "local:trusty/aaa-3", Alive),
		mkInput(uuid1, "local:xenial/bbb-5", Dead), //Should be handled and removed.
		mkInput(uuid1, "cs:xenial/boo-2", Alive),   // Should be ignored
	)
	c.Assert(err, jc.ErrorIsNil)

	sequences, closer := s.state.getRawCollection(sequenceC)
	defer closer()

	mkExpected := func(uuid, urlBase string, counter int) bson.M {
		name := "charmrev-" + urlBase
		return bson.M{
			"_id":        uuid + ":" + name,
			"name":       name,
			"model-uuid": uuid,
			"counter":    counter,
		}
	}
	expected := []bson.M{
		mkExpected(uuid0, "local:trusty/bar", 3),
		mkExpected(uuid0, "local:xenial/foo", 2),
		mkExpected(uuid1, "local:trusty/aaa", 4),
		mkExpected(uuid1, "local:xenial/bbb", 6),
	}
	s.assertUpgradedData(
		c, AddLocalCharmSequences,
		expectUpgradedData{sequences, expected},
	)

	// Expect Dead charm documents to be removed.
	var docs []bson.M
	c.Assert(charms.Find(nil).All(&docs), jc.ErrorIsNil)
	var ids []string
	for _, doc := range docs {
		ids = append(ids, doc["_id"].(string))
	}
	c.Check(ids, jc.SameContents, []string{
		uuid0 + ":local:trusty/bar-2",
		// uuid0:local:trusty/bar-1 is gone
		uuid0 + ":local:xenial/foo-1",
		uuid0 + ":cs:xenial/moo-2",
		uuid1 + ":local:trusty/aaa-3",
		// uuid1:local:xenial/bbb-5 is gone
		uuid1 + ":cs:xenial/boo-2",
	})
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

func (s *upgradesSuite) TestUpdateLegacyLXDCloud(c *gc.C) {
	cloudColl, cloudCloser := s.state.getRawCollection(cloudsC)
	defer cloudCloser()
	cloudCredColl, cloudCredCloser := s.state.getRawCollection(cloudCredentialsC)
	defer cloudCredCloser()

	_, err := cloudColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = cloudCredColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = cloudColl.Insert(bson.M{
		"_id":        "localhost",
		"name":       "localhost",
		"type":       "lxd",
		"auth-types": []string{"empty"},
		"endpoint":   "",
		"regions": bson.M{
			"localhost": bson.M{
				"endpoint": "",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = cloudCredColl.Insert(bson.M{
		"_id":       "localhost#admin#streetcred",
		"owner":     "admin",
		"cloud":     "localhost",
		"name":      "streetcred",
		"revoked":   false,
		"auth-type": "empty",
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedClouds := []bson.M{{
		"_id":        "localhost",
		"name":       "localhost",
		"type":       "lxd",
		"auth-types": []interface{}{"certificate"},
		"endpoint":   "foo",
		"regions": bson.M{
			"localhost": bson.M{
				"endpoint": "foo",
			},
		},
	}}

	expectedCloudCreds := []bson.M{{
		"_id":       "localhost#admin#streetcred",
		"owner":     "admin",
		"cloud":     "localhost",
		"name":      "streetcred",
		"revoked":   false,
		"auth-type": "certificate",
		"attributes": bson.M{
			"foo": "bar",
			"baz": "qux",
		},
	}}

	newCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"foo": "bar",
		"baz": "qux",
	})
	f := func(st *State) error {
		return UpdateLegacyLXDCloudCredentials(st, "foo", newCred)
	}
	s.assertUpgradedData(c, f,
		expectUpgradedData{cloudColl, expectedClouds},
		expectUpgradedData{cloudCredColl, expectedCloudCreds},
	)
}

func (s *upgradesSuite) TestUpdateLegacyLXDCloudUnchanged(c *gc.C) {
	cloudColl, cloudCloser := s.state.getRawCollection(cloudsC)
	defer cloudCloser()
	cloudCredColl, cloudCredCloser := s.state.getRawCollection(cloudCredentialsC)
	defer cloudCredCloser()

	_, err := cloudColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = cloudCredColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = cloudColl.Insert(bson.M{
		// Non-LXD clouds should be altogether unchanged.
		"_id":        "foo",
		"name":       "foo",
		"type":       "dummy",
		"auth-types": []string{"empty"},
		"endpoint":   "unchanged",
	}, bson.M{
		// A LXD cloud with endpoints already set should
		// only have its auth-types updated.
		"_id":        "localhost",
		"name":       "localhost",
		"type":       "lxd",
		"auth-types": []string{"empty"},
		"endpoint":   "unchanged",
		"regions": bson.M{
			"localhost": bson.M{
				"endpoint": "unchanged",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = cloudCredColl.Insert(bson.M{
		// Credentials for non-LXD clouds should be unchanged.
		"_id":       "foo#admin#default",
		"owner":     "admin",
		"cloud":     "foo",
		"name":      "default",
		"revoked":   false,
		"auth-type": "empty",
	}, bson.M{
		// LXD credentials with an auth-type other than
		// "empty" should be unchanged.
		"_id":       "localhost#admin#streetcred",
		"owner":     "admin",
		"cloud":     "localhost",
		"name":      "streetcred",
		"revoked":   false,
		"auth-type": "unchanged",
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedClouds := []bson.M{{
		"_id":        "foo",
		"name":       "foo",
		"type":       "dummy",
		"auth-types": []interface{}{"empty"},
		"endpoint":   "unchanged",
	}, {
		"_id":        "localhost",
		"name":       "localhost",
		"type":       "lxd",
		"auth-types": []interface{}{"certificate"},
		"endpoint":   "unchanged",
		"regions": bson.M{
			"localhost": bson.M{
				"endpoint": "unchanged",
			},
		},
	}}

	expectedCloudCreds := []bson.M{{
		"_id":       "foo#admin#default",
		"owner":     "admin",
		"cloud":     "foo",
		"name":      "default",
		"revoked":   false,
		"auth-type": "empty",
	}, {
		"_id":       "localhost#admin#streetcred",
		"owner":     "admin",
		"cloud":     "localhost",
		"name":      "streetcred",
		"revoked":   false,
		"auth-type": "unchanged",
	}}

	newCred := cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"foo": "bar",
		"baz": "qux",
	})
	f := func(st *State) error {
		return UpdateLegacyLXDCloudCredentials(st, "foo", newCred)
	}
	s.assertUpgradedData(c, f,
		expectUpgradedData{cloudColl, expectedClouds},
		expectUpgradedData{cloudCredColl, expectedCloudCreds},
	)
}

func (s *upgradesSuite) TestUpgradeNoProxy(c *gc.C) {
	settingsColl, settingsCloser := s.state.getRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		"_id": "foo",
		"settings": bson.M{
			"no-proxy": "127.0.0.1,localhost,::1"},
	}, bson.M{
		"_id": "bar",
		"settings": bson.M{
			"no-proxy": "localhost"},
	}, bson.M{
		"_id": "baz",
		"settings": bson.M{
			"no-proxy":        "192.168.1.1,10.0.0.2",
			"another-setting": "anothervalue"},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedSettings := []bson.M{
		{
			"_id": "bar",
			"settings": bson.M{
				"no-proxy": "127.0.0.1,::1,localhost"},
		}, {
			"_id": "baz",
			"settings": bson.M{
				"no-proxy":        "10.0.0.2,127.0.0.1,192.168.1.1,::1,localhost",
				"another-setting": "anothervalue"},
		}, {
			"_id": "foo",
			"settings": bson.M{
				"no-proxy": "127.0.0.1,::1,localhost"},
		}}

	s.assertUpgradedData(c, UpgradeNoProxyDefaults,
		expectUpgradedData{settingsColl, expectedSettings},
	)
}

func (s *upgradesSuite) TestAddNonDetachableStorageMachineId(c *gc.C) {
	volumesColl, volumesCloser := s.state.getRawCollection(volumesC)
	defer volumesCloser()
	volumeAttachmentsColl, volumeAttachmentsCloser := s.state.getRawCollection(volumeAttachmentsC)
	defer volumeAttachmentsCloser()

	filesystemsColl, filesystemsCloser := s.state.getRawCollection(filesystemsC)
	defer filesystemsCloser()
	filesystemAttachmentsColl, filesystemAttachmentsCloser := s.state.getRawCollection(filesystemAttachmentsC)
	defer filesystemAttachmentsCloser()

	uuid := s.state.ModelUUID()

	err := volumesColl.Insert(bson.M{
		"_id":        uuid + ":0",
		"name":       "0",
		"model-uuid": uuid,
		"machineid":  "42",
	}, bson.M{
		"_id":        uuid + ":1",
		"name":       "1",
		"model-uuid": uuid,
		"info": bson.M{
			"pool": "modelscoped",
		},
	}, bson.M{
		"_id":        uuid + ":2",
		"name":       "2",
		"model-uuid": uuid,
		"params": bson.M{
			"pool": "static",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = volumeAttachmentsColl.Insert(bson.M{
		"_id":        uuid + ":123:2",
		"model-uuid": uuid,
		"machineid":  "123",
		"volumeid":   "2",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = filesystemsColl.Insert(bson.M{
		"_id":          uuid + ":0",
		"filesystemid": "0",
		"model-uuid":   uuid,
		"machineid":    "42",
	}, bson.M{
		"_id":          uuid + ":1",
		"filesystemid": "1",
		"model-uuid":   uuid,
		"info": bson.M{
			"pool": "modelscoped",
		},
	}, bson.M{
		"_id":          uuid + ":2",
		"filesystemid": "2",
		"model-uuid":   uuid,
		"params": bson.M{
			"pool": "static",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = filesystemAttachmentsColl.Insert(bson.M{
		"_id":          uuid + ":123:2",
		"model-uuid":   uuid,
		"machineid":    "123",
		"filesystemid": "2",
	})
	c.Assert(err, jc.ErrorIsNil)

	// We expect that:
	//  - volume-0 and filesystem-0 are unchanged, since they
	//    already have machineid fields
	//  - volume-1 and filesystem-1 are unchanged, since they
	//    are detachable
	//  - volume-2's and filesystem-2's machineid fields are
	//    set to 123, the machine to which they are inherently
	//    bound
	expectedVolumes := []bson.M{{
		"_id":        uuid + ":0",
		"name":       "0",
		"model-uuid": uuid,
		"machineid":  "42",
	}, {
		"_id":        uuid + ":1",
		"name":       "1",
		"model-uuid": uuid,
		"info": bson.M{
			"pool": "modelscoped",
		},
	}, {
		"_id":        uuid + ":2",
		"name":       "2",
		"model-uuid": uuid,
		"params": bson.M{
			"pool": "static",
		},
		"machineid": "123",
	}}
	expectedFilesystems := []bson.M{{
		"_id":          uuid + ":0",
		"filesystemid": "0",
		"model-uuid":   uuid,
		"machineid":    "42",
	}, {
		"_id":          uuid + ":1",
		"filesystemid": "1",
		"model-uuid":   uuid,
		"info": bson.M{
			"pool": "modelscoped",
		},
	}, {
		"_id":          uuid + ":2",
		"filesystemid": "2",
		"model-uuid":   uuid,
		"params": bson.M{
			"pool": "static",
		},
		"machineid": "123",
	}}

	s.assertUpgradedData(c, AddNonDetachableStorageMachineId,
		expectUpgradedData{volumesColl, expectedVolumes},
		expectUpgradedData{filesystemsColl, expectedFilesystems},
	)
}

func (s *upgradesSuite) TestRemoveNilValueApplicationSettings(c *gc.C) {
	settingsColl, settingsCloser := s.state.getRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		"_id": "modelXXX:a#dontchangeapp",
		// this document should not be affected
		"settings": bson.M{
			"keepme": "have value"},
	}, bson.M{
		"_id": "modelXXX:a#removeall",
		// this settings will become empty
		"settings": bson.M{
			"keepme":   nil,
			"removeme": nil,
		},
	}, bson.M{
		"_id": "modelXXX:a#removeone",
		// one setting needs to be removed
		"settings": bson.M{
			"keepme":   "have value",
			"removeme": nil,
		},
	}, bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-application setting: should not be touched
		"settings": bson.M{
			"keepme":   "have value",
			"removeme": nil,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedSettings := []bson.M{
		{
			"_id":      "modelXXX:a#dontchangeapp",
			"settings": bson.M{"keepme": "have value"},
		}, {
			"_id":      "modelXXX:a#removeall",
			"settings": bson.M{},
		}, {
			"_id":      "modelXXX:a#removeone",
			"settings": bson.M{"keepme": "have value"},
		}, {
			"_id": "someothersettingshouldnotbetouched",
			"settings": bson.M{
				"keepme":   "have value",
				"removeme": nil,
			},
		}}

	s.assertUpgradedData(c, RemoveNilValueApplicationSettings,
		expectUpgradedData{settingsColl, expectedSettings},
	)
}

func (s *upgradesSuite) TestAddControllerLogCollectionsSizeSettingsKeepExisting(c *gc.C) {
	settingsColl, settingsCloser := s.state.getRawCollection(controllersC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		"_id": "controllerSettings",
		"settings": bson.M{
			"key":              "value",
			"max-logs-age":     "96h",
			"max-logs-size":    "5G",
			"max-txn-log-size": "8G",
		},
	}, bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-controller data: should not be touched
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedSettings := []bson.M{
		{
			"_id": "controllerSettings",
			"settings": bson.M{
				"key":              "value",
				"max-logs-age":     "96h",
				"max-logs-size":    "5G",
				"max-txn-log-size": "8G",
			},
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}

	s.assertUpgradedData(c, AddControllerLogCollectionsSizeSettings,
		expectUpgradedData{settingsColl, expectedSettings},
	)
}

func (s *upgradesSuite) TestAddControllerLogCollectionsSizeSettings(c *gc.C) {
	settingsColl, settingsCloser := s.state.getRawCollection(controllersC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		"_id":      "controllerSettings",
		"settings": bson.M{"key": "value"},
	}, bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-controller data: should not be touched
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedSettings := []bson.M{
		{
			"_id": "controllerSettings",
			"settings": bson.M{
				"key":              "value",
				"max-logs-age":     "72h",
				"max-logs-size":    "4096M",
				"max-txn-log-size": "10M",
			},
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}

	s.assertUpgradedData(c, AddControllerLogCollectionsSizeSettings,
		expectUpgradedData{settingsColl, expectedSettings},
	)
}

func (s *upgradesSuite) makeModel(c *gc.C, name string, attr testing.Attrs) *State {
	uuid := utils.MustNewUUID()
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": name,
		"uuid": uuid.String(),
	}.Merge(attr))
	m, err := s.state.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, st, err := s.state.NewModel(ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       m.Owner(),
		StorageProviderRegistry: provider.CommonStorageProviders(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}

func (s *upgradesSuite) TestAddStatusHistoryPruneSettings(c *gc.C) {
	settingsColl, settingsCloser := s.state.getRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	m1 := s.makeModel(c, "m1", testing.Attrs{
		"max-status-history-age":  "96h",
		"max-status-history-size": "4G",
	})
	defer m1.Close()

	m2 := s.makeModel(c, "m2", testing.Attrs{})
	defer m2.Close()

	err = settingsColl.Insert(bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-model setting: should not be touched
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg1, err := m1.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected1 := cfg1.AllAttrs()
	expected1["resource-tags"] = ""

	cfg2, err := m2.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected2 := cfg2.AllAttrs()
	expected2["max-status-history-age"] = "336h"
	expected2["max-status-history-size"] = "5G"
	expected2["resource-tags"] = ""

	expectedSettings := bsonMById{
		{
			"_id":        m1.ModelUUID() + ":e",
			"settings":   bson.M(expected1),
			"model-uuid": m1.ModelUUID(),
		}, {
			"_id":        m2.ModelUUID() + ":e",
			"settings":   bson.M(expected2),
			"model-uuid": m2.ModelUUID(),
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}
	sort.Sort(expectedSettings)

	s.assertUpgradedData(c, AddStatusHistoryPruneSettings,
		expectUpgradedData{settingsColl, expectedSettings},
	)
}

func (s *upgradesSuite) TestAddUpdateStatusHookSettings(c *gc.C) {
	settingsColl, settingsCloser := s.state.getRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	m1 := s.makeModel(c, "m1", testing.Attrs{
		"update-status-hook-interval": "20m",
	})
	defer m1.Close()

	m2 := s.makeModel(c, "m2", testing.Attrs{})
	defer m2.Close()

	err = settingsColl.Insert(bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-model setting: should not be touched
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	cfg1, err := m1.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected1 := cfg1.AllAttrs()
	expected1["resource-tags"] = ""

	cfg2, err := m2.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected2 := cfg2.AllAttrs()
	expected2["update-status-hook-interval"] = "5m"
	expected2["resource-tags"] = ""

	expectedSettings := bsonMById{
		{
			"_id":        m1.ModelUUID() + ":e",
			"settings":   bson.M(expected1),
			"model-uuid": m1.ModelUUID(),
		}, {
			"_id":        m2.ModelUUID() + ":e",
			"settings":   bson.M(expected2),
			"model-uuid": m2.ModelUUID(),
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}
	sort.Sort(expectedSettings)

	s.assertUpgradedData(c, AddUpdateStatusHookSettings,
		expectUpgradedData{settingsColl, expectedSettings},
	)
}

func (s *upgradesSuite) TestAddStorageInstanceConstraints(c *gc.C) {
	storageInstancesColl, storageInstancesCloser := s.state.getRawCollection(storageInstancesC)
	defer storageInstancesCloser()
	storageConstraintsColl, storageConstraintsCloser := s.state.getRawCollection(storageConstraintsC)
	defer storageConstraintsCloser()
	volumesColl, volumesCloser := s.state.getRawCollection(volumesC)
	defer volumesCloser()
	filesystemsColl, filesystemsCloser := s.state.getRawCollection(filesystemsC)
	defer filesystemsCloser()
	unitsColl, unitsCloser := s.state.getRawCollection(unitsC)
	defer unitsCloser()

	uuid := s.state.ModelUUID()

	err := storageInstancesColl.Insert(bson.M{
		"_id":         uuid + ":pgdata/0",
		"id":          "pgdata/0",
		"model-uuid":  uuid,
		"storagekind": StorageKindUnknown,
		"constraints": bson.M{
			"pool": "goodidea",
			"size": 99,
		},
	}, bson.M{
		// corresponds to volume-0
		"_id":         uuid + ":pgdata/1",
		"id":          "pgdata/1",
		"model-uuid":  uuid,
		"storagekind": StorageKindBlock,
		"storagename": "pgdata",
	}, bson.M{
		// corresponds to volume-1
		"_id":         uuid + ":pgdata/2",
		"id":          "pgdata/2",
		"model-uuid":  uuid,
		"storagekind": StorageKindBlock,
		"storagename": "pgdata",
	}, bson.M{
		// corresponds to filesystem-0
		"_id":         uuid + ":pgdata/3",
		"id":          "pgdata/3",
		"model-uuid":  uuid,
		"storagekind": StorageKindFilesystem,
		"storagename": "pgdata",
	}, bson.M{
		// corresponds to filesystem-1
		"_id":         uuid + ":pgdata/4",
		"id":          "pgdata/4",
		"model-uuid":  uuid,
		"storagekind": StorageKindFilesystem,
		"storagename": "pgdata",
	}, bson.M{
		// no volume or filesystem, owned by postgresql/0
		"_id":         uuid + ":pgdata/5",
		"id":          "pgdata/5",
		"model-uuid":  uuid,
		"storagekind": StorageKindBlock,
		"storagename": "pgdata",
		"owner":       "unit-postgresql-0",
	}, bson.M{
		// no volume, filesystem, or owner
		"_id":         uuid + ":pgdata/6",
		"id":          "pgdata/6",
		"model-uuid":  uuid,
		"storagekind": StorageKindBlock,
		"storagename": "pgdata",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = volumesColl.Insert(bson.M{
		"_id":        uuid + ":0",
		"name":       "0",
		"model-uuid": uuid,
		"storageid":  "pgdata/1",
		"info": bson.M{
			"pool": "modelscoped",
			"size": 1024,
		},
	}, bson.M{
		"_id":        uuid + ":1",
		"name":       "1",
		"model-uuid": uuid,
		"storageid":  "pgdata/2",
		"params": bson.M{
			"pool": "static",
			"size": 2048,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = filesystemsColl.Insert(bson.M{
		"_id":          uuid + ":0",
		"filesystemid": "0",
		"model-uuid":   uuid,
		"storageid":    "pgdata/3",
		"info": bson.M{
			"pool": "modelscoped",
			"size": 4096,
		},
	}, bson.M{
		"_id":          uuid + ":1",
		"filesystemid": "1",
		"model-uuid":   uuid,
		"storageid":    "pgdata/4",
		"params": bson.M{
			"pool": "static",
			"size": 8192,
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = unitsColl.Insert(bson.M{
		"_id":         uuid + ":postgresql/0",
		"name":        "postgresql/0",
		"model-uuid":  uuid,
		"application": "postgresql",
		"life":        Alive,
		"series":      "xenial",
		"charmurl":    "local:xenial/postgresql-1",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = storageConstraintsColl.Insert(bson.M{
		"_id":        uuid + ":asc#postgresql#local:xenial/postgresql-1",
		"model-uuid": uuid,
		"constraints": bson.M{
			"pgdata": bson.M{
				"pool":  "pgdata-pool",
				"size":  1234,
				"count": 99,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// We expect that:
	//  - pgdata/0 is unchanged, since it already has a constraints field.
	//  - pgdata/1 gets constraints from volume-0's info
	//  - pgdata/2 gets constraints from volume-1's params
	//  - pgdata/3 gets constraints from filesystem-0's info
	//  - pgdata/4 gets constraints from filesystem-1's params
	//  - pgdata/5 gets constraints from the postgresql application's
	//    storage constraints.
	//  - pgdata/6 gets default constraints.

	expectedStorageInstances := []bson.M{{
		"_id":         uuid + ":pgdata/0",
		"id":          "pgdata/0",
		"model-uuid":  uuid,
		"storagekind": int(StorageKindUnknown),
		"constraints": bson.M{
			"pool": "goodidea",
			"size": 99,
		},
	}, {
		"_id":         uuid + ":pgdata/1",
		"id":          "pgdata/1",
		"model-uuid":  uuid,
		"storagekind": int(StorageKindBlock),
		"storagename": "pgdata",
		"constraints": bson.M{
			"pool": "modelscoped",
			"size": int64(1024),
		},
	}, {
		"_id":         uuid + ":pgdata/2",
		"id":          "pgdata/2",
		"model-uuid":  uuid,
		"storagekind": int(StorageKindBlock),
		"storagename": "pgdata",
		"constraints": bson.M{
			"pool": "static",
			"size": int64(2048),
		},
	}, {
		"_id":         uuid + ":pgdata/3",
		"id":          "pgdata/3",
		"model-uuid":  uuid,
		"storagekind": int(StorageKindFilesystem),
		"storagename": "pgdata",
		"constraints": bson.M{
			"pool": "modelscoped",
			"size": int64(4096),
		},
	}, {
		"_id":         uuid + ":pgdata/4",
		"id":          "pgdata/4",
		"model-uuid":  uuid,
		"storagekind": int(StorageKindFilesystem),
		"storagename": "pgdata",
		"constraints": bson.M{
			"pool": "static",
			"size": int64(8192),
		},
	}, {
		"_id":         uuid + ":pgdata/5",
		"id":          "pgdata/5",
		"model-uuid":  uuid,
		"storagekind": int(StorageKindBlock),
		"storagename": "pgdata",
		"owner":       "unit-postgresql-0",
		"constraints": bson.M{
			"pool": "pgdata-pool",
			"size": int64(1234),
		},
	}, {
		"_id":         uuid + ":pgdata/6",
		"id":          "pgdata/6",
		"model-uuid":  uuid,
		"storagekind": int(StorageKindBlock),
		"storagename": "pgdata",
		"constraints": bson.M{
			"pool": "loop",
			"size": int64(1024),
		},
	}}

	s.assertUpgradedData(c, AddStorageInstanceConstraints,
		expectUpgradedData{storageInstancesColl, expectedStorageInstances},
	)
}

type bsonMById []bson.M

func (x bsonMById) Len() int { return len(x) }

func (x bsonMById) Swap(i, j int) { x[i], x[j] = x[j], x[i] }

func (x bsonMById) Less(i, j int) bool {
	return x[i]["_id"].(string) < x[j]["_id"].(string)
}

func (s *upgradesSuite) TestSplitLogCollection(c *gc.C) {
	db := s.state.MongoSession().DB(logsDB)
	oldLogs := db.C("logs")

	uuids := []string{"fake-1", "fake-2", "fake-3"}

	expected := map[string][]bson.M{}

	for i := 0; i < 15; i++ {
		modelUUID := uuids[i%3]
		logRow := bson.M{
			"_id": fmt.Sprintf("fake-objectid-%02d", i),
			"t":   100 * i,
			"e":   modelUUID,
			"r":   "2.1.2",
			"n":   fmt.Sprintf("fake-entitiy-%d", i),
			"m":   "juju.testing",
			"l":   "fake-file.go:1234",
			"v":   int(loggo.DEBUG),
			"x":   "test message",
		}
		err := oldLogs.Insert(logRow)
		c.Assert(err, jc.ErrorIsNil)

		delete(logRow, "e")
		vals := expected[modelUUID]
		expected[modelUUID] = append(vals, logRow)
	}

	err := SplitLogCollections(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// Now check the logs.
	for _, uuid := range uuids {
		newLogs := db.C(fmt.Sprintf("logs.%s", uuid))
		numDocs, err := newLogs.Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(numDocs, gc.Equals, 5)

		var docs []bson.M
		err = newLogs.Find(nil).All(&docs)
		c.Assert(err, jc.ErrorIsNil)

		sort.Sort(bsonMById(docs))
		c.Assert(docs, jc.DeepEquals, expected[uuid])
	}

	numDocs, err := oldLogs.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numDocs, gc.Equals, 0)

	// Run again, should be fine.
	err = SplitLogCollections(s.state)
	c.Logf("%#v", errors.Cause(err))
	c.Assert(err, jc.ErrorIsNil)

	// Now check the logs, just to be sure.
	for _, uuid := range uuids {
		newLogs := db.C(fmt.Sprintf("logs.%s", uuid))
		numDocs, err := newLogs.Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(numDocs, gc.Equals, 5)

		var docs []bson.M
		err = newLogs.Find(nil).All(&docs)
		c.Assert(err, jc.ErrorIsNil)

		sort.Sort(bsonMById(docs))
		c.Assert(docs, jc.DeepEquals, expected[uuid])
	}
}

func (s *upgradesSuite) TestSplitLogsIgnoresDupeRecordsAlreadyThere(c *gc.C) {
	db := s.state.MongoSession().DB(logsDB)
	oldLogs := db.C("logs")

	uuids := []string{"fake-1", "fake-2", "fake-3"}
	expected := map[string][]bson.M{}

	for i := 0; i < 15; i++ {
		modelUUID := uuids[i%3]
		logRow := bson.M{
			"_id": fmt.Sprintf("fake-objectid-%02d", i),
			"t":   100 * i,
			"e":   modelUUID,
			"r":   "2.1.2",
			"n":   fmt.Sprintf("fake-entitiy-%d", i),
			"m":   "juju.testing",
			"l":   "fake-file.go:1234",
			"v":   int(loggo.DEBUG),
			"x":   "test message",
		}
		err := oldLogs.Insert(logRow)
		c.Assert(err, jc.ErrorIsNil)

		delete(logRow, "e")
		vals := expected[modelUUID]
		expected[modelUUID] = append(vals, logRow)
	}

	// Put the first expected output row in each destination
	// collection already.
	for modelUUID, rows := range expected {
		targetColl := db.C("logs." + modelUUID)
		err := targetColl.Insert(rows[0])
		c.Assert(err, jc.ErrorIsNil)
	}

	err := SplitLogCollections(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// Now check the logs - the duplicates were ignored.
	for _, uuid := range uuids {
		newLogs := db.C(fmt.Sprintf("logs.%s", uuid))
		numDocs, err := newLogs.Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(numDocs, gc.Equals, 5)

		var docs []bson.M
		err = newLogs.Find(nil).All(&docs)
		c.Assert(err, jc.ErrorIsNil)

		sort.Sort(bsonMById(docs))
		c.Assert(docs, jc.DeepEquals, expected[uuid])
	}

	numDocs, err := oldLogs.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numDocs, gc.Equals, 0)
}

func (s *upgradesSuite) TestSplitLogsHandlesNoLogsCollection(c *gc.C) {
	db := s.state.MongoSession().DB(logsDB)
	names, err := db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(names...).Contains("logs"), jc.IsFalse)

	err = SplitLogCollections(s.state)
	c.Assert(err, jc.ErrorIsNil)
}
