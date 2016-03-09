// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

type upgradesSuite struct {
	internalStateSuite
}

var _ = gc.Suite(&upgradesSuite{})

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
	_, err := s.state.AddMachine("quantal", JobManageModel)
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

func (s *upgradesSuite) getDocMap(c *gc.C, docID, collection string) (map[string]interface{}, error) {
	docMap := map[string]interface{}{}
	coll, closer := s.state.getRawCollection(collection)
	defer closer()
	err := coll.Find(bson.D{{"_id", docID}}).One(&docMap)
	return docMap, err
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

func (s *upgradesSuite) TestMigrateSettingsSchema(c *gc.C) {
	// Insert test documents.
	settingsColl, closer := s.state.getRawCollection(settingsC)
	defer closer()
	err := settingsColl.Insert(
		bson.D{
			// Post-model-uuid migration, with no settings.
			{"_id", "1"},
			{"model-uuid", "model-uuid"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
		},
		bson.D{
			// Post-model-uuid migration, with settings. One
			// of the settings is called "settings", and
			// one "version".
			{"_id", "2"},
			{"model-uuid", "model-uuid"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
			{"settings", int64(123)},
			{"version", "onetwothree"},
		},
		bson.D{
			// Pre-model-uuid migration, with no settings.
			{"_id", "3"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
		},
		bson.D{
			// Pre-model-uuid migration, with settings.
			{"_id", "4"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
			{"settings", int64(123)},
			{"version", "onetwothree"},
		},
		bson.D{
			// Already migrated, with no settings.
			{"_id", "5"},
			{"model-uuid", "model-uuid"},
			{"txn-revno", int64(99)},
			{"txn-queue", []string{}},
			{"version", int64(98)},
			{"settings", map[string]interface{}{}},
		},
		bson.D{
			// Already migrated, with settings.
			{"_id", "6"},
			{"model-uuid", "model-uuid"},
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
		"_id":        "1",
		"model-uuid": "model-uuid",
		"txn-revno":  int64(100),
		"settings":   bson.M{},
		"version":    int64(99),
	}, {
		"_id":        "2",
		"model-uuid": "model-uuid",
		"txn-revno":  int64(101),
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
		"_id":        "5",
		"model-uuid": "model-uuid",
		"txn-revno":  int64(99),
		"version":    int64(98),
		"settings":   bson.M{},
	}, {
		"_id":        "6",
		"model-uuid": "model-uuid",
		"txn-revno":  int64(99),
		"version":    int64(98),
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
			bson.D{{"model-uuid", bson.D{{"$ne", s.state.ModelUUID()}}}},
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
	_, err = s.state.AddModelUser(ModelUserSpec{
		User:        ownerTag,
		CreatedBy:   ownerTag,
		DisplayName: "",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Add a coule of test spaces, but notably NOT the default one.
	_, err = s.state.AddSpace("db", "", nil, false)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.state.AddSpace("apps", "", nil, true)
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
		serviceBindings, err := services[i].EndpointBindings()
		if err != nil {
			c.Fatalf("unexpected error getting service %q bindings: %v", serviceName, err)
		}
		currentBindings[serviceName] = serviceBindings
	}
	return currentBindings
}

func (s *upgradesSuite) testAddDefaultEndpointBindingsToServices(c *gc.C, runTwice bool) {
	services := s.setupAddDefaultEndpointBindingsToServices(c)
	initialBindings := s.getServicesBindings(c, services)
	wpAllDefaults := map[string]string{
		"url":             "",
		"logging-dir":     "",
		"monitoring-port": "",
		"db":              "",
		"cache":           "",
	}
	msAllDefaults := map[string]string{
		"server": "",
	}
	expectedInitialAndFinal := map[string]map[string]string{
		"wp-no-bindings":      wpAllDefaults,
		"wp-default-bindings": wpAllDefaults,
		"wp-given-bindings": map[string]string{
			"url":             "apps",
			"logging-dir":     "",
			"monitoring-port": "",
			"db":              "db",
			"cache":           "",
		},

		"ms-no-bindings":      msAllDefaults,
		"ms-default-bindings": msAllDefaults,
		"ms-given-bindings": map[string]string{
			"server": "db",
		},
	}
	c.Assert(initialBindings, jc.DeepEquals, expectedInitialAndFinal)

	assertFinalBindings := func() {
		finalBindings := s.getServicesBindings(c, services)
		c.Assert(finalBindings, jc.DeepEquals, expectedInitialAndFinal)
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
