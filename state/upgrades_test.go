// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type upgradesSuite struct {
	gitjujutesting.CleanupSuite
	testing.BaseSuite
	gitjujutesting.MgoSuite
	state *State
	owner names.UserTag
}

func (s *upgradesSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
	s.CleanupSuite.SetUpSuite(c)
}

func (s *upgradesSuite) TearDownSuite(c *gc.C) {
	s.CleanupSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *upgradesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.CleanupSuite.SetUpTest(c)
	var err error
	s.owner = names.NewLocalUserTag("upgrade-admin")
	s.state, err = Initialize(s.owner, TestingMongoInfo(), testing.EnvironConfig(c), TestingDialOpts(), Policy(nil))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) TearDownTest(c *gc.C) {
	if s.state != nil {
		s.state.Close()
	}
	s.CleanupSuite.TearDownTest(c)
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
	err := s.state.runRawTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)

	err = MigrateUserLastConnectionToLastLogin(s.state)
	c.Assert(err, jc.ErrorIsNil)
	user, err := s.state.User(names.NewLocalUserTag(userId))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*user.LastLogin(), gc.Equals, now)

	// check to see if _id_ field is removed
	userMap := map[string]interface{}{}
	users, closer := s.state.getRawCollection("users")
	defer closer()
	err = users.Find(bson.D{{"_id", userId}}).One(&userMap)
	c.Assert(err, jc.ErrorIsNil)
	_, keyExists := userMap["_id_"]
	c.Assert(keyExists, jc.IsFalse)
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
	c.Assert(admin.UserTag().Username(), gc.DeepEquals, s.owner.Username())
	bob, err := s.state.EnvironmentUser(bobTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bob.UserTag().Username(), gc.DeepEquals, bobTag.Username())
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
	c.Assert(admin.UserTag().Username(), gc.DeepEquals, s.owner.Username())
	bob, err := s.state.EnvironmentUser(bobTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bob.UserTag().Username(), gc.DeepEquals, bobTag.Username())
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
	c.Assert(newDoc.ProviderId, gc.Equals, network.Id("net1"))
	c.Assert(newDoc.CIDR, gc.Equals, "0.1.2.0/24")

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.ProviderId, gc.Equals, network.Id("net2"))
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
	// The test expects three records because there is a preexisting environment constraints doc in mongo.
	c.Assert(count, gc.Equals, 3)

	var newDoc constraintsDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(*newDoc.CpuCores, gc.Equals, uint64(4))
	c.Assert(*newDoc.Networks, gc.DeepEquals, networks1)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(*newDoc.CpuCores, gc.Equals, uint64(8))
	c.Assert(*newDoc.Networks, gc.DeepEquals, networks2)
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
			"code": MeterGreen,
		},
		bson.M{
			"_id":  "u#bar/0",
			"code": MeterRed,
		},
	)

	var newDoc meterStatusDoc
	s.FindId(c, coll, newIDs[0], &newDoc)
	c.Assert(newDoc.Code, gc.Equals, MeterGreen)

	s.FindId(c, coll, newIDs[1], &newDoc)
	c.Assert(newDoc.Code, gc.Equals, MeterRed)
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
	c.Assert(env.ServerTag().Id(), gc.Equals, env.UUID())
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
	c.Assert(env.ServerTag().Id(), gc.Equals, env.UUID())
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
	_, err = s.state.AddEnvironmentUser(ownerTag, ownerTag)
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
				c.Assert(unit.doc.Ports, jc.DeepEquals, []network.Port{
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
		func(st *State, pDoc *portsDoc, portsAssert interface{}, ports ...PortRange) ([]txn.Op, error) {
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
			}}, nil
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
	_, err = s.state.AddEnvironmentUser(ownerTag, ownerTag)
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
		_, _, err := unit.GetMeterStatus()
		c.Assert(err, gc.ErrorMatches, "cannot retrieve meter status for unit .*: not found")
	}

	// run meter status upgrade
	err := CreateUnitMeterStatus(s.state)
	c.Assert(err, jc.ErrorIsNil)

	// assert the units do not have meter status documents
	for _, unit := range units {
		code, info, err := unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(code, gc.Equals, "NOT SET")
		c.Assert(info, gc.Equals, "")
	}

	// run migration again to make sure it's idempotent
	err = CreateUnitMeterStatus(s.state)
	c.Assert(err, jc.ErrorIsNil)
	for _, unit := range units {
		code, info, err := unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(code, gc.Equals, "NOT SET")
		c.Assert(info, gc.Equals, "")
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
		txn.Op{
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
	machineID := "9999"
	var instID instance.Id = "9999"
	s.azSetUp(c, machineID, instID)

	azfunc := func(*State, instance.Id) (string, error) {
		return "a_zone", nil
	}

	err := AddAvailabilityZoneToInstanceData(s.state, azfunc)
	c.Assert(err, jc.ErrorIsNil)

	s.checkAvailabilityZone(c, machineID, "a_zone")
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

	zone, ok := instanceMap["availzone"]
	c.Check(ok, jc.IsTrue)
	c.Check(zone, gc.Equals, expectedZone)
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
	query := s.state.db.C(sequenceC).FindId(s.state.docID("machine"))
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
