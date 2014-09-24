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
	charmtesting "gopkg.in/juju/charm.v4/testing"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
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
	s.checkAddEnvUUIDToCollection(c, servicesC, AddEnvUUIDToServicesID)
}

func (s *upgradesSuite) TestAddEnvUUIDToServicesIDIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, servicesC, AddEnvUUIDToServicesID)
}

func (s *upgradesSuite) TestAddEnvUUIDToUnits(c *gc.C) {
	s.checkAddEnvUUIDToCollection(c, unitsC, AddEnvUUIDToUnits)
}

func (s *upgradesSuite) TestAddEnvUUIDToUnitsIdempotent(c *gc.C) {
	s.checkAddEnvUUIDToCollectionIdempotent(c, unitsC, AddEnvUUIDToUnits)
}

func (s *upgradesSuite) checkAddEnvUUIDToCollection(
	c *gc.C,
	collName string,
	upgradeStep func(*State) error,
) {
	s.addLegacyDocWithNoEnvUUID(c, collName, "foo")

	coll, closer := s.state.getCollection(collName)
	defer closer()

	err := upgradeStep(s.state)
	c.Assert(err, gc.IsNil)

	var doc map[string]string
	err = coll.Find(bson.D{{"_id", "foo"}}).One(&doc)
	c.Assert(err, gc.Equals, mgo.ErrNotFound)

	err = coll.Find(bson.D{{"_id", s.state.docID("foo")}}).One(&doc)
	c.Assert(err, gc.IsNil)
	c.Assert(doc["name"], gc.Equals, "foo")
	c.Assert(doc["env-uuid"], gc.Equals, s.state.EnvironTag().Id())
}

func (s *upgradesSuite) checkAddEnvUUIDToCollectionIdempotent(
	c *gc.C,
	collName string,
	upgradeStep func(*State) error,
) {
	s.addLegacyDocWithNoEnvUUID(c, collName, "foo")

	coll, closer := s.state.getCollection(collName)
	defer closer()

	err := upgradeStep(s.state)
	c.Assert(err, gc.IsNil)

	err = upgradeStep(s.state)
	c.Assert(err, gc.IsNil)

	var docs []map[string]string
	err = coll.Find(nil).All(&docs)
	c.Assert(err, gc.IsNil)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0]["_id"], gc.Equals, s.state.docID("foo"))
}

func (s *upgradesSuite) addLegacyDocWithNoEnvUUID(c *gc.C, collName, entityName string) {
	// Bare minimum entity document as of 1.21-alpha1
	oldDoc := struct {
		Name string `bson:"_id"`
	}{Name: entityName}
	ops := []txn.Op{{
		C:      collName,
		Id:     entityName,
		Assert: txn.DocMissing,
		Insert: oldDoc,
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

func openLegacyPort(c *gc.C, unit *Unit, number int, proto string) {
	port := network.Port{Protocol: proto, Number: number}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     unit.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$addToSet", bson.D{{"ports", port}}}},
	}}
	err := unit.st.runTransaction(ops)
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	// Add the charm, services and units, assign to machines.
	services := make([]*Service, 3)
	units := make(map[int][]*Unit)
	networks := []string{network.DefaultPublic}
	charm := AddTestingCharm(c, s.state, "wordpress")
	stateOwner, err := s.state.AddUser("bob", "notused", "notused", "bob")
	c.Assert(err, gc.IsNil)
	ownerTag := stateOwner.UserTag()
	_, err = s.state.AddEnvironmentUser(ownerTag, ownerTag)
	c.Assert(err, gc.IsNil)

	for i := range services {
		name := fmt.Sprintf("wp%d", i)
		services[i] = AddTestingServiceWithNetworks(
			c, s.state, name, charm, ownerTag, networks,
		)
		numUnits := i + 1
		units[i] = make([]*Unit, numUnits)
		for j := 0; j < numUnits; j++ {
			unit, err := services[i].AddUnit()
			c.Assert(err, gc.IsNil)
			switch {
			case j == 0:
				// The first unit of each service goes to a machine
				// with the same index as the service.
				err = unit.AssignToMachine(machines[i])
				c.Assert(err, gc.IsNil)
			case j == 1 && i >= 1:
				// Co-locate the second unit of each service. Leave
				// units[2][2] unassigned.
				err = unit.AssignToMachine(machines[i-1])
				c.Assert(err, gc.IsNil)
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
	portRange, err := NewPortRange(units[2][1].Name(), 100, 110, "tcp")
	c.Assert(err, gc.IsNil)
	portsMachine2, err := GetOrCreatePorts(
		s.state, machines[2].Id(), network.DefaultPublic,
	)
	c.Assert(err, gc.IsNil)
	err = portsMachine2.OpenPorts(portRange)
	c.Assert(err, gc.IsNil)
	//
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
		portsKey := PortsGlobalKey(machines[i].Id(), network.DefaultPublic)
		ports, err := s.state.Ports(portsKey)
		if i != 2 {
			c.Assert(err, jc.Satisfies, errors.IsNotFound)
			c.Assert(ports, gc.IsNil)
		} else {
			c.Assert(err, gc.IsNil)
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
			c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
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
	machines, units := s.setUpPortsMigration(c)

	// Ensure there are no new-style port ranges before the migration,
	// except for macines[2].
	s.assertInitialMachinePorts(c, machines, units)

	err := MigrateUnitPortsToOpenedPorts(s.state)
	c.Assert(err, gc.IsNil)

	// Ensure there are no ports on the migrated units' documents,
	// except for units[2][2].
	s.assertUnitPortsPostMigration(c, units)

	// Ensure new-style port ranges are migrated as expected.
	s.assertFinalMachinePorts(c, machines, units)
}

func (s *upgradesSuite) TestMigrateUnitPortsToOpenedPortsIdempotent(c *gc.C) {
	machines, units := s.setUpPortsMigration(c)

	// Ensure there are no new-style port ranges before the migration,
	// except for macines[2].
	s.assertInitialMachinePorts(c, machines, units)

	err := MigrateUnitPortsToOpenedPorts(s.state)
	c.Assert(err, gc.IsNil)

	// Ensure there are no ports on the migrated units' documents,
	// except for units[2][2].
	s.assertUnitPortsPostMigration(c, units)

	// Ensure new-style port ranges are migrated as expected.
	s.assertFinalMachinePorts(c, machines, units)

	// Migrate and check again, should work fine.
	err = MigrateUnitPortsToOpenedPorts(s.state)
	c.Assert(err, gc.IsNil)
	s.assertUnitPortsPostMigration(c, units)
	s.assertFinalMachinePorts(c, machines, units)
}
