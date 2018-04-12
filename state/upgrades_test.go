// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type upgradesSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&upgradesSuite{})

func (s *upgradesSuite) TestStripLocalUserDomainCredentials(c *gc.C) {
	coll, closer := s.state.db().GetRawCollection(cloudCredentialsC)
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
		"invalid":    false,
		"auth-type":  "userpass",
		"attributes": bson.M{"user": "fred"},
	}, {
		"_id":        "aws#fred#default",
		"owner":      "user-mary@external",
		"cloud":      "cloud-aws",
		"name":       "default",
		"revoked":    false,
		"invalid":    false,
		"auth-type":  "userpass",
		"attributes": bson.M{"user": "fred"},
	}}
	s.assertStrippedUserData(c, coll, expected)
}

func (s *upgradesSuite) TestStripLocalUserDomainModels(c *gc.C) {
	coll, closer := s.state.db().GetRawCollection(modelsC)
	defer closer()

	var initialModels []bson.M
	err := coll.Find(nil).Sort("_id").All(&initialModels)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(initialModels, gc.HasLen, 1)

	err = coll.Insert(
		modelDoc{
			Type:            ModelTypeIAAS,
			UUID:            "0000-dead-beaf-0001",
			Owner:           "user-admin@local",
			Name:            "controller",
			ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			Cloud:           "cloud-aws",
			CloudRegion:     "us-west-1",
			CloudCredential: "aws#fred@local#default",
			EnvironVersion:  0,
		},
		modelDoc{
			Type:            ModelTypeIAAS,
			UUID:            "0000-dead-beaf-0002",
			Owner:           "user-mary@external",
			Name:            "default",
			ControllerUUID:  "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			Cloud:           "cloud-aws",
			CloudRegion:     "us-west-1",
			CloudCredential: "aws#mary@external#default",
			EnvironVersion:  0,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	initialModel := initialModels[0]
	delete(initialModel, "txn-queue")
	delete(initialModel, "txn-revno")
	initialModel["owner"] = "test-admin"

	expected := []bson.M{{
		"_id":              "0000-dead-beaf-0001",
		"type":             "iaas",
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
		"environ-version":  0,
	}, {
		"_id":              "0000-dead-beaf-0002",
		"type":             "iaas",
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
		"environ-version":  0,
	},
		initialModel,
	}

	s.assertStrippedUserData(c, coll, expected)
}

func (s *upgradesSuite) TestStripLocalUserDomainModelNames(c *gc.C) {
	coll, closer := s.state.db().GetRawCollection(usermodelnameC)
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
	coll, closer := s.state.db().GetRawCollection(collName)
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
	coll, closer := s.state.db().GetRawCollection(permissionsC)
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
	coll, closer := s.state.db().GetRawCollection(modelUserLastConnectionC)
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
		c.Logf("Run: %d", i)
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
			c.Assert(docs, jc.DeepEquals, expect.expected,
				gc.Commentf("differences: %s", pretty.Diff(docs, expect.expected)))
		}
	}
}

