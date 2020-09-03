// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/clock/testclock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	mongoutils "github.com/juju/juju/mongo/utils"
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
		"passwordhash":     "",
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
		"passwordhash":     "",
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
		"_id": "test-admin:testmodel",
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
	c.Assert(initialPermissions, gc.HasLen, 3)

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

	for i, initial := range initialPermissions {
		perm := initial
		delete(perm, "txn-queue")
		delete(perm, "txn-revno")
		initialPermissions[i] = perm
	}

	expected := []bson.M{initialPermissions[0], initialPermissions[1], initialPermissions[2], {
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
	s.assertUpgradedData(c, StripLocalUserDomain, upgradedData(coll, expected))
}

type expectUpgradedData struct {
	coll     *mgo.Collection
	expected []bson.M
	filter   bson.D
}

func upgradedData(coll *mgo.Collection, expected []bson.M) expectUpgradedData {
	return expectUpgradedData{
		coll:     coll,
		expected: expected,
	}
}

func upgradedDataWithFilter(coll *mgo.Collection, expected []bson.M, filter bson.D) expectUpgradedData {
	return expectUpgradedData{
		coll:     coll,
		expected: expected,
		filter:   filter,
	}
}

func (s *upgradesSuite) assertUpgradedData(c *gc.C, upgrade func(*StatePool) error, expect ...expectUpgradedData) {
	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := upgrade(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		for _, expect := range expect {
			var docs []bson.M
			err = expect.coll.Find(expect.filter).Sort("_id").All(&docs)
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
	c.Assert(initialPermissions, gc.HasLen, 3)

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

	for i, initial := range initialPermissions {
		perm := initial
		delete(perm, "txn-queue")
		delete(perm, "txn-revno")
		initialPermissions[i] = perm
	}

	expected := []bson.M{initialPermissions[0], initialPermissions[1], initialPermissions[2], {
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
	s.assertUpgradedData(c, RenameAddModelPermission, upgradedData(coll, expected))
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
		{
			"_id":     "uuid:1",
			"attempt": 1,
		},
		{
			"_id":     "uuid:11",
			"attempt": 11,
		},
		{
			"_id":     "uuid:2",
			"attempt": 2,
		},
	}
	s.assertUpgradedData(c, AddMigrationAttempt, upgradedData(coll, expected))
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
		upgradedData(sequences, expected),
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
	f := func(pool *StatePool) error {
		st := pool.SystemState()
		return UpdateLegacyLXDCloudCredentials(st, "foo", newCred)
	}
	s.assertUpgradedData(c, f,
		upgradedData(cloudColl, expectedClouds),
		upgradedData(cloudCredColl, expectedCloudCreds),
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
	f := func(pool *StatePool) error {
		st := pool.SystemState()
		return UpdateLegacyLXDCloudCredentials(st, "foo", newCred)
	}
	s.assertUpgradedData(c, f,
		upgradedData(cloudColl, expectedClouds),
		upgradedData(cloudCredColl, expectedCloudCreds),
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
		upgradedData(settingsColl, expectedSettings),
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
	}, bson.M{
		"_id":        uuid + ":3",
		"name":       "3",
		"model-uuid": uuid,
		"hostid":     "666",
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
	}, bson.M{
		"_id":          uuid + ":3",
		"filesystemid": "3",
		"model-uuid":   uuid,
		"hostid":       "666",
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
	}, {
		"_id":        uuid + ":3",
		"name":       "3",
		"model-uuid": uuid,
		"hostid":     "666",
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
	}, {
		"_id":          uuid + ":3",
		"filesystemid": "3",
		"model-uuid":   uuid,
		"hostid":       "666",
	}}

	s.assertUpgradedData(c, AddNonDetachableStorageMachineId,
		upgradedData(volumesColl, expectedVolumes),
		upgradedData(filesystemsColl, expectedFilesystems),
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
		upgradedData(settingsColl, expectedSettings),
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
			"max-txn-log-size": "8G",
			"model-logs-size":  "5M",
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
				"max-txn-log-size": "8G",
				"model-logs-size":  "5M",
			},
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}

	s.assertUpgradedData(c, AddControllerLogCollectionsSizeSettings,
		upgradedData(settingsColl, expectedSettings),
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
				"max-txn-log-size": "10M",
			},
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}

	s.assertUpgradedData(c, AddControllerLogCollectionsSizeSettings,
		upgradedData(settingsColl, expectedSettings),
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
	_, st, err := s.controller.NewModel(ModelArgs{
		Type:                    ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   m.Owner(),
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
		upgradedData(settingsColl, expectedSettings),
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
		upgradedData(storageInstancesColl, expectedStorageInstances),
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

	err := SplitLogCollections(s.pool)
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
	err = SplitLogCollections(s.pool)
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

	err := SplitLogCollections(s.pool)
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
	cols, err := db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(cols...).Contains("logs"), jc.IsFalse)

	err = SplitLogCollections(s.pool)
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
		upgradedData(relations, expectedRelations),
		upgradedData(scopes, expectedScopes),
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
		upgradedData(models, expectedModels),
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
		upgradedData(models, expectedModels))
}

func (s *upgradesSuite) checkAddPruneSettings(c *gc.C, ageProp, sizeProp, defaultAge, defaultSize string, updateFunc func(pool *StatePool) error) {
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
		upgradedData(settingsColl, expectedSettings),
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

	err = MoveOldAuditLog(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	db := s.state.MongoSession().DB("juju")
	cols, err := db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(cols...).Contains("audit.log"), jc.IsFalse)

	err = MoveOldAuditLog(s.pool)
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
	s.assertUpgradedData(c, MoveOldAuditLog, upgradedData(oldLog, data))

	db := s.state.MongoSession().DB("juju")
	cols, err := db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(cols...).Contains("audit.log"), jc.IsFalse)
}

func (s *upgradesSuite) TestAddRelationStatus(c *gc.C) {
	// Set a test clock so we can dictate the
	// time set in the new status doc.
	clock := testclock.NewClock(time.Unix(0, 123))
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
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedStatuses := []bson.M{{
		"_id":        s.state.ModelUUID() + ":r#0",
		"model-uuid": s.state.ModelUUID(),
		"status":     "joining",
		"statusdata": bson.M{},
		"statusinfo": "",
		"updated":    int64(123),
	}, {
		"_id":        s.state.ModelUUID() + ":r#1",
		"model-uuid": s.state.ModelUUID(),
		"status":     "joined",
		"statusdata": bson.M{},
		"statusinfo": "",
		"updated":    int64(123),
	}, {
		"_id":        s.state.ModelUUID() + ":r#2",
		"model-uuid": s.state.ModelUUID(),
		"status":     "broken",
		"statusdata": bson.M{},
		"statusinfo": "",
		"updated":    int64(321),
	}}

	s.assertUpgradedData(c, AddRelationStatus,
		upgradedData(statuses, expectedStatuses),
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
	err := stor.SaveMetadata(added)
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
	s.assertUpgradedData(c, DeleteCloudImageMetadata, upgradedData(coll, expected))
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

	err = MoveMongoSpaceToHASpaceConfig(s.pool)
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

	err = MoveMongoSpaceToHASpaceConfig(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(getHASpaceConfig(s.state, c), gc.Equals, "")
}

func (s *upgradesSuite) TestNoCopyMongoSpaceToHASpaceConfigWhenAlreadySet(c *gc.C) {
	settings, err := readSettings(s.state.db(), controllersC, ControllerSettingsGlobalKey)
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

	err = MoveMongoSpaceToHASpaceConfig(s.pool)
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

	err = MoveMongoSpaceToHASpaceConfig(s.pool)
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
		upgradedData(settingsColl, expected))
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
	s.assertUpgradedData(c, RemoveVotingMachineIds, upgradedData(controllerColl, expectedDocs))
}

func (s *upgradesSuite) TestUpgradeContainerImageStreamDefault(c *gc.C) {
	// Value not set
	m1 := s.makeModel(c, "m1", coretesting.Attrs{
		"other-setting":  "val",
		"dotted.setting": "value",
		"dollar$setting": "value",
	})
	defer m1.Close()
	// Value set to the empty string
	m2 := s.makeModel(c, "m2", coretesting.Attrs{
		"container-image-stream": "",
		"other-setting":          "val",
	})
	defer m2.Close()
	// Value set to something other that default
	m3 := s.makeModel(c, "m3", coretesting.Attrs{
		"container-image-stream": "daily",
	})
	defer m3.Close()

	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	// To simulate a 2.3.5 without any setting, delete the record from it.
	err := settingsColl.UpdateId(m1.ModelUUID()+":e",
		bson.M{"$unset": bson.M{"settings.container-image-stream": 1}},
	)
	c.Assert(err, jc.ErrorIsNil)
	// And an extra document from somewhere else that we shouldn't touch
	err = settingsColl.Insert(
		bson.M{
			"_id":      "not-a-model",
			"settings": bson.M{"other-setting": "val"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Read all the settings from the database, but make sure to change the
	// documents we think we're changing, and the rest should go through
	// unchanged.
	var rawSettings bson.M
	iter := settingsColl.Find(nil).Sort("_id").Iter()
	defer iter.Close()

	expectedSettings := []bson.M{}

	expectedChanges := map[string]bson.M{
		m1.ModelUUID() + ":e": {"container-image-stream": "released", "other-setting": "val"},
		m2.ModelUUID() + ":e": {"container-image-stream": "released", "other-setting": "val"},
		m3.ModelUUID() + ":e": {"container-image-stream": "daily"},
		"not-a-model":         {"other-setting": "val"},
	}
	for iter.Next(&rawSettings) {
		expSettings := copyMap(rawSettings, nil)
		delete(expSettings, "txn-queue")
		delete(expSettings, "txn-revno")
		delete(expSettings, "version")
		id, ok := expSettings["_id"]
		c.Assert(ok, jc.IsTrue)
		idStr, ok := id.(string)
		c.Assert(ok, jc.IsTrue)
		c.Assert(idStr, gc.Not(gc.Equals), "")
		if changes, ok := expectedChanges[idStr]; ok {
			raw, ok := expSettings["settings"]
			c.Assert(ok, jc.IsTrue)
			settings, ok := raw.(bson.M)
			c.Assert(ok, jc.IsTrue)
			for k, v := range changes {
				settings[k] = v
			}
		}
		expectedSettings = append(expectedSettings, expSettings)
	}
	c.Assert(iter.Close(), jc.ErrorIsNil)

	s.assertUpgradedData(c, UpgradeContainerImageStreamDefault,
		upgradedData(settingsColl, expectedSettings),
	)
}

func (s *upgradesSuite) TestRemoveContainerImageStreamFromNonModelSettings(c *gc.C) {
	// a model with a valid setting
	m1 := s.makeModel(c, "m1", coretesting.Attrs{
		"other-setting":          "val",
		"container-image-stream": "released",
	})
	defer m1.Close()
	// a model with a custom setting
	m2 := s.makeModel(c, "m2", coretesting.Attrs{
		"container-image-stream": "daily",
		"other-setting":          "val",
	})
	defer m2.Close()

	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	// A document that isn't a model with an accidental setting
	err := settingsColl.Insert(
		bson.M{
			"_id": "not-a-model",
			"settings": bson.M{
				"container-image-stream":               "released",
				"other-setting":                        "val",
				mongoutils.EscapeKey("dotted.setting"): "value",
				mongoutils.EscapeKey("dollar$setting"): "value",
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	// A document that doesn't have the setting
	err = settingsColl.Insert(
		bson.M{
			"_id": "applicationsetting",
			"settings": bson.M{
				"other-setting": "val",
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	// A document that has the setting, but it shouldn't be touched because it is a custom value.
	err = settingsColl.Insert(
		bson.M{
			"_id": "otherapplication",
			"settings": bson.M{
				"container-image-stream": "custom",
				"other-setting":          "val",
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Read all the settings from the database, and change the 'not-a-model'
	// content which has 'container-image-stream' that needs to be removed.
	// documents we think we're changing, and the rest should go through
	// unchanged.
	var rawSettings bson.M
	iter := settingsColl.Find(nil).Sort("_id").Iter()
	defer iter.Close()

	expectedSettings := []bson.M{}

	for iter.Next(&rawSettings) {
		expSettings := copyMap(rawSettings, nil)
		delete(expSettings, "txn-queue")
		delete(expSettings, "txn-revno")
		delete(expSettings, "version")
		id, ok := expSettings["_id"]
		c.Assert(ok, jc.IsTrue)
		idStr, ok := id.(string)
		c.Assert(ok, jc.IsTrue)
		c.Assert(idStr, gc.Not(gc.Equals), "")
		if idStr == "not-a-model" {
			raw, ok := expSettings["settings"]
			c.Assert(ok, jc.IsTrue)
			settings, ok := raw.(bson.M)
			c.Assert(ok, jc.IsTrue)
			delete(settings, "container-image-stream")
		}
		expectedSettings = append(expectedSettings, expSettings)
	}
	c.Assert(iter.Close(), jc.ErrorIsNil)

	// Note that the assertions on this are very hard to read for humans,
	// because Settings documents have a ton of keys and nested sub documents.
	// But it is a more accurate depiction of what is in that table.
	s.assertUpgradedData(c, RemoveContainerImageStreamFromNonModelSettings,
		upgradedData(settingsColl, expectedSettings),
	)
}

func (s *upgradesSuite) TestAddCloudModelCounts(c *gc.C) {
	modelsColl, closer := s.state.db().GetRawCollection(modelsC)
	defer closer()

	err := modelsColl.Insert(
		modelDoc{
			Type:           ModelTypeIAAS,
			UUID:           "0000-dead-beaf-0001",
			Owner:          "user-admin@local",
			Name:           "controller",
			ControllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			Cloud:          "cloud-foo",
		},
		modelDoc{
			Type:           ModelTypeIAAS,
			UUID:           "0000-dead-beaf-0002",
			Owner:          "user-mary@external",
			Name:           "default",
			ControllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			Cloud:          "cloud-foo",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	cloudsColl, closer := s.state.db().GetRawCollection(cloudsC)
	defer closer()

	err = cloudsColl.Insert(
		bson.M{
			"_id":        "cloud-foo",
			"name":       "cloud-foo",
			"type":       "dummy",
			"auth-types": []string{"empty"},
			"endpoint":   "here",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	refCountColl, closer := s.state.db().GetRawCollection(globalRefcountsC)
	defer closer()
	expected := []bson.M{{
		"_id":      "cloudModel#cloud-foo",
		"refcount": 2,
	}, {
		"_id":      "cloudModel#dummy",
		"refcount": 1, // unchanged
	}}
	s.assertUpgradedData(c, AddCloudModelCounts, upgradedData(refCountColl, expected))
}

func (s *upgradesSuite) TestMigrateStorageMachineIdFields(c *gc.C) {
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
		"hostid":     "666",
	}, bson.M{
		"_id":        uuid + ":2",
		"name":       "2",
		"model-uuid": uuid,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = volumeAttachmentsColl.Insert(bson.M{
		"_id":        uuid + ":123:0",
		"model-uuid": uuid,
		"machineid":  "123",
		"volumeid":   "0",
	}, bson.M{
		"_id":        uuid + ":123:1",
		"model-uuid": uuid,
		"hostid":     "123",
		"volumeid":   "1",
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
		"hostid":       "666",
	}, bson.M{
		"_id":          uuid + ":2",
		"filesystemid": "2",
		"model-uuid":   uuid,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = filesystemAttachmentsColl.Insert(bson.M{
		"_id":          uuid + ":123:3",
		"model-uuid":   uuid,
		"machineid":    "123",
		"filesystemid": "0",
	}, bson.M{
		"_id":          uuid + ":123:4",
		"model-uuid":   uuid,
		"hostid":       "123",
		"filesystemid": "1",
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedVolumes := []bson.M{{
		"_id":        uuid + ":0",
		"name":       "0",
		"model-uuid": uuid,
		"hostid":     "42",
	}, {
		"_id":        uuid + ":1",
		"name":       "1",
		"model-uuid": uuid,
		"hostid":     "666",
	}, {
		"_id":        uuid + ":2",
		"name":       "2",
		"model-uuid": uuid,
	}}
	expectedFilesystems := []bson.M{{
		"_id":          uuid + ":0",
		"filesystemid": "0",
		"model-uuid":   uuid,
		"hostid":       "42",
	}, {
		"_id":          uuid + ":1",
		"filesystemid": "1",
		"model-uuid":   uuid,
		"hostid":       "666",
	}, {
		"_id":          uuid + ":2",
		"filesystemid": "2",
		"model-uuid":   uuid,
	}}
	expectedVolumeAttachments := []bson.M{{
		"_id":        uuid + ":123:0",
		"model-uuid": uuid,
		"hostid":     "123",
		"volumeid":   "0",
	}, {
		"_id":        uuid + ":123:1",
		"model-uuid": uuid,
		"hostid":     "123",
		"volumeid":   "1",
	}}
	expectedFilesystemAttachments := []bson.M{{
		"_id":          uuid + ":123:3",
		"model-uuid":   uuid,
		"hostid":       "123",
		"filesystemid": "0",
	}, {
		"_id":          uuid + ":123:4",
		"model-uuid":   uuid,
		"hostid":       "123",
		"filesystemid": "1",
	}}

	s.assertUpgradedData(c, MigrateStorageMachineIdFields,
		upgradedData(volumesColl, expectedVolumes),
		upgradedData(filesystemsColl, expectedFilesystems),
		upgradedData(volumeAttachmentsColl, expectedVolumeAttachments),
		upgradedData(filesystemAttachmentsColl, expectedFilesystemAttachments),
	)
}

func (s *upgradesSuite) TestMigrateAddModelPermissions(c *gc.C) {
	permissionsColl, closer := s.state.db().GetRawCollection(permissionsC)
	defer closer()

	controllerKey := controllerKey(s.state.ControllerUUID())
	modelKey := modelKey(s.state.ModelUUID())
	err := permissionsColl.Insert(
		permissionDoc{
			ID:               permissionID(controllerKey, "us#bob"),
			SubjectGlobalKey: "us#bob",
			ObjectGlobalKey:  controllerKey,
			Access:           "add-model",
		},
		permissionDoc{
			ID:               permissionID("somemodel", "us#bob"),
			SubjectGlobalKey: "us#bob",
			ObjectGlobalKey:  "somemodel",
			Access:           "read",
		},
		permissionDoc{
			ID:               permissionID(controllerKey, "us#mary"),
			SubjectGlobalKey: "us#mary",
			ObjectGlobalKey:  controllerKey,
			Access:           "login",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	expected := docById{{
		"_id":                permissionID(controllerKey, "us#test-admin"),
		"object-global-key":  controllerKey,
		"subject-global-key": "us#test-admin",
		"access":             "superuser",
	}, {
		"_id":                permissionID(modelKey, "us#test-admin"),
		"object-global-key":  modelKey,
		"subject-global-key": "us#test-admin",
		"access":             "admin",
	}, {
		"_id":                permissionID("cloud#dummy", "us#test-admin"),
		"subject-global-key": "us#test-admin",
		"object-global-key":  "cloud#dummy",
		"access":             "admin",
	}, {
		"_id":                permissionID(controllerKey, "us#bob"),
		"subject-global-key": "us#bob",
		"object-global-key":  controllerKey,
		"access":             "login",
	}, {
		"_id":                permissionID("somemodel", "us#bob"),
		"subject-global-key": "us#bob",
		"object-global-key":  "somemodel",
		"access":             "read",
	}, {
		"_id":                permissionID("cloud#dummy", "us#bob"),
		"subject-global-key": "us#bob",
		"object-global-key":  "cloud#dummy",
		"access":             "add-model",
	}, {
		"_id":                permissionID(controllerKey, "us#mary"),
		"subject-global-key": "us#mary",
		"object-global-key":  controllerKey,
		"access":             "login",
	}}
	sort.Sort(expected)
	s.assertUpgradedData(c, MigrateAddModelPermissions, upgradedData(permissionsColl, expected))
}

func (s *upgradesSuite) TestSetEnableDiskUUIDOnVsphere(c *gc.C) {
	coll, closer := s.state.db().GetRawCollection(settingsC)
	defer closer()

	_, err := coll.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	m1 := s.makeModel(c, "m1", coretesting.Attrs{
		"type": "someprovider",
	})
	defer func() { _ = m1.Close() }()
	m2 := s.makeModel(c, "m2", coretesting.Attrs{
		"type": "vsphere",
	})
	defer func() { _ = m2.Close() }()
	m3 := s.makeModel(c, "m3", coretesting.Attrs{
		"type":             "vsphere",
		"enable-disk-uuid": true,
	})
	defer func() { _ = m3.Close() }()

	err = coll.Insert(bson.M{
		"_id": "someothersettingshouldnotbetouched",
		// non-model setting: should not be touched
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	getAttrs := func(st *State) map[string]interface{} {
		model, err := st.Model()
		c.Assert(err, jc.ErrorIsNil)
		cfg, err := model.ModelConfig()
		c.Assert(err, jc.ErrorIsNil)
		attrs := cfg.AllAttrs()
		attrs["resource-tags"] = ""
		return attrs
	}

	expected1 := getAttrs(m1)
	expected2 := getAttrs(m2)
	expected2["enable-disk-uuid"] = false

	expected3 := getAttrs(m3)

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
			"_id":        m3.ModelUUID() + ":e",
			"settings":   bson.M(expected3),
			"model-uuid": m3.ModelUUID(),
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}
	sort.Sort(expectedSettings)

	c.Logf(pretty.Sprint(expectedSettings))
	s.assertUpgradedData(c, SetEnableDiskUUIDOnVsphere,
		upgradedData(coll, expectedSettings),
	)
}

func (s *upgradesSuite) TestUpdateInheritedControllerConfig(c *gc.C) {
	coll, closer := s.state.db().GetRawCollection(globalSettingsC)
	defer closer()

	_, err := coll.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	err = coll.Insert(bson.M{
		"_id":      "controller",
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)
	expectedSettings := bsonMById{
		{
			"_id":        "cloud#dummy",
			"model-uuid": "",
			"settings":   bson.M{"key": "value"},
		},
	}

	c.Logf(pretty.Sprint(expectedSettings))
	s.assertUpgradedData(c, UpdateInheritedControllerConfig,
		upgradedData(coll, expectedSettings),
	)
}

type fakeBroker struct {
	caas.Broker
}

func (f *fakeBroker) GetClusterMetadata(storageClass string) (result *caas.ClusterMetadata, err error) {
	return &caas.ClusterMetadata{
		NominatedStorageClass: &caas.StorageProvisioner{
			Name: "storage-provisioner",
		},
	}, nil
}

func (s *upgradesSuite) makeCaasModel(c *gc.C, name string, cred names.CloudCredentialTag, attr coretesting.Attrs) *State {
	uuid := utils.MustNewUUID()
	cfg := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"name": name,
		"uuid": uuid.String(),
	}.Merge(attr))
	m, err := s.state.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, st, err := s.controller.NewModel(ModelArgs{
		Type:                    ModelTypeCAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		CloudCredential:         cred,
		Config:                  cfg,
		Owner:                   m.Owner(),
		StorageProviderRegistry: provider.CommonStorageProviders(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return st
}

func (s *upgradesSuite) TestUpdateKubernetesStorageConfig(c *gc.C) {
	tag := names.NewCloudCredentialTag(fmt.Sprintf("dummy/%s/default", s.owner.Id()))
	err := s.state.UpdateCloudCredential(tag, cloud.NewEmptyCredential())
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(&NewBroker, func(args environs.OpenParams) (caas.Broker, error) {
		return &fakeBroker{}, nil
	})

	m1 := s.makeCaasModel(c, "m1", tag, coretesting.Attrs{
		"type": "kubernetes",
	})
	defer m1.Close()

	settingsColl, settingsCloser := m1.database.GetRawCollection(settingsC)
	defer settingsCloser()

	// Two rounds to check idempotency.
	for i := 0; i < 2; i++ {
		c.Logf("Run: %d", i)
		err := UpdateKubernetesStorageConfig(s.pool)
		c.Assert(err, jc.ErrorIsNil)

		var docs []bson.M
		err = settingsColl.FindId(m1.ModelUUID() + ":e").All(&docs)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(docs, gc.HasLen, 1)
		settings, ok := docs[0]["settings"].(bson.M)
		c.Assert(ok, jc.IsTrue)
		c.Assert(settings["operator-storage"], gc.Equals, "storage-provisioner")
		c.Assert(settings["workload-storage"], gc.Equals, "storage-provisioner")
	}
}

func (s *upgradesSuite) TestUpdateKubernetesStorageConfigWithDyingModel(c *gc.C) {
	tag := names.NewCloudCredentialTag(fmt.Sprintf("dummy/%s/default", s.owner.Id()))
	err := s.state.UpdateCloudCredential(tag, cloud.NewEmptyCredential())
	c.Assert(err, jc.ErrorIsNil)

	s.PatchValue(&NewBroker, func(args environs.OpenParams) (caas.Broker, error) {
		return &fakeBroker{}, nil
	})

	m1 := s.makeCaasModel(c, "m1", tag, coretesting.Attrs{
		"type": "kubernetes",
	})
	defer m1.Close()
	model, err := m1.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = model.Destroy(DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)

	settingsColl, settingsCloser := m1.database.GetRawCollection(settingsC)
	defer settingsCloser()

	// Doesn't fail...
	err = UpdateKubernetesStorageConfig(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	// ...makes no changes to settings.
	var docs []bson.M
	err = settingsColl.FindId(m1.ModelUUID() + ":e").All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 1)
	settings, ok := docs[0]["settings"].(bson.M)
	c.Assert(ok, jc.IsTrue)
	c.Assert(settings["operator-storage"], gc.Equals, nil)
	c.Assert(settings["workload-storage"], gc.Equals, nil)
}

func (s *upgradesSuite) TestEnsureDefaultModificationStatus(c *gc.C) {
	coll, closer := s.state.db().GetRawCollection(statusesC)
	defer closer()

	model1 := s.makeModel(c, "model-old", coretesting.Attrs{})
	defer model1.Close()
	model2 := s.makeModel(c, "model-new", coretesting.Attrs{})
	defer model2.Close()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	s.makeMachine(c, uuid1, "0", Alive)
	s.makeMachine(c, uuid2, "1", Dying)

	expected := bsonMById{
		{
			"_id":        uuid1 + ":m#0#modification",
			"model-uuid": uuid1,
			"status":     "idle",
			"statusinfo": "",
			"statusdata": bson.M{},
			"updated":    int64(1),
		}, {
			"_id":        uuid2 + ":m#1#modification",
			"model-uuid": uuid2,
			"status":     "idle",
			"statusinfo": "",
			"statusdata": bson.M{},
			"updated":    int64(1),
		},
	}

	sort.Sort(expected)
	c.Log(pretty.Sprint(expected))
	s.assertUpgradedData(c, EnsureDefaultModificationStatus,
		upgradedDataWithFilter(coll, expected, bson.D{{"_id", bson.RegEx{"#modification$", ""}}}),
	)
}

func (s *upgradesSuite) TestEnsureApplicationDeviceConstraints(c *gc.C) {
	coll, closer := s.state.db().GetRawCollection(deviceConstraintsC)
	defer closer()

	model1 := s.makeModel(c, "model-old", coretesting.Attrs{})
	defer model1.Close()
	model2 := s.makeModel(c, "model-new", coretesting.Attrs{})
	defer model2.Close()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	s.makeApplication(c, uuid1, "app1", Alive)
	s.makeApplication(c, uuid2, "app2", Dying)

	expected := bsonMById{
		{
			"_id":         uuid1 + ":adc#app1#cs:test-charm",
			"constraints": bson.M{},
		}, {
			"_id":         uuid2 + ":adc#app2#cs:test-charm",
			"constraints": bson.M{},
		},
	}

	sort.Sort(expected)
	c.Log(pretty.Sprint(expected))
	s.assertUpgradedData(c, EnsureApplicationDeviceConstraints,
		upgradedDataWithFilter(coll, expected, bson.D{{"_id", bson.RegEx{Pattern: ":adc#"}}}),
	)
}

// makeApplication doesn't do what you think it does here. You can read the
// applicationDoc, but you can't update it using the txn.Op. It will report that
// the transaction failed because the `Assert: txn.DocExists` is wrong, even
// though we got the application from the database.
// We should move the Insert into a bson.M/bson.D
func (s *upgradesSuite) makeApplication(c *gc.C, uuid, name string, life Life) {
	coll, closer := s.state.db().GetRawCollection(applicationsC)
	defer closer()

	curl := charm.MustParseURL("test-charm")
	err := coll.Insert(applicationDoc{
		DocID:     ensureModelUUID(uuid, name),
		Name:      name,
		ModelUUID: uuid,
		CharmURL:  curl,
		Life:      life,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestRemoveInstanceCharmProfileDataCollection(c *gc.C) {
	db := s.state.MongoSession().DB(jujuDB)
	db.C("instanceCharmProfileData")
	err := RemoveInstanceCharmProfileDataCollection(s.pool)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestRemoveInstanceCharmProfileDataCollectionNoCollection(c *gc.C) {
	db := s.state.MongoSession().DB(jujuDB)
	cols, err := db.CollectionNames()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(set.NewStrings(cols...).Contains("instanceCharmProfileData"), jc.IsFalse)

	err = RemoveInstanceCharmProfileDataCollection(s.pool)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestUpdateK8sModelNameIndex(c *gc.C) {
	modelsColl, closer := s.state.db().GetRawCollection(modelsC)
	defer closer()
	err := modelsColl.Insert(bson.M{
		"_id":   utils.MustNewUUID().String(),
		"type":  "iaas",
		"name":  "model1",
		"owner": "fred",
		"cloud": "lxd",
	}, bson.M{
		"_id":   utils.MustNewUUID().String(),
		"type":  "caas",
		"name":  "model2",
		"owner": "mary",
		"cloud": "microk8s",
	}, bson.M{
		"_id":   utils.MustNewUUID().String(),
		"type":  "caas",
		"name":  "model3",
		"owner": "jane",
		"cloud": "microk8s",
	})
	c.Assert(err, jc.ErrorIsNil)

	modelNameColl, closer := s.state.db().GetRawCollection(usermodelnameC)
	defer closer()

	err = modelNameColl.Insert(bson.M{
		"_id": "fred:model1",
	}, bson.M{
		"_id": "mary:model2",
	}, bson.M{
		"_id": "microk8s:model3",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id": "fred:model1",
		}, {
			"_id": "mary:model2",
		}, {
			"_id": "jane:model3",
		}, {
			"_id": "test-admin:testmodel",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, UpdateK8sModelNameIndex,
		upgradedData(modelNameColl, expected),
	)
}

func (s *upgradesSuite) TestAddModelLogsSize(c *gc.C) {
	settingsColl, settingsCloser := s.state.db().GetRawCollection(controllersC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		"_id": "controllerSettings",
		"settings": bson.M{
			"key": "value",
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
				"key":             "value",
				"model-logs-size": "20M",
			},
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}

	s.assertUpgradedData(c, AddModelLogsSize, upgradedData(settingsColl, expectedSettings))
}

func (s *upgradesSuite) TestAddControllerNodeDocs(c *gc.C) {
	machinesColl, closer := s.state.db().GetRawCollection(machinesC)
	defer closer()
	controllerNodesColl, closer2 := s.state.db().GetRawCollection(controllerNodesC)
	defer closer2()
	controllersColl, closer3 := s.state.db().GetRawCollection(controllersC)
	defer closer3()

	// Will will never have different UUIDs in practice but testing
	// with that scenario avoids any potential bad code assumptions.
	uuid1 := "uuid1"
	uuid2 := "uuid2"
	err := machinesColl.Insert(bson.M{
		"_id":       ensureModelUUID(uuid1, "1"),
		"machineid": "1",
		"novote":    false,
		"hasvote":   true,
		"jobs":      []MachineJob{JobManageModel},
	}, bson.M{
		"_id":       ensureModelUUID(uuid1, "2"),
		"machineid": "2",
		"novote":    false,
		"hasvote":   false,
		"jobs":      []MachineJob{JobManageModel},
	}, bson.M{
		"_id":       ensureModelUUID(uuid1, "3"),
		"machineid": "3",
		"novote":    true,
		"hasvote":   false,
		"jobs":      []MachineJob{JobManageModel},
	}, bson.M{
		"_id":       ensureModelUUID(uuid1, "4"),
		"machineid": "3",
		"jobs":      []MachineJob{JobHostUnits},
	}, bson.M{
		"_id":       ensureModelUUID(uuid1, "5"),
		"machineid": "5",
		"jobs":      []MachineJob{JobManageModel},
	}, bson.M{
		"_id":       ensureModelUUID(uuid2, "1"),
		"machineid": "1",
		"novote":    false,
		"hasvote":   false,
		"jobs":      []MachineJob{JobManageModel},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = controllersColl.Update(
		bson.D{{"_id", modelGlobalKey}},
		bson.D{{"$set", bson.D{{"machineids", []string{"0", "1"}}}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":           uuid1 + ":1",
			"has-vote":      true,
			"wants-vote":    true,
			"password-hash": "",
		}, {
			"_id":           uuid1 + ":2",
			"has-vote":      false,
			"wants-vote":    true,
			"password-hash": "",
		}, {
			"_id":           uuid1 + ":3",
			"has-vote":      false,
			"wants-vote":    false,
			"password-hash": "",
		}, {
			"_id":           uuid2 + ":1",
			"has-vote":      false,
			"wants-vote":    true,
			"password-hash": "",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, AddControllerNodeDocs,
		upgradedData(controllerNodesColl, expected),
	)

	// Ensure obsolete machine doc fields are gone.
	var mdocs bsonMById
	err = machinesColl.Find(nil).All(&mdocs)
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(mdocs)
	for _, d := range mdocs {
		delete(d, "txn-queue")
		delete(d, "txn-revno")
	}
	c.Assert(mdocs, jc.DeepEquals, bsonMById{{
		"_id":       ensureModelUUID(uuid1, "1"),
		"machineid": "1",
		"jobs":      []interface{}{2},
	}, bson.M{
		"_id":       ensureModelUUID(uuid1, "2"),
		"machineid": "2",
		"jobs":      []interface{}{2},
	}, bson.M{
		"_id":       ensureModelUUID(uuid1, "3"),
		"machineid": "3",
		"jobs":      []interface{}{2},
	}, bson.M{
		"_id":       ensureModelUUID(uuid1, "4"),
		"machineid": "3",
		"jobs":      []interface{}{1},
	}, bson.M{
		"_id":       ensureModelUUID(uuid1, "5"),
		"machineid": "5",
		"jobs":      []interface{}{2},
	}, bson.M{
		"_id":       ensureModelUUID(uuid2, "1"),
		"machineid": "1",
		"jobs":      []interface{}{2},
	}})

	// Check machineids has been renamed to controller-ids.
	var cdocs []bson.M
	err = controllersColl.FindId(modelGlobalKey).All(&cdocs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cdocs, gc.HasLen, 1)
	cdoc := cdocs[0]
	delete(cdoc, "txn-queue")
	delete(cdoc, "txn-revno")
	c.Assert(cdoc, jc.DeepEquals, bson.M{
		"_id":            modelGlobalKey,
		"model-uuid":     s.state.modelTag.Id(),
		"cloud":          "dummy",
		"controller-ids": []interface{}{"0", "1"},
	})
}

func (s *upgradesSuite) TestAddSpaceIdToSpaceDocs(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(spacesC)
	defer closer()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()
	uuidc := s.state.controllerModelTag.Id()

	err := col.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "space1"),
		"model-uuid": uuid1,
		"life":       Alive,
		"name":       "space1",
		"is-public":  true,
		"providerid": "provider1",
	}, bson.M{
		"_id":        ensureModelUUID(uuid2, "space2"),
		"model-uuid": uuid2,
		"life":       Alive,
		"name":       "space2",
		"is-public":  false,
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		// The altered spaces:
		{
			"_id":        uuid1 + ":1",
			"model-uuid": uuid1,
			"spaceid":    "1",
			"life":       0,
			"name":       "space1",
			"is-public":  true,
			"providerid": "provider1",
		}, {
			"_id":        uuid2 + ":1",
			"model-uuid": uuid2,
			"spaceid":    "1",
			"life":       0,
			"name":       "space2",
			"is-public":  false,
		},
		// The default space for each model, including the controller.
		{
			"_id":        uuid1 + ":0",
			"model-uuid": uuid1,
			"spaceid":    "0",
			"life":       0,
			"name":       network.AlphaSpaceName,
			"is-public":  true,
		}, {
			"_id":        uuid2 + ":0",
			"model-uuid": uuid2,
			"spaceid":    "0",
			"life":       0,
			"name":       network.AlphaSpaceName,
			"is-public":  true,
		}, {
			"_id":        uuidc + ":0",
			"model-uuid": uuidc,
			"spaceid":    "0",
			"life":       0,
			"name":       network.AlphaSpaceName,
			"is-public":  true,
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, AddSpaceIdToSpaceDocs, upgradedData(col, expected))
}

func (s *upgradesSuite) TestEnsureRelationApplicationSettings(c *gc.C) {
	settingsCol, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()

	relationsCol, relationsCloser := s.state.db().GetRawCollection(relationsC)
	defer relationsCloser()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	defer func() { _ = model1.Close() }()

	uuid1 := model1.ModelUUID()

	err := relationsCol.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "mariadb:cluster"),
		"key":        "mariadb:cluster",
		"model-uuid": uuid1,
		"id":         0,
		"endpoints": []bson.M{{
			"applicationname": "mariadb",
			"relation": bson.M{
				"name":      "cluster",
				"role":      "peer",
				"interface": "mysql-ha",
				"optional":  false,
				"limit":     1,
				"scope":     "global",
			},
		}},
		"life":             0,
		"unitcount":        1,
		"suspended":        false,
		"suspended-reason": "",
	}, bson.M{
		"_id":        ensureModelUUID(uuid1, "mediawiki:db mariadb:db"),
		"key":        "mediawiki:db mariadb:db",
		"model-uuid": uuid1,
		"id":         1,
		"endpoints": []bson.M{{
			"applicationname": "mediawiki",
			"relation": bson.M{
				"name":      "db",
				"role":      "requirer",
				"interface": "mysql",
				"optional":  false,
				"limit":     1,
				"scope":     "global",
			},
		}, {
			"applicationname": "mariadb",
			"relation": bson.M{
				"name":      "db",
				"role":      "provider",
				"interface": "mysql",
				"optional":  false,
				"limit":     0,
				"scope":     "",
			},
		}},
		"life":             0,
		"unitcount":        3,
		"suspended":        false,
		"suspended-reason": "",
	}, bson.M{
		"_id":        ensureModelUUID(uuid1, "nrpe:general-info mediawiki:juju-info"),
		"key":        "nrpe:general-info mediawiki:juju-info",
		"model-uuid": uuid1,
		"id":         2,
		"endpoints": []bson.M{{
			"applicationname": "mediawiki",
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
		"life":             1,
		"unitcount":        4,
		"suspended":        false,
		"suspended-reason": "",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = settingsCol.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "r#1#mariadb"),
		"model-uuid": uuid1,
		"settings": bson.M{
			"olden-yolk": "blue paradigm",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{{
		"_id":        ensureModelUUID(uuid1, "r#0#mariadb"),
		"model-uuid": uuid1,
		"settings":   bson.M{},
	}, {
		"_id":        ensureModelUUID(uuid1, "r#1#mariadb"),
		"model-uuid": uuid1,
		"settings": bson.M{
			"olden-yolk": "blue paradigm",
		},
	}, {
		"_id":        ensureModelUUID(uuid1, "r#1#mediawiki"),
		"model-uuid": uuid1,
		"settings":   bson.M{},
	}, {
		"_id":        ensureModelUUID(uuid1, "r#2#mediawiki"),
		"model-uuid": uuid1,
		"settings":   bson.M{},
	}, {
		"_id":        ensureModelUUID(uuid1, "r#2#nrpe"),
		"model-uuid": uuid1,
		"settings":   bson.M{},
	}}

	s.assertUpgradedData(c, EnsureRelationApplicationSettings,
		upgradedDataWithFilter(settingsCol, expected, bson.D{{"_id", bson.RegEx{`:r#\d+#.*$`, ""}}}),
	)
}

func (s *upgradesSuite) TestChangeSubnetAZtoSlice(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(subnetsC)
	defer closer()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	err := col.Insert(bson.M{
		"_id":              ensureModelUUID(uuid1, "0"),
		"model-uuid":       uuid1,
		"providerid":       "provider1",
		"availabilityzone": "testme",
	}, bson.M{
		"_id":        ensureModelUUID(uuid2, "0"),
		"model-uuid": uuid2,
		"is-public":  true,
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		// The altered subnets:
		{
			"_id":                uuid1 + ":0",
			"model-uuid":         uuid1,
			"providerid":         "provider1",
			"availability-zones": []interface{}{"testme"},
		}, {
			"_id":        uuid2 + ":0",
			"model-uuid": uuid2,
			"is-public":  true,
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, ChangeSubnetAZtoSlice, upgradedData(col, expected))
}

func (s *upgradesSuite) TestChangeSubnetSpaceNameToSpaceID(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(subnetsC)
	defer closer()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	_, err := model1.AddSpace("testme", "42", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	err = col.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "0"),
		"model-uuid": uuid1,
		"space-name": "testme",
	}, bson.M{
		"_id":        ensureModelUUID(uuid1, "1"),
		"model-uuid": uuid1,
	}, bson.M{
		"_id":        ensureModelUUID(uuid2, "0"),
		"model-uuid": uuid2,
		"space-id":   "6",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		// The altered subnets:
		{
			"_id":        uuid1 + ":0",
			"model-uuid": uuid1,
			"space-id":   "1",
		}, {
			"_id":        uuid1 + ":1",
			"model-uuid": uuid1,
			"space-id":   "0",
		}, {
			"_id":        uuid2 + ":0",
			"model-uuid": uuid2,
			"space-id":   "6",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, ChangeSubnetSpaceNameToSpaceID, upgradedData(col, expected))
}

func (s *upgradesSuite) TestAddSubnetIdToSubnetDocs(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(subnetsC)
	defer closer()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()
	cidr1 := "10.0.0.0/16"
	cidr2 := "10.0.42.0/16"

	err := col.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, cidr1),
		"model-uuid": uuid1,
		"life":       Alive,
		"providerid": "provider1",
		"cidr":       cidr1,
	}, bson.M{
		"_id":        ensureModelUUID(uuid2, cidr2),
		"model-uuid": uuid2,
		"life":       Alive,
		"is-public":  false,
		"cidr":       cidr2,
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		// The altered subnets:
		{
			"_id":        uuid1 + ":0",
			"model-uuid": uuid1,
			"subnet-id":  "0",
			"life":       0,
			"providerid": "provider1",
			"cidr":       cidr1,
		}, {
			"_id":        uuid2 + ":0",
			"model-uuid": uuid2,
			"subnet-id":  "0",
			"life":       0,
			"cidr":       cidr2,
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, AddSubnetIdToSubnetDocs, upgradedData(col, expected))
}

func (s *upgradesSuite) TestReplacePortsDocSubnetIDCIDR(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(openedPortsC)
	defer closer()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	subnet2, err := model2.AddSubnet(network.SubnetInfo{CIDR: "10.0.0.0/16"})
	c.Assert(err, jc.ErrorIsNil)

	err = col.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "m#3#42"),
		"model-uuid": uuid1,
		"machine-id": "3",
		"subnet-id":  "42",
	}, bson.M{
		"_id":        ensureModelUUID(uuid1, "m#4#"),
		"model-uuid": uuid1,
		"machine-id": "4",
	}, bson.M{
		"_id":        ensureModelUUID(uuid2, "m#4#"+subnet2.CIDR()),
		"model-uuid": uuid2,
		"machine-id": "4",
		"subnet-id":  subnet2.CIDR(),
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		// The altered portDocs:
		{
			"_id":        uuid1 + ":m#3#42",
			"model-uuid": uuid1,
			"machine-id": "3",
			"subnet-id":  "42",
		}, {
			"_id":        uuid1 + ":m#4#",
			"model-uuid": uuid1,
			"machine-id": "4",
		}, {
			"_id":        uuid2 + ":m#4#" + subnet2.ID(),
			"model-uuid": uuid2,
			"machine-id": "4",
			"subnet-id":  subnet2.ID(),
			"ports":      []interface{}{},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, ReplacePortsDocSubnetIDCIDR, upgradedData(col, expected))
}

func (s *upgradesSuite) TestConvertAddressSpaceIDs(c *gc.C) {
	mod := s.makeModel(c, "the-model", coretesting.Attrs{})
	defer func() { _ = mod.Close() }()

	uuid := mod.modelUUID()
	s.makeMachine(c, uuid, "0", Alive)
	s.makeMachine(c, uuid, "1", Alive)

	type oldAddress struct {
		Value       string `bson:"value"`
		AddressType string `bson:"addresstype"`
		Scope       string `bson:"networkscope,omitempty"`
		Origin      string `bson:"origin,omitempty"`
		SpaceName   string `bson:"spacename,omitempty"`
		SpaceID     string `bson:"spaceid,omitempty"`
	}

	m1Addrs := []oldAddress{
		{
			Value:     "1.1.1.1",
			SpaceName: "space1",
			SpaceID:   "provider1",
		},
		{
			Value:   "2.2.2.2",
			SpaceID: "no-change",
		},
	}
	m1MachineAddrs := []oldAddress{
		{
			Value:     "3.3.3.3",
			SpaceName: "space3",
			// This is an invalid form, but should still be
			// correctly matched to the ID for provider3.
			SpaceID: "pRoViDeR3",
		},
	}
	m2Private := oldAddress{
		Value:     "4.4.4.4",
		SpaceName: "space1",
		SpaceID:   "provider1",
	}
	m2Public := oldAddress{
		Value: "5.5.5.5",
	}

	// Update the various machine addresses and add some CAAS documents.
	ops := []txn.Op{
		{
			C:  machinesC,
			Id: ensureModelUUID(uuid, "0"),
			Update: bson.D{
				{"$set", bson.D{{"addresses", m1Addrs}}},
				{"$set", bson.D{{"machineaddresses", m1MachineAddrs}}},
			},
		},
		{
			C:  machinesC,
			Id: ensureModelUUID(uuid, "1"),
			Update: bson.D{
				{"$set", bson.D{{"preferredprivateaddress", m2Private}}},
				{"$set", bson.D{{"preferredpublicaddress", m2Public}}},
			},
		},
		{
			C:  cloudContainersC,
			Id: ensureModelUUID(uuid, "0"),
			Insert: bson.M{
				"model-uuid":  uuid,
				"address":     &m2Public,
				"provider-id": "c0",
			},
		},
		{
			C:  cloudContainersC,
			Id: ensureModelUUID(uuid, "1"),
			Insert: bson.M{
				"model-uuid":  uuid,
				"address":     nil,
				"provider-id": "c1",
			},
		},
		{
			C:  cloudServicesC,
			Id: ensureModelUUID(uuid, "0"),
			Insert: bson.M{
				"model-uuid":  uuid,
				"addresses":   []oldAddress{{Value: "6.6.6.6"}, {Value: "7.7.7.7"}},
				"provider-id": "s0",
			},
		},
	}
	c.Assert(s.state.db().RunRawTransaction(ops), jc.ErrorIsNil)

	// Add spaces for our lookup.
	s.makeSpace(c, uuid, "space1", "1")
	s.makeSpace(c, uuid, "space2", "2")
	s.makeSpace(c, uuid, "space3", "3")

	expMachines := bsonMById{
		{
			"_id":                      ensureModelUUID(uuid, "0"),
			"model-uuid":               uuid,
			"machineid":                "0",
			"nonce":                    "",
			"passwordhash":             "",
			"clean":                    false,
			"life":                     0,
			"force-destroyed":          false,
			"series":                   "",
			"jobs":                     []interface{}{},
			"supportedcontainersknown": false,
			"containertype":            "",
			"principals":               []interface{}{},
			"addresses": []interface{}{
				bson.M{
					"value":       "1.1.1.1",
					"spaceid":     "1",
					"addresstype": "",
				},
				bson.M{
					"value":       "2.2.2.2",
					"spaceid":     "no-change",
					"addresstype": "",
				},
			},
			"machineaddresses": []interface{}{
				bson.M{
					"value":       "3.3.3.3",
					"spaceid":     "3",
					"addresstype": "",
				},
			},
			// These were unset and end up with the zero-types.
			"preferredpublicaddress": bson.M{
				"value":       "",
				"addresstype": "",
			},
			"preferredprivateaddress": bson.M{
				"value":       "",
				"addresstype": "",
			},
		}, {
			"_id":                      ensureModelUUID(uuid, "1"),
			"model-uuid":               uuid,
			"machineid":                "1",
			"nonce":                    "",
			"passwordhash":             "",
			"clean":                    false,
			"life":                     0,
			"force-destroyed":          false,
			"series":                   "",
			"jobs":                     []interface{}{},
			"supportedcontainersknown": false,
			"containertype":            "",
			"principals":               []interface{}{},
			"addresses":                []interface{}{},
			"machineaddresses":         []interface{}{},
			"preferredprivateaddress": bson.M{
				"value":       "4.4.4.4",
				"spaceid":     "1",
				"addresstype": "",
			},
			// Address without space ends up in the default (empty) space.
			"preferredpublicaddress": bson.M{
				"value":       "5.5.5.5",
				"spaceid":     "0",
				"addresstype": "",
			},
		},
	}

	expServices := bsonMById{{
		"_id":         ensureModelUUID(uuid, "0"),
		"model-uuid":  uuid,
		"provider-id": "s0",
		"addresses": []interface{}{
			bson.M{
				"value":       "6.6.6.6",
				"spaceid":     "0",
				"addresstype": "",
			},
			bson.M{
				"value":       "7.7.7.7",
				"spaceid":     "0",
				"addresstype": "",
			},
		},
	}}

	expContainers := bsonMById{
		{
			"_id":         ensureModelUUID(uuid, "0"),
			"model-uuid":  uuid,
			"provider-id": "c0",
			"address": bson.M{
				"value":       "5.5.5.5",
				"spaceid":     "0",
				"addresstype": "",
			},
		},
		{
			"_id":         ensureModelUUID(uuid, "1"),
			"provider-id": "c1",
			"model-uuid":  uuid,
			"address":     interface{}(nil),
		},
	}

	machines, mCloser := s.state.db().GetRawCollection(machinesC)
	defer mCloser()
	services, sCloser := s.state.db().GetRawCollection(cloudServicesC)
	defer sCloser()
	containers, cCloser := s.state.db().GetRawCollection(cloudContainersC)
	defer cCloser()

	s.assertUpgradedData(c, ConvertAddressSpaceIDs,
		upgradedData(machines, expMachines),
		upgradedData(services, expServices),
		upgradedData(containers, expContainers),
	)
}

func (s *upgradesSuite) TestReplaceSpaceNameWithIDEndpointBindings(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(endpointBindingsC)
	defer closer()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	space1, err := model1.AddSpace("testspace", "testspace-43253", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	space2, err := model2.AddSpace("testspace2", "testspace-43253567", nil, false)
	c.Assert(err, jc.ErrorIsNil)

	err = col.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "a#ubuntu"),
		"model-uuid": uuid1,
		"bindings": bson.M{
			"one": space1.Name(),
			"two": network.AlphaSpaceName,
		},
	}, bson.M{
		"_id":        ensureModelUUID(uuid1, "a#ghost"),
		"model-uuid": uuid1,
		"bindings": bindingsMap{
			"one": space1.Name(),
			"":    space1.Name(),
		},
	}, bson.M{
		"_id":        ensureModelUUID(uuid2, "a#ubuntu"),
		"model-uuid": uuid2,
		"bindings": bindingsMap{
			"one": space2.Id(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		// The altered endpointBindings:
		{
			"_id":        uuid1 + ":a#ubuntu",
			"model-uuid": uuid1,
			"bindings":   bson.M{"one": space1.Id(), "two": network.AlphaSpaceId, "": network.AlphaSpaceId},
		}, {
			"_id":        uuid1 + ":a#ghost",
			"model-uuid": uuid1,
			"bindings":   bson.M{"one": space1.Id(), "": space1.Id()},
		}, {
			"_id":        uuid2 + ":a#ubuntu",
			"model-uuid": uuid2,
			"bindings":   bson.M{"one": space2.Id(), "": network.AlphaSpaceId},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, ReplaceSpaceNameWithIDEndpointBindings, upgradedData(col, expected))
}

func (s *upgradesSuite) TestEnsureDefaultSpaceSetting(c *gc.C) {
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)

	// Should not be changed because we already have a value.
	m1 := s.makeModel(c, "m1", coretesting.Attrs{
		config.DefaultSpace: "something",
	})
	defer func() { _ = m1.Close() }()

	// Should be set to "" because we do not have a value yet.
	m2 := s.makeModel(c, "m2", coretesting.Attrs{})
	defer func() { _ = m2.Close() }()

	m3 := s.makeModel(c, "m3", coretesting.Attrs{})
	defer func() { _ = m3.Close() }()

	err = settingsColl.Insert(bson.M{
		"_id":      "someothersettingshouldnotbetouched",
		"settings": bson.M{"key": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)

	getCfg := func(st *State) map[string]interface{} {
		m, err := st.Model()
		c.Assert(err, jc.ErrorIsNil)
		cfg, err := m.ModelConfig()
		c.Assert(err, jc.ErrorIsNil)
		exp := cfg.AllAttrs()
		exp["resource-tags"] = ""
		return exp
	}

	exp1 := getCfg(m1)

	exp2 := getCfg(m2)
	exp2[config.DefaultSpace] = ""

	exp3 := getCfg(m3)

	// Should be set to "" because it has the old default value "_default".
	// "_default" will no longer pass the config validation for DefaultSpace,
	// so add the hard way.
	exp3[config.DefaultSpace] = "_default"
	err = settingsColl.Update(
		bson.D{{"_id", m3.ModelUUID() + ":e"}},
		bson.D{{"$set", bson.D{{"settings", exp3}}}},
	)
	c.Assert(err, jc.ErrorIsNil)

	exp3[config.DefaultSpace] = ""

	expectedSettings := bsonMById{
		{
			"_id":        m1.ModelUUID() + ":e",
			"settings":   bson.M(exp1),
			"model-uuid": m1.ModelUUID(),
		}, {
			"_id":        m2.ModelUUID() + ":e",
			"settings":   bson.M(exp2),
			"model-uuid": m2.ModelUUID(),
		}, {
			"_id":        m3.ModelUUID() + ":e",
			"settings":   bson.M(exp3),
			"model-uuid": m3.ModelUUID(),
		}, {
			"_id":      "someothersettingshouldnotbetouched",
			"settings": bson.M{"key": "value"},
		},
	}
	sort.Sort(expectedSettings)

	s.assertUpgradedData(c, EnsureDefaultSpaceSetting, upgradedData(settingsColl, expectedSettings))
}

func (s *upgradesSuite) TestRemoveControllerConfigMaxLogAgeAndSize(c *gc.C) {
	settingsColl, settingsCloser := s.state.db().GetRawCollection(controllersC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		"_id": "controllerSettings",
		"settings": bson.M{
			"key":           "value",
			"max-logs-age":  "72h",
			"max-logs-size": "4096M",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedSettings := []bson.M{
		{
			"_id": "controllerSettings",
			"settings": bson.M{
				"key": "value",
			},
		},
	}
	s.assertUpgradedData(c, RemoveControllerConfigMaxLogAgeAndSize, upgradedData(settingsColl, expectedSettings))
}

func (s *upgradesSuite) TestIncrementTaskSequence(c *gc.C) {
	st := s.pool.SystemState()
	st1 := s.newState(c)
	st2 := s.newState(c)
	sequenceColl, closer := st.db().GetRawCollection(sequenceC)
	defer closer()

	// No tasks sequence requests, so no update.
	err := IncrementTasksSequence(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	for _, s := range []*State{st, st1, st2} {
		n, err := sequenceColl.FindId(s.ModelUUID() + ":tasks").Count()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(n, gc.Equals, 0)
	}

	_, err = sequence(st1, "tasks")
	c.Assert(err, jc.ErrorIsNil)
	err = IncrementTasksSequence(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	for i, s := range []*State{st, st1, st2} {
		var data bson.M
		err = sequenceColl.FindId(s.ModelUUID() + ":tasks").One(&data)
		if i != 1 {
			c.Assert(err, gc.Equals, mgo.ErrNotFound)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		counter, ok := data["counter"].(int)
		c.Assert(ok, jc.IsTrue)
		c.Assert(counter, gc.Equals, 2)
	}
}

func (s *upgradesSuite) makeMachine(c *gc.C, uuid, id string, life Life) {
	col, closer := s.state.db().GetRawCollection(machinesC)
	defer closer()

	err := col.Insert(machineDoc{
		DocID:     ensureModelUUID(uuid, id),
		Id:        id,
		ModelUUID: uuid,
		Life:      life,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) makeSpace(c *gc.C, uuid, name, id string) {
	coll, closer := s.state.db().GetRawCollection(spacesC)
	defer closer()

	err := coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid, id),
		"model-uuid": uuid,
		"spaceid":    id,
		"name":       name,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TestAddMachineIDToSubordinates(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(unitsC)
	defer closer()

	uuid1 := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()
	uuid3 := utils.MustNewUUID().String()

	err := col.Insert(bson.M{
		"_id":        uuid1 + ":principal/1",
		"model-uuid": uuid1,
		"machineid":  "1",
	}, bson.M{
		"_id":        uuid1 + ":telegraf/1",
		"model-uuid": uuid1,
		"principal":  "principal/1",
	}, bson.M{
		"_id":        uuid2 + ":another/0",
		"model-uuid": uuid2,
		"machineid":  "42",
	}, bson.M{
		"_id":        uuid2 + ":telegraf/0",
		"model-uuid": uuid2,
		"principal":  "another/0",
	}, bson.M{
		"_id":        uuid2 + ":livepatch/0",
		"model-uuid": uuid2,
		"principal":  "another/0",
	}, bson.M{
		// uuid3 is our CAAS model that doesn't have machine IDs for the princpals.
		"_id":        uuid3 + ":base/0",
		"model-uuid": uuid3,
	}, bson.M{
		"_id":        uuid3 + ":subordinate/0",
		"model-uuid": uuid3,
		"principal":  "base/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        uuid1 + ":principal/1",
			"model-uuid": uuid1,
			"machineid":  "1",
		}, {
			"_id":        uuid1 + ":telegraf/1",
			"model-uuid": uuid1,
			"principal":  "principal/1",
			"machineid":  "1",
		}, {
			"_id":        uuid2 + ":another/0",
			"model-uuid": uuid2,
			"machineid":  "42",
		}, {
			"_id":        uuid2 + ":telegraf/0",
			"model-uuid": uuid2,
			"principal":  "another/0",
			"machineid":  "42",
		}, {
			"_id":        uuid2 + ":livepatch/0",
			"model-uuid": uuid2,
			"principal":  "another/0",
			"machineid":  "42",
		}, {
			// uuid3 is our CAAS model that doesn't have machine IDs for the princpals.
			"_id":        uuid3 + ":base/0",
			"model-uuid": uuid3,
		}, {
			"_id":        uuid3 + ":subordinate/0",
			"model-uuid": uuid3,
			"principal":  "base/0",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, AddMachineIDToSubordinates, upgradedData(col, expected))
}

func (s *upgradesSuite) TestAddOriginToIPAddresses(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(ipAddressesC)
	defer closer()

	uuid1 := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()
	uuid3 := utils.MustNewUUID().String()

	err := col.Insert(bson.M{
		"_id":        uuid1 + ":principal/1",
		"model-uuid": uuid1,
		"origin":     "",
	}, bson.M{
		"_id":        uuid1 + ":telegraf/1",
		"model-uuid": uuid1,
		"origin":     "machine",
	}, bson.M{
		"_id":        uuid2 + ":telegraf/0",
		"model-uuid": uuid2,
		"origin":     "provider",
	}, bson.M{
		"_id":        uuid3 + ":base/0",
		"model-uuid": uuid3,
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        uuid1 + ":principal/1",
			"model-uuid": uuid1,
			"origin":     "provider",
		}, {
			"_id":        uuid1 + ":telegraf/1",
			"model-uuid": uuid1,
			"origin":     "machine",
		}, {
			"_id":        uuid2 + ":telegraf/0",
			"model-uuid": uuid2,
			"origin":     "provider",
		}, {
			"_id":        uuid3 + ":base/0",
			"model-uuid": uuid3,
			"origin":     "provider",
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, AddOriginToIPAddresses, upgradedData(col, expected))
}

func (s *upgradesSuite) TestDropPresenceDatabase(c *gc.C) {
	presenceDBName := "presence"
	db := s.state.session.DB(presenceDBName)
	col := db.C("presence")

	err := col.Insert(bson.M{"test": "foo"})
	c.Assert(err, jc.ErrorIsNil)

	presenceDBExists := func() bool {
		names, err := s.state.session.DatabaseNames()
		c.Assert(err, jc.ErrorIsNil)
		dbNames := set.NewStrings(names...)
		return dbNames.Contains(presenceDBName)
	}

	c.Assert(presenceDBExists(), jc.IsTrue)

	err = DropPresenceDatabase(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(presenceDBExists(), jc.IsFalse)

	// Running again is no error.
	err = DropPresenceDatabase(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(presenceDBExists(), jc.IsFalse)
}
func (s *upgradesSuite) TestRemoveUnsupportedLinkLayer(c *gc.C) {
	uuid := utils.MustNewUUID().String()

	devCol, devCloser := s.state.db().GetRawCollection(linkLayerDevicesC)
	defer devCloser()

	retainedDev := bson.M{
		"_id":        uuid + ":m#0#d#eth0",
		"model-uuid": uuid,
		"name":       "eth0",
	}

	err := devCol.Insert(
		retainedDev,
		bson.M{
			"_id":        uuid + ":m#0#d#unsupported0",
			"model-uuid": uuid,
			"name":       "unsupported0",
		},
		bson.M{
			"_id":        uuid + ":m#0#d#unsupported1",
			"model-uuid": uuid,
			"name":       "unsupported1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	addrCol, addrCloser := s.state.db().GetRawCollection(ipAddressesC)
	defer addrCloser()

	retainedAddr := bson.M{
		"_id":         uuid + ":m#0#d#eth0#ip#30.30.30.30",
		"model-uuid":  uuid,
		"device-name": "eth0",
	}

	err = addrCol.Insert(
		retainedAddr,
		bson.M{
			"_id":         uuid + ":m#0#d#unsupported0#ip#10.10.10.10",
			"model-uuid":  uuid,
			"device-name": "unsupported0",
		},
		bson.M{
			"_id":         uuid + ":m#0#d#unsupported1#ip#20.20.20.20",
			"model-uuid":  uuid,
			"device-name": "unsupported1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.assertUpgradedData(c, RemoveUnsupportedLinkLayer,
		upgradedData(devCol, []bson.M{retainedDev}),
		upgradedData(addrCol, []bson.M{retainedAddr}),
	)
}

func (s *upgradesSuite) TestAddBakeryConfig(c *gc.C) {
	const bakeryConfigKey = "bakeryConfig"
	controllerColl, controllerCloser := s.state.db().GetRawCollection(controllersC)
	defer controllerCloser()

	err := controllerColl.RemoveId(bakeryConfigKey)
	c.Assert(err, jc.ErrorIsNil)

	bakeryConfig := s.state.NewBakeryConfig()
	_, err = bakeryConfig.GetLocalUsersKey()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = AddBakeryConfig(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	key, err := bakeryConfig.GetLocalUsersKey()
	c.Assert(err, jc.ErrorIsNil)

	// Check it's idempotent.
	err = AddBakeryConfig(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	key2, err := bakeryConfig.GetLocalUsersKey()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(key, jc.DeepEquals, key2)
}

func (s *upgradesSuite) TestReplaceNeverSetWithUnset(c *gc.C) {
	const bakeryConfigKey = "bakeryConfig"
	coll, closer := s.state.db().GetCollection(statusesC)
	defer closer()

	type oldDoc struct {
		ID         string        `bson:"_id"`
		Status     status.Status `bson:"status"`
		StatusInfo string        `bson:"statusinfo"`
		NeverSet   bool          `bson:"neverset"`
	}

	// Insert two statuses, one with neverset true, and one with false.
	ops := []txn.Op{
		{
			C:  statusesC,
			Id: "neverset-true",
			Insert: oldDoc{
				Status:     status.Waiting,
				StatusInfo: status.MessageWaitForMachine,
				NeverSet:   true,
			},
		}, {
			C:  statusesC,
			Id: "neverset-false",
			Insert: oldDoc{
				Status:     status.Active,
				StatusInfo: "all good",
			},
		},
	}
	err := s.state.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	err = ReplaceNeverSetWithUnset(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	checkNoNeverSetAttribute := func() {
		var doc bson.M
		err := coll.FindId("neverset-true").One(&doc)
		c.Assert(err, jc.ErrorIsNil)
		_, found := doc["neverset"]
		c.Check(found, jc.IsFalse)
		err = coll.FindId("neverset-false").One(&doc)
		c.Assert(err, jc.ErrorIsNil)
		_, found = doc["neverset"]
		c.Check(found, jc.IsFalse)
	}

	checkDocs := func() {
		var doc statusDoc
		err := coll.FindId("neverset-true").One(&doc)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(doc, jc.DeepEquals, statusDoc{
			ModelUUID: s.state.ModelUUID(),
			Status:    status.Unset,
		})
		err = coll.FindId("neverset-false").One(&doc)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(doc, jc.DeepEquals, statusDoc{
			ModelUUID:  s.state.ModelUUID(),
			Status:     status.Active,
			StatusInfo: "all good",
		})
	}
	checkDocs()
	checkNoNeverSetAttribute()

	// Check it's idempotent.
	err = ReplaceNeverSetWithUnset(s.pool)
	c.Assert(err, jc.ErrorIsNil)

	checkDocs()
	checkNoNeverSetAttribute()
}

func (s *upgradesSuite) TestResetDefaultRelationLimitInCharmMetadata(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(charmsC)
	defer closer()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	model3 := s.makeModel(c, "model-3", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
		_ = model3.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()
	uuid3 := model3.ModelUUID()

	// Setup charm metadata as it would appear when parsed by a 2.7
	// controller (limit forced to 1 if not defined in the metadata).
	err := col.Insert(
		genCharmDocWithMetaAndRelationLimit(uuid1, 1),
		genCharmDocWithMetaAndRelationLimit(uuid2, 1),
		genCharmDocWithMetaAndRelationLimit(uuid3, 1),
	)
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		genCharmDocWithMetaAndRelationLimit(uuid1, 0),
		genCharmDocWithMetaAndRelationLimit(uuid2, 0),
		genCharmDocWithMetaAndRelationLimit(uuid3, 0),
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, ResetDefaultRelationLimitInCharmMetadata, upgradedData(col, expected))
}

func genCharmDocWithMetaAndRelationLimit(modelUUID string, relLimit int) bson.M {
	return bson.M{
		"_id":        modelUUID + ":cs:percona-cluster-290",
		"model-uuid": modelUUID,
		"meta": bson.M{
			"name":        "percona-cluster",
			"summary":     "",
			"description": "",
			"subordinate": false,
			"provides": bson.M{
				"db": bson.M{
					"name":      "db",
					"role":      "provider",
					"interface": "mysql",
					"optional":  false,
					"limit":     42, // ensures that "provides" endpoints are not changed
					"scope":     "global",
				},
				"master": bson.M{
					"name":      "master",
					"role":      "provider",
					"interface": "mysql-async-replication",
					"optional":  false,
					"limit":     42, // ensures that "provides" endpoints are not changed
					"scope":     "global",
				},
			},
			"requires": bson.M{
				"ha": bson.M{
					"name":      "ha",
					"role":      "requirer",
					"interface": "hacluster",
					"optional":  false,
					"limit":     relLimit,
					"scope":     "container",
				},
				"slave": bson.M{
					"name":      "slave",
					"role":      "requirer",
					"interface": "mysql-async-replication",
					"optional":  false,
					"limit":     relLimit,
					"scope":     "global",
				},
			},
			"peers": bson.M{
				"cluster": bson.M{
					"name":      "cluster",
					"role":      "peer",
					"interface": "percona-cluster",
					"optional":  false,
					"limit":     relLimit,
					"scope":     "global",
				},
			},
		},
	}

}

func (s *upgradesSuite) TestAddCharmHubToModelConfig(c *gc.C) {
	// Value not set
	m1 := s.makeModel(c, "m1", coretesting.Attrs{
		"other-setting":  "val",
		"dotted.setting": "value",
		"dollar$setting": "value",
	})
	defer func() { _ = m1.Close() }()
	// Value set to something other that default
	m2 := s.makeModel(c, "m3", coretesting.Attrs{
		"charm-hub-url": "http://meshuggah.rocks",
	})
	defer func() { _ = m2.Close() }()

	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	// To simulate a 2.9.0 without any setting, delete the record from it.
	err := settingsColl.UpdateId(m1.ModelUUID()+":e",
		bson.M{"$unset": bson.M{"settings.charm-hub-url": 1}},
	)
	c.Assert(err, jc.ErrorIsNil)
	// And an extra document from somewhere else that we shouldn't touch
	err = settingsColl.Insert(
		bson.M{
			"_id":      "not-a-model",
			"settings": bson.M{"other-setting": "val"},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// Read all the settings from the database, but make sure to change the
	// documents we think we're changing, and the rest should go through
	// unchanged.
	var rawSettings bson.M
	iter := settingsColl.Find(nil).Sort("_id").Iter()
	defer iter.Close()

	expectedSettings := []bson.M{}

	expectedChanges := map[string]bson.M{
		m1.ModelUUID() + ":e": {"charm-hub-url": charmhub.CharmHubServerURL, "other-setting": "val"},
		m2.ModelUUID() + ":e": {"charm-hub-url": "http://meshuggah.rocks"},
		"not-a-model":         {"other-setting": "val"},
	}
	for iter.Next(&rawSettings) {
		expSettings := copyMap(rawSettings, nil)
		delete(expSettings, "txn-queue")
		delete(expSettings, "txn-revno")
		delete(expSettings, "version")
		id, ok := expSettings["_id"]
		c.Assert(ok, jc.IsTrue)
		idStr, ok := id.(string)
		c.Assert(ok, jc.IsTrue)
		c.Assert(idStr, gc.Not(gc.Equals), "")
		if changes, ok := expectedChanges[idStr]; ok {
			raw, ok := expSettings["settings"]
			c.Assert(ok, jc.IsTrue)
			settings, ok := raw.(bson.M)
			c.Assert(ok, jc.IsTrue)
			for k, v := range changes {
				settings[k] = v
			}
		}
		expectedSettings = append(expectedSettings, expSettings)
	}
	c.Assert(iter.Close(), jc.ErrorIsNil)

	s.assertUpgradedData(c, AddCharmHubToModelConfig,
		upgradedData(settingsColl, expectedSettings),
	)
}

func (s *upgradesSuite) TestRollUpAndConvertOpenedPortDocuments(c *gc.C) {
	col, closer := s.state.db().GetRawCollection(openedPortsC)
	defer closer()

	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	err := col.Insert(
		// ---- model 1 ----
		bson.M{
			"_id":        ensureModelUUID(uuid1, "m#3"),
			"model-uuid": uuid1,
			"machine-id": "3",
			"ports": []bson.M{
				{"unitname": "foo/0", "fromport": 10, "toport": 42, "protocol": "tcp"},
				{"unitname": "bar/0", "fromport": 20, "toport": 40, "protocol": "udp"},
			},
		},
		bson.M{
			"_id":        ensureModelUUID(uuid1, "m#3#42"),
			"model-uuid": uuid1,
			"machine-id": "3",
			// NOTE(achilleasa) A doc with a non-empty subnet ID
			// should never appear in a real juju deployment. It is
			// added here to make sure the upgrade step does not
			// choke if it sees one.
			"subnet-id": "7007",
			"ports": []bson.M{
				{"unitname": "foo/0", "fromport": 1337, "toport": 1337, "protocol": "tcp"},
			},
		},
		bson.M{
			"_id":        ensureModelUUID(uuid1, "m#4#"),
			"model-uuid": uuid1,
			"machine-id": "4",
			"ports": []bson.M{
				{"unitname": "baz/0", "fromport": 10, "toport": 42, "protocol": "tcp"},
			},
		},
		// ---- model 2 ----
		bson.M{
			"_id":        ensureModelUUID(uuid2, "m#4#42"),
			"model-uuid": uuid2,
			"machine-id": "4",
			"subnet-id":  "42",
			"ports": []bson.M{
				{"unitname": "foo/0", "fromport": -1, "toport": -1, "protocol": "icmp"},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		// The altered portDocs:
		{
			"_id":        uuid1 + ":3",
			"model-uuid": uuid1,
			"machine-id": "3",
			"unit-port-ranges": bson.M{
				"foo/0": bson.M{
					allEndpoints: []interface{}{
						bson.M{"fromport": 10, "toport": 42, "protocol": "tcp"},
						bson.M{"fromport": 1337, "toport": 1337, "protocol": "tcp"},
					},
				},
				"bar/0": bson.M{
					allEndpoints: []interface{}{
						bson.M{"fromport": 20, "toport": 40, "protocol": "udp"},
					},
				},
			},
		}, {
			"_id":        uuid1 + ":4",
			"model-uuid": uuid1,
			"machine-id": "4",
			"unit-port-ranges": bson.M{
				"baz/0": bson.M{
					allEndpoints: []interface{}{
						bson.M{"fromport": 10, "toport": 42, "protocol": "tcp"},
					},
				},
			},
		}, {
			"_id":        uuid2 + ":4",
			"model-uuid": uuid2,
			"machine-id": "4",
			"unit-port-ranges": bson.M{
				"foo/0": bson.M{
					allEndpoints: []interface{}{
						bson.M{"fromport": -1, "toport": -1, "protocol": "icmp"},
					},
				},
			},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, RollUpAndConvertOpenedPortDocuments, upgradedData(col, expected))
}

func (s *upgradesSuite) TestAddCharmOriginToApplication(c *gc.C) {
	model1 := s.makeModel(c, "model-1", coretesting.Attrs{})
	model2 := s.makeModel(c, "model-2", coretesting.Attrs{})
	defer func() {
		_ = model1.Close()
		_ = model2.Close()
	}()

	uuid1 := model1.ModelUUID()
	uuid2 := model2.ModelUUID()

	coll, closer := s.state.db().GetRawCollection(applicationsC)
	defer closer()

	err := coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app1"),
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("cs:test").String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid1, "app2"),
		"model-uuid": uuid1,
		"charmurl":   charm.MustParseURL("local:test").String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app3"),
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("local:test").String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = coll.Insert(bson.M{
		"_id":        ensureModelUUID(uuid2, "app4"),
		"model-uuid": uuid2,
		"charmurl":   charm.MustParseURL("local:test").String(),
		"charm-origin": bson.M{
			"channel": bson.M{
				"track": "latest",
				"risk":  "edge",
			},
			"hash":     "xxxx",
			"id":       "yyyy",
			"revision": 12,
			"source":   "charm-hub",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	expected := bsonMById{
		{
			"_id":        ensureModelUUID(uuid1, "app1"),
			"model-uuid": uuid1,
			"charmurl":   "cs:test",
			"charm-origin": bson.M{
				"channel": bson.M{
					"risk": "",
				},
				"hash":     "",
				"id":       "cs:test",
				"revision": -1,
				"source":   "charm-store",
			},
		},
		{
			"_id":        ensureModelUUID(uuid1, "app2"),
			"model-uuid": uuid1,
			"charmurl":   "local:test",
			"charm-origin": bson.M{
				"channel": bson.M{
					"risk": "",
				},
				"hash":     "",
				"id":       "local:test",
				"revision": -1,
				"source":   "local",
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app3"),
			"model-uuid": uuid2,
			"charmurl":   "local:test",
			"charm-origin": bson.M{
				"channel": bson.M{
					"risk": "",
				},
				"hash":     "",
				"id":       "local:test",
				"revision": -1,
				"source":   "local",
			},
		},
		{
			"_id":        ensureModelUUID(uuid2, "app4"),
			"model-uuid": uuid2,
			"charmurl":   "local:test",
			"charm-origin": bson.M{
				"channel": bson.M{
					"track": "latest",
					"risk":  "edge",
				},
				"hash":     "xxxx",
				"id":       "yyyy",
				"revision": 12,
				"source":   "charm-hub",
			},
		},
	}

	sort.Sort(expected)
	s.assertUpgradedData(c, AddCharmOriginToApplications,
		upgradedData(coll, expected),
	)
}

func (s *upgradesSuite) TestAddAzureProviderNetworkConfig(c *gc.C) {
	settingsColl, settingsCloser := s.state.db().GetRawCollection(settingsC)
	defer settingsCloser()
	_, err := settingsColl.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = settingsColl.Insert(bson.M{
		// already upgraded
		"_id": "foo",
		"settings": bson.M{
			"type":          "azure",
			"use-public-ip": false},
	}, bson.M{
		// non azure model
		"_id": "bar",
		"settings": bson.M{
			"type": "ec2"},
	}, bson.M{
		"_id": "baz",
		"settings": bson.M{
			"type": "azure"},
	})
	c.Assert(err, jc.ErrorIsNil)

	expectedSettings := bsonMById{
		{
			"_id": "bar",
			"settings": bson.M{
				"type": "ec2"},
		}, {
			"_id": "baz",
			"settings": bson.M{
				"type":          "azure",
				"use-public-ip": true,
			},
		}, {
			"_id": "foo",
			"settings": bson.M{
				"type":          "azure",
				"use-public-ip": false},
		}}

	//sort.Sort(expectedSettings)
	s.assertUpgradedData(c, AddAzureProviderNetworkConfig,
		upgradedData(settingsColl, expectedSettings),
	)
}

type docById []bson.M

func (d docById) Len() int           { return len(d) }
func (d docById) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d docById) Less(i, j int) bool { return d[i]["_id"].(string) < d[j]["_id"].(string) }