func (s *upgradesSuite) TestRenameAddModelPermission(c *gc.C) {
	coll, closer := s.state.db().GetRawCollection(permissionsC)
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
	coll, closer := s.state.db().GetRawCollection(migrationsC)
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

	charms, closer := s.state.db().GetRawCollection(charmsC)
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

	sequences, closer := s.state.db().GetRawCollection(sequenceC)
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

func (s *upgradesSuite) TestUpdateLegacyLXDCloud(c *gc.C) {
	cloudColl, cloudCloser := s.state.db().GetRawCollection(cloudsC)
	defer cloudCloser()
	cloudCredColl, cloudCredCloser := s.state.db().GetRawCollection(cloudCredentialsC)
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
		"_id":            "localhost#admin#streetcred",
		"owner":          "admin",
		"cloud":          "localhost",
		"name":           "streetcred",
		"revoked":        false,
		"invalid":        false,
		"invalid-reason": "",
		"auth-type":      "certificate",
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
	cloudColl, cloudCloser := s.state.db().GetRawCollection(cloudsC)
	defer cloudCloser()
	cloudCredColl, cloudCredCloser := s.state.db().GetRawCollection(cloudCredentialsC)
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
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
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
	volumesColl, volumesCloser := s.state.db().GetRawCollection(volumesC)
	defer volumesCloser()
	volumeAttachmentsColl, volumeAttachmentsCloser := s.state.db().GetRawCollection(volumeAttachmentsC)
	defer volumeAttachmentsCloser()

	filesystemsColl, filesystemsCloser := s.state.db().GetRawCollection(filesystemsC)
	defer filesystemsCloser()
	filesystemAttachmentsColl, filesystemAttachmentsCloser := s.state.db().GetRawCollection(filesystemAttachmentsC)
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
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
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
	settingsColl, settingsCloser := s.state.db().GetRawCollection(controllersC)
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
	settingsColl, settingsCloser := s.state.db().GetRawCollection(controllersC)
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

func (s *upgradesSuite) makeModel(c *gc.C, name string, attr coretesting.Attrs) *State {
	uuid := utils.MustNewUUID()
	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"name": name,
		"uuid": uuid.String(),
	}.Merge(attr))
	m, err := s.state.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, st, err := s.state.NewModel(ModelArgs{
		Type:        ModelTypeIAAS,
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
	s.checkAddPruneSettings(c, "max-status-history-age", "max-status-history-size", config.DefaultStatusHistoryAge, config.DefaultStatusHistorySize, AddStatusHistoryPruneSettings)
}

func (s *upgradesSuite) TestAddActionPruneSettings(c *gc.C) {
	s.checkAddPruneSettings(c, "max-action-results-age", "max-action-results-size", config.DefaultActionResultsAge, config.DefaultActionResultsSize, AddActionPruneSettings)
}

func (s *upgradesSuite) TestAddUpdateStatusHookSettings(c *gc.C) {
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	// One model has a valid setting that is not default.
	m1 := s.makeModel(c, "m1", coretesting.Attrs{
		"update-status-hook-interval": "20m",
	})
	defer m1.Close()

	// This model is missing a setting entirely.
	m2 := s.makeModel(c, "m2", coretesting.Attrs{})
	defer m2.Close()
	// We remove the 'update-status-hook-interval' value to
	// represent an old-style model that needs updating.
	settingsKey := m2.ModelUUID() + ":e"
	err = settingsColl.UpdateId(settingsKey,
		bson.M{"$unset": bson.M{"settings.update-status-hook-interval": ""}})
	c.Assert(err, jc.ErrorIsNil)

	// And something that isn't model settings
	err = settingsColl.Insert(bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-model setting: should not be touched
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	model1, err := m1.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg1, err := model1.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected1 := cfg1.AllAttrs()
	expected1["resource-tags"] = ""

	model2, err := m2.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg2, err := model2.ModelConfig()
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
	storageInstancesColl, storageInstancesCloser := s.state.db().GetRawCollection(storageInstancesC)
	defer storageInstancesCloser()
	storageConstraintsColl, storageConstraintsCloser := s.state.db().GetRawCollection(storageConstraintsC)
	defer storageConstraintsCloser()
	volumesColl, volumesCloser := s.state.db().GetRawCollection(volumesC)
	defer volumesCloser()
	filesystemsColl, filesystemsCloser := s.state.db().GetRawCollection(filesystemsC)
	defer filesystemsCloser()
	unitsColl, unitsCloser := s.state.db().GetRawCollection(unitsC)
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
			"m":   "juju.coretesting",
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
			"m":   "juju.coretesting",
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

func (s *upgradesSuite) TestCorrectRelationUnitCounts(c *gc.C) {
	relations, rCloser := s.state.db().GetRawCollection(relationsC)
	defer rCloser()
	scopes, sCloser := s.state.db().GetRawCollection(relationScopesC)
	defer sCloser()
	applications, aCloser := s.state.db().GetRawCollection(applicationsC)
	defer aCloser()

	// Use the non-controller model to ensure we can run the function
	// across multiple models.
	otherState := s.makeModel(c, "crack-up", coretesting.Attrs{})
	defer otherState.Close()

	uuid := otherState.ModelUUID()

	err := relations.Insert(bson.M{
		"_id":        uuid + ":min:juju-info nrpe:general-info",
		"key":        "min:juju-info nrpe:general-info",
		"model-uuid": uuid,
		"id":         4,
		"endpoints": []bson.M{{
			"applicationname": "min",
			"relation": bson.M{
				"name":      "juju-info",
				"role":      "provider",
				"interface": "juju-info",
				"optional":  false,
				"limit":     0,
				"scope":     "container",
			},
		}, {
			"applicationname": "nrpe",
			"relation": bson.M{
				"name":      "general-info",
				"role":      "requirer",
				"interface": "juju-info",
				"optional":  false,
				"limit":     1,
				"scope":     "container",
			},
		}},
		"unitcount": 6,
	}, bson.M{
		"_id":        uuid + ":ntp:ntp-peers",
		"key":        "ntp:ntp-peers",
		"model-uuid": uuid,
		"id":         3,
		"endpoints": []bson.M{{
			"applicationname": "ntp",
			"relation": bson.M{
				"name":      "ntp-peers",
				"role":      "peer",
				"interface": "ntp",
				"optional":  false,
				"limit":     1,
				"scope":     "global",
			},
		}},
		"unitcount": 2,
	}, bson.M{
		"_id":        uuid + ":ntp:juju-info nrpe:general-info",
		"key":        "ntp:juju-info nrpe:general-info",
		"model-uuid": uuid,
		"id":         5,
		"endpoints": []bson.M{{
			"applicationname": "ntp",
			"relation": bson.M{
				"name":      "juju-info",
				"role":      "provider",
				"interface": "juju-info",
				"optional":  false,
				"limit":     0,
				"scope":     "container",
			},
		}, {
			"applicationname": "nrpe",
			"relation": bson.M{
				"name":      "general-info",
				"role":      "requirer",
				"interface": "juju-info",
				"optional":  false,
				"limit":     1,
				"scope":     "container",
			},
		}},
		"unitcount": 4,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = scopes.Insert(bson.M{
		"_id":        uuid + ":r#4#min/0#provider#min/0",
		"key":        "r#4#min/0#provider#min/0",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#4#min/0#requirer#nrpe/0",
		"key":        "r#4#min/0#requirer#nrpe/0",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#4#min/1#provider#min/1",
		"key":        "r#4#min/1#provider#min/1",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#4#min/1#requirer#nrpe/1",
		"key":        "r#4#min/1#requirer#nrpe/1",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#4#min2/0#requirer#nrpe/2",
		"key":        "r#4#min2/0#requirer#nrpe/2",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#4#min2/1#requirer#nrpe/3",
		"key":        "r#4#min2/1#requirer#nrpe/3",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#3#peer#ntp/0",
		"key":        "r#3#peer#ntp/0",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#3#peer#ntp/1",
		"key":        "r#3#peer#ntp/1",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#5#min/0#provider#ntp/0",
		"key":        "r#5#min/0#provider#ntp/0",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#5#min/0#requirer#nrpe/0",
		"key":        "r#5#min/0#requirer#nrpe/0",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#5#min/1#provider#ntp/1",
		"key":        "r#5#min/1#provider#ntp/1",
		"model-uuid": uuid,
		"departing":  false,
	}, bson.M{
		"_id":        uuid + ":r#5#min/1#requirer#nrpe/1",
		"key":        "r#5#min/1#requirer#nrpe/1",
		"model-uuid": uuid,
		"departing":  false,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = applications.Insert(bson.M{
		"_id":         uuid + ":min",
		"name":        "min",
		"model-uuid":  uuid,
		"subordinate": false,
	}, bson.M{
		"_id":         uuid + ":ntp",
		"name":        "ntp",
		"model-uuid":  uuid,
		"subordinate": true,
	}, bson.M{
		"_id":         uuid + ":nrpe",
		"name":        "nrpe",
		"model-uuid":  uuid,
		"subordinate": true,
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedRelations := []bson.M{{
		"_id":        uuid + ":min:juju-info nrpe:general-info",
		"key":        "min:juju-info nrpe:general-info",
		"model-uuid": uuid,
		"id":         4,
		"endpoints": []interface{}{bson.M{
			"applicationname": "min",
			"relation": bson.M{
				"name":      "juju-info",
				"role":      "provider",
				"interface": "juju-info",
				"optional":  false,
				"limit":     0,
				"scope":     "container",
			},
		}, bson.M{
			"applicationname": "nrpe",
			"relation": bson.M{
				"name":      "general-info",
				"role":      "requirer",
				"interface": "juju-info",
				"optional":  false,
				"limit":     1,
				"scope":     "container",
			},
		}},
		"unitcount": 4,
	}, {
		"_id":        uuid + ":ntp:juju-info nrpe:general-info",
		"key":        "ntp:juju-info nrpe:general-info",
		"model-uuid": uuid,
		"id":         5,
		"endpoints": []interface{}{bson.M{
			"applicationname": "ntp",
			"relation": bson.M{
				"name":      "juju-info",
				"role":      "provider",
				"interface": "juju-info",
				"optional":  false,
				"limit":     0,
				"scope":     "container",
			},
		}, bson.M{
			"applicationname": "nrpe",
			"relation": bson.M{
				"name":      "general-info",
				"role":      "requirer",
				"interface": "juju-info",
				"optional":  false,
				"limit":     1,
				"scope":     "container",
			},
		}},
		"unitcount": 4,
	}, {
		"_id":        uuid + ":ntp:ntp-peers",
		"key":        "ntp:ntp-peers",
		"model-uuid": uuid,
		"id":         3,
		"endpoints": []interface{}{bson.M{
			"applicationname": "ntp",
			"relation": bson.M{
				"name":      "ntp-peers",
				"role":      "peer",
				"interface": "ntp",
				"optional":  false,
				"limit":     1,
				"scope":     "global",
			},
		}},
		"unitcount": 2,
	}}
	expectedScopes := []bson.M{{
		"_id":        uuid + ":r#3#peer#ntp/0",
		"key":        "r#3#peer#ntp/0",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#3#peer#ntp/1",
		"key":        "r#3#peer#ntp/1",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#4#min/0#provider#min/0",
		"key":        "r#4#min/0#provider#min/0",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#4#min/0#requirer#nrpe/0",
		"key":        "r#4#min/0#requirer#nrpe/0",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#4#min/1#provider#min/1",
		"key":        "r#4#min/1#provider#min/1",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#4#min/1#requirer#nrpe/1",
		"key":        "r#4#min/1#requirer#nrpe/1",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#5#min/0#provider#ntp/0",
		"key":        "r#5#min/0#provider#ntp/0",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#5#min/0#requirer#nrpe/0",
		"key":        "r#5#min/0#requirer#nrpe/0",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#5#min/1#provider#ntp/1",
		"key":        "r#5#min/1#provider#ntp/1",
		"model-uuid": uuid,
		"departing":  false,
	}, {
		"_id":        uuid + ":r#5#min/1#requirer#nrpe/1",
		"key":        "r#5#min/1#requirer#nrpe/1",
		"model-uuid": uuid,
		"departing":  false,
	}}
	s.assertUpgradedData(c, CorrectRelationUnitCounts,
		expectUpgradedData{relations, expectedRelations},
		expectUpgradedData{scopes, expectedScopes},
	)
}

func (s *upgradesSuite) TestAddModelEnvironVersion(c *gc.C) {
	models, closer := s.state.db().GetRawCollection(modelsC)
	defer closer()

	err := models.RemoveId(s.state.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)

	err = models.Insert(bson.M{
		"_id": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
	}, bson.M{
		"_id":             "deadbeef-0bad-400d-8000-4b1d0d06f00e",
		"environ-version": 1,
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedModels := []bson.M{{
		"_id":             "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"environ-version": 0,
	}, {
		"_id":             "deadbeef-0bad-400d-8000-4b1d0d06f00e",
		"environ-version": 1,
	}}
	s.assertUpgradedData(c, AddModelEnvironVersion,
		expectUpgradedData{models, expectedModels},
	)
}

func (s *upgradesSuite) TestAddModelType(c *gc.C) {
	models, closer := s.state.db().GetRawCollection(modelsC)
	defer closer()

	err := models.RemoveId(s.state.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)

	err = models.Insert(
		bson.M{
			"_id": "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		}, bson.M{
			"_id":  "deadbeef-0bad-400d-8000-4b1d0d06f00e",
			"type": "caas",
		})
	c.Assert(err, jc.ErrorIsNil)

	expectedModels := []bson.M{{
		"_id":  "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"type": "iaas",
	}, {
		"_id":  "deadbeef-0bad-400d-8000-4b1d0d06f00e",
		"type": "caas",
	}}
	s.assertUpgradedData(c, AddModelType,
		expectUpgradedData{models, expectedModels})
}

func (s *upgradesSuite) checkAddPruneSettings(c *gc.C, ageProp, sizeProp, defaultAge, defaultSize string, updateFunc func(st *State) error) {
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	m1 := s.makeModel(c, "m1", coretesting.Attrs{
		ageProp:  "96h",
		sizeProp: "4G",
	})
	defer m1.Close()

	m2 := s.makeModel(c, "m2", coretesting.Attrs{})
	defer m2.Close()

	err = settingsColl.Insert(bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-model setting: should not be touched
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	model1, err := m1.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg1, err := model1.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected1 := cfg1.AllAttrs()
	expected1["resource-tags"] = ""

	model2, err := m2.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg2, err := model2.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected2 := cfg2.AllAttrs()
	expected2[ageProp] = defaultAge
	expected2[sizeProp] = defaultSize
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

	s.assertUpgradedData(c, updateFunc,
		expectUpgradedData{settingsColl, expectedSettings},
	)
}

func (s *upgradesSuite) TestMigrateLeasesToGlobalTime(c *gc.C) {
	leases, closer := s.state.db().GetRawCollection(leasesC)
	defer closer()

	// Use the non-controller model to ensure we can run the function
	// across multiple models.
	otherState := s.makeModel(c, "crack-up", coretesting.Attrs{})
	defer otherState.Close()

	uuid := otherState.ModelUUID()

	err := leases.Insert(bson.M{
		"_id":        uuid + ":some-garbage",
		"model-uuid": uuid,
	}, bson.M{
		"_id":        uuid + ":clock#some-namespace#some-name#",
		"model-uuid": uuid,
		"type":       "clock",
	}, bson.M{
		"_id":        uuid + ":lease#some-namespace#some-name#",
		"model-uuid": uuid,
		"type":       "lease",
		"namespace":  "some-namespace",
		"name":       "some-name",
		"holder":     "hand",
		"expiry":     "later",
		"writer":     "ghost",
	})
	c.Assert(err, jc.ErrorIsNil)

	// - garbage doc is left alone has it has no "type" field
	// - clock doc is removed, but no replacement required
	// - lease doc is removed and replaced
	expectedLeases := []bson.M{{
		"_id":        uuid + ":some-garbage",
		"model-uuid": uuid,
	}, bson.M{
		"_id":        uuid + ":some-namespace#some-name#",
		"model-uuid": uuid,
		"namespace":  "some-namespace",
		"name":       "some-name",
		"holder":     "hand",
		"start":      int64(0),
		"duration":   int64(time.Minute),
		"writer":     "ghost",
	}}
	s.assertUpgradedData(c, MigrateLeasesToGlobalTime,
		expectUpgradedData{leases, expectedLeases},
	)
}

func (s *upgradesSuite) TestMoveOldAuditLogNoRecords(c *gc.C) {
	// Ensure an empty audit log collection exists.
	auditLog, closer := s.state.db().GetRawCollection("audit.log")
	defer closer()
	err := auditLog.Create(&mgo.CollectionInfo{})
	c.Assert(err, jc.ErrorIsNil)

	// Sanity check.
	count, err := auditLog.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(count, gc.Equals, 0)

	err = MoveOldAuditLog(s.state)
	c.Assert(err, jc.ErrorIsNil)

	db := s.state.MongoSession().DB("juju")
	names, err := db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(names...).Contains("audit.log"), jc.IsFalse)

	err = MoveOldAuditLog(s.state)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestMoveOldAuditLogRename(c *gc.C) {
	auditLog, closer := s.state.db().GetRawCollection("audit.log")
	defer closer()
	oldLog, oldCloser := s.state.db().GetRawCollection("old-audit.log")
	defer oldCloser()

	// Put some rows into audit log and check that they're moved.
	data := []bson.M{
		{"_id": "band", "king": "gizzard", "lizard": "wizard"},
		{"_id": "song", "crumbling": "castle"},
	}
	err := auditLog.Insert(data[0], data[1])
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgradedData(c, MoveOldAuditLog,
		expectUpgradedData{oldLog, data},
	)

	db := s.state.MongoSession().DB("juju")
	names, err := db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(names...).Contains("audit.log"), jc.IsFalse)
}

func (s *upgradesSuite) TestMigrateLeasesToGlobalTimeWithNewTarget(c *gc.C) {
	// It is possible that API servers will try to coordinate the singular lease before we can get to the upgrade steps.
	// While upgrading leases, if we encounter any leases that already exist in the new GlobalTime format, they should
	// be considered authoritative, and the old lease should just be deleted.
	leases, closer := s.state.db().GetRawCollection(leasesC)
	defer closer()

	// Use the non-controller model to ensure we can run the function
	// across multiple models.
	otherState := s.makeModel(c, "crack-up", coretesting.Attrs{})
	defer otherState.Close()

	uuid := otherState.ModelUUID()

	err := leases.Insert(bson.M{
		"_id":        uuid + ":some-garbage",
		"model-uuid": uuid,
	}, bson.M{
		"_id":        uuid + ":clock#some-namespace#some-name#",
		"model-uuid": uuid,
		"type":       "clock",
	}, bson.M{
		"_id":        uuid + ":lease#some-namespace#some-name#",
		"model-uuid": uuid,
		"type":       "lease",
		"namespace":  "some-namespace",
		"name":       "some-name",
		"holder":     "hand",
		"expiry":     "later",
		"writer":     "ghost",
	}, bson.M{
		"_id":        uuid + ":lease#some-namespace2#some-name2#",
		"model-uuid": uuid,
		"type":       "lease",
		"namespace":  "some-namespace2",
		"name":       "some-name2",
		"holder":     "hand",
		"expiry":     "later",
		"writer":     "ghost",
	}, bson.M{
		// some-namespace2 has already been created in the new format
		"_id":        uuid + ":some-namespace2#some-name2#",
		"model-uuid": uuid,
		"namespace":  "some-namespace2",
		"name":       "some-name2",
		"holder":     "foot",
		"start":      int64(0),
		"duration":   int64(time.Minute),
		"writer":     "gobble",
	})
	c.Assert(err, jc.ErrorIsNil)

	// - garbage doc is left alone has it has no "type" field
	// - clock doc is removed, but no replacement required
	// - lease doc is removed and replaced
	// - second old lease doc is removed, and the new lease doc is not overwritten
	expectedLeases := []bson.M{{
		"_id":        uuid + ":some-garbage",
		"model-uuid": uuid,
	}, {
		"_id":        uuid + ":some-namespace#some-name#",
		"model-uuid": uuid,
		"namespace":  "some-namespace",
		"name":       "some-name",
		"holder":     "hand",
		"start":      int64(0),
		"duration":   int64(time.Minute),
		"writer":     "ghost",
	}, {
		"_id":        uuid + ":some-namespace2#some-name2#",
		"model-uuid": uuid,
		"namespace":  "some-namespace2",
		"name":       "some-name2",
		"holder":     "foot",
		"start":      int64(0),
		"duration":   int64(time.Minute),
		"writer":     "gobble",
	}}
	s.assertUpgradedData(c, MigrateLeasesToGlobalTime,
		expectUpgradedData{leases, expectedLeases},
	)
}

func (s *upgradesSuite) TestAddRelationStatus(c *gc.C) {
	// Set a test clock so we can dictate the
	// time set in the new status doc.
	clock := testing.NewClock(time.Unix(0, 123))
	s.state.SetClockForTesting(clock)

	relations, closer := s.state.db().GetRawCollection(relationsC)
	defer closer()

	statuses, closer := s.state.db().GetRawCollection(statusesC)
	defer closer()

	err := relations.Insert(bson.M{
		"_id":        s.state.ModelUUID() + ":0",
		"id":         0,
		"model-uuid": s.state.ModelUUID(),
	}, bson.M{
		"_id":        s.state.ModelUUID() + ":1",
		"id":         1,
		"model-uuid": s.state.ModelUUID(),
		"unitcount":  1,
	}, bson.M{
		"_id":        s.state.ModelUUID() + ":2",
		"id":         2,
		"model-uuid": s.state.ModelUUID(),
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = statuses.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = statuses.Insert(bson.M{
		"_id":        s.state.ModelUUID() + ":r#2",
		"model-uuid": s.state.ModelUUID(),
		"status":     "broken",
		"statusdata": bson.M{},
		"statusinfo": "",
		"updated":    int64(321),
		"neverset":   false,
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedStatuses := []bson.M{{
		"_id":        s.state.ModelUUID() + ":r#0",
		"model-uuid": s.state.ModelUUID(),
		"status":     "joining",
		"statusdata": bson.M{},
		"statusinfo": "",
		"updated":    int64(123),
		"neverset":   false,
	}, {
		"_id":        s.state.ModelUUID() + ":r#1",
		"model-uuid": s.state.ModelUUID(),
		"status":     "joined",
		"statusdata": bson.M{},
		"statusinfo": "",
		"updated":    int64(123),
		"neverset":   false,
	}, {
		"_id":        s.state.ModelUUID() + ":r#2",
		"model-uuid": s.state.ModelUUID(),
		"status":     "broken",
		"statusdata": bson.M{},
		"statusinfo": "",
		"updated":    int64(321),
		"neverset":   false,
	}}

	s.assertUpgradedData(c, AddRelationStatus,
		expectUpgradedData{statuses, expectedStatuses},
	)
}

func (s *upgradesSuite) TestDeleteCloudImageMetadata(c *gc.C) {
	stor := cloudimagemetadata.NewStorage(cloudimagemetadataC, &environMongo{s.state})
	attrs1 := cloudimagemetadata.MetadataAttributes{
		Stream:  "stream",
		Region:  "region-test",
		Version: "14.04",
		Series:  "trusty",
		Arch:    "arch",
		Source:  "custom",
	}
	attrs2 := cloudimagemetadata.MetadataAttributes{
		Stream:  "chalk",
		Region:  "nether",
		Version: "12.04",
		Series:  "precise",
		Arch:    "amd64",
		Source:  "test",
	}
	now := time.Now().UnixNano()
	added := []cloudimagemetadata.Metadata{
		{attrs1, 0, "1", now},
		{attrs2, 0, "2", now},
	}
	err := stor.SaveMetadataNoExpiry(added)
	c.Assert(err, jc.ErrorIsNil)

	expected := []bson.M{{
		"_id":               "stream:region-test:trusty:arch:::custom",
		"date_created":      now,
		"image_id":          "1",
		"priority":          0,
		"stream":            "stream",
		"region":            "region-test",
		"series":            "trusty",
		"arch":              "arch",
		"root_storage_size": int64(0),
		"source":            "custom",
	}}

	coll, closer := s.state.db().GetRawCollection(cloudimagemetadataC)
	defer closer()
	s.assertUpgradedData(c, DeleteCloudImageMetadata, expectUpgradedData{coll, expected})
}

func (s *upgradesSuite) TestCopyMongoSpaceToHASpaceConfigWhenValid(c *gc.C) {
	c.Assert(getHASpaceConfig(s.state, c), gc.Equals, "")

	sn := "mongo-space"
	controllerColl, controllerCloser := s.state.db().GetRawCollection(controllersC)
	defer controllerCloser()
	err := controllerColl.UpdateId(modelGlobalKey, bson.M{"$set": bson.M{
		"mongo-space-name":  sn,
		"mongo-space-state": "valid",
	}})
	c.Assert(err, jc.ErrorIsNil)

	err = MoveMongoSpaceToHASpaceConfig(s.state)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(getHASpaceConfig(s.state, c), gc.Equals, sn)
}

func (s *upgradesSuite) TestNoCopyMongoSpaceToHASpaceConfigWhenNotValid(c *gc.C) {
	c.Assert(getHASpaceConfig(s.state, c), gc.Equals, "")

	sn := "mongo-space"
	controllerColl, controllerCloser := s.state.db().GetRawCollection(controllersC)
	defer controllerCloser()
	err := controllerColl.UpdateId(modelGlobalKey, bson.M{"$set": bson.M{
		"mongo-space-name":  sn,
		"mongo-space-state": "invalid",
	}})
	c.Assert(err, jc.ErrorIsNil)

	err = MoveMongoSpaceToHASpaceConfig(s.state)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(getHASpaceConfig(s.state, c), gc.Equals, "")
}

func (s *upgradesSuite) TestNoCopyMongoSpaceToHASpaceConfigWhenAlreadySet(c *gc.C) {
	settings, err := readSettings(s.state.db(), controllersC, controllerSettingsGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	settings.Set(controller.JujuHASpace, "already-set")
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)

	controllerColl, controllerCloser := s.state.db().GetRawCollection(controllersC)
	defer controllerCloser()
	err = controllerColl.UpdateId(modelGlobalKey, bson.M{"$set": bson.M{
		"mongo-space-name":  "should-not-be-copied",
		"mongo-space-state": "valid",
	}})
	c.Assert(err, jc.ErrorIsNil)

	err = MoveMongoSpaceToHASpaceConfig(s.state)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(getHASpaceConfig(s.state, c), gc.Equals, "already-set")
}

func (s *upgradesSuite) TestMoveMongoSpaceToHASpaceConfigDeletesOldKeys(c *gc.C) {
	controllerColl, controllerCloser := s.state.db().GetRawCollection(controllersC)
	defer controllerCloser()
	err := controllerColl.UpdateId(modelGlobalKey, bson.M{"$set": bson.M{
		"mongo-space-name":  "whatever",
		"mongo-space-state": "valid",
	}})
	c.Assert(err, jc.ErrorIsNil)

	err = MoveMongoSpaceToHASpaceConfig(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// Holds Mongo space fields removed from controllersDoc.
	type controllersUpgradeDoc struct {
		MongoSpaceName  string `bson:"mongo-space-name"`
		MongoSpaceState string `bson:"mongo-space-state"`
	}
	var doc controllersUpgradeDoc
	err = controllerColl.Find(bson.D{{"_id", modelGlobalKey}}).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(doc.MongoSpaceName, gc.Equals, "")
	c.Check(doc.MongoSpaceState, gc.Equals, "")
}

func getHASpaceConfig(st *State, c *gc.C) string {
	cfg, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	return cfg.JujuHASpace()
}

func (s *upgradesSuite) TestCreateMissingApplicationConfig(c *gc.C) {
	// Setup models w/ applications that have setting configurations as if we've updated from <2.4-beta1 -> 2.4-beta1
	// At least 2x models, one that was created before the update and one after (i.e. 1 missing the config and another that has that in place.)
	// Ensure an update adds any missing applicationConfig entries.
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()

	model1 := s.makeModel(c, "model-old", coretesting.Attrs{})
	defer model1.Close()
	model2 := s.makeModel(c, "model-new", coretesting.Attrs{})
	defer model2.Close()

	chDir := testcharms.Repo.CharmDir("dummy")
	chInfo := CharmInfo{
		Charm:       chDir,
		ID:          charm.MustParseURL(fmt.Sprintf("cs:xenial/%s-%d", chDir.Meta().Name, chDir.Revision())),
		StoragePath: "dummy-1",
		SHA256:      "dummy-1-sha256",
	}
	ch, err := s.state.AddCharm(chInfo)
	c.Assert(err, jc.ErrorIsNil)

	app1, err := model1.AddApplication(AddApplicationArgs{Name: "dummy", Charm: ch})
	c.Assert(err, jc.ErrorIsNil)
	// This one will be left alone to model a 2.4-beta1 created app.
	_, err = model1.AddApplication(AddApplicationArgs{Name: "dummy2", Charm: ch})
	c.Assert(err, jc.ErrorIsNil)
	app2, err := model2.AddApplication(AddApplicationArgs{Name: "dummy", Charm: ch})
	c.Assert(err, jc.ErrorIsNil)

	// Remove any application config that has been added (to model a pre-2.4-beta1 collection)
	err = settingsColl.Remove(bson.M{
		"_id": fmt.Sprintf("%s:%s", model1.ModelUUID(), app1.applicationConfigKey()),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = settingsColl.Remove(bson.M{
		"_id": fmt.Sprintf("%s:%s", model2.ModelUUID(), app2.applicationConfigKey()),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Remove everything except the remaining application config.
	_, err = settingsColl.RemoveAll(bson.M{
		"_id": bson.M{"$not": bson.RegEx{"#application$", ""}},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := []bson.M{{
		"_id":        fmt.Sprintf("%s:a#dummy#application", model1.ModelUUID()),
		"model-uuid": model1.ModelUUID(),
		"settings":   bson.M{},
	}, {
		"_id":        fmt.Sprintf("%s:a#dummy2#application", model1.ModelUUID()),
		"model-uuid": model1.ModelUUID(),
		"settings":   bson.M{},
	}, {
		"_id":        fmt.Sprintf("%s:a#dummy#application", model2.ModelUUID()),
		"model-uuid": model2.ModelUUID(),
		"settings":   bson.M{},
	}}

	sort.Slice(expected, func(i, j int) bool {
		return expected[i]["_id"].(string) < expected[j]["_id"].(string)
	})

	s.assertUpgradedData(c, CreateMissingApplicationConfig,
		expectUpgradedData{settingsColl, expected})
}

func (s *upgradesSuite) TestRemoveVotingMachineIds(c *gc.C) {
	// Setup the database with a 2.3 controller info which had 'votingmachineids'
	controllerColl, controllerCloser := s.state.db().GetRawCollection(controllersC)
	defer controllerCloser()
	err := controllerColl.UpdateId(modelGlobalKey, bson.M{"$set": bson.M{"votingmachineids": []string{"0"}}})
	c.Assert(err, jc.ErrorIsNil)
	// The only document we should touch is the modelGlobalKey
	var expectedDocs []bson.M
	err = controllerColl.Find(nil).Sort("_id").All(&expectedDocs)
	c.Assert(err, jc.ErrorIsNil)
	for _, doc := range expectedDocs {
		delete(doc, "txn-queue")
		delete(doc, "txn-revno")
		delete(doc, "version")
		if doc["_id"] != modelGlobalKey {
			continue
		}
		delete(doc, "votingmachineids")
	}
	s.assertUpgradedData(c, RemoveVotingMachineIds, expectUpgradedData{coll: controllerColl, expected: expectedDocs})
}
