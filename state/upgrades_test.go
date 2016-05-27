// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
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

func (s *upgradesSuite) removeAllMachinesPreferredAddressFields(c *gc.C, machines []*Machine) {
	ops := make([]txn.Op, len(machines))
	for i, machine := range machines {
		ops[i] = txn.Op{
			C:  machinesC,
			Id: machine.doc.DocID,
			Update: bson.D{{"$unset", bson.D{
				{"preferredpublicaddress", nil},
				{"preferredprivateaddress", nil},
			}}}}
	}
	err := s.state.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradesSuite) setAllMachinesPreferredAddressFields(c *gc.C, machines []*Machine, addr string) {
	stateAddr := fromNetworkAddress(network.NewAddress(addr), OriginUnknown)

	ops := make([]txn.Op, len(machines))
	for i, machine := range machines {
		ops[i] = txn.Op{
			C:  machinesC,
			Id: machine.doc.DocID,
			Update: bson.D{{"$set", bson.D{
				{"preferredpublicaddress", stateAddr},
				{"preferredprivateaddress", stateAddr},
			}}}}
	}
	err := s.state.runTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func assertMachineAddresses(c *gc.C, machine *Machine, publicAddress, privateAddress string) {
	err := machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	addr, err := machine.PublicAddress()
	c.Assert(addr.Value, gc.Equals, publicAddress)
	if publicAddress != "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.Satisfies, network.IsNoAddressError)
	}

	privAddr, err := machine.PrivateAddress()
	c.Assert(privAddr.Value, gc.Equals, privateAddress)
	if privateAddress != "" {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, jc.Satisfies, network.IsNoAddressError)
	}
}

func (s *upgradesSuite) createMachinesWithAddresses(c *gc.C) []*Machine {
	template := MachineTemplate{Series: "quantal", Jobs: []MachineJob{JobHostUnits}}
	machines, err := s.state.AddMachines([]MachineTemplate{
		template, template, template, template, template, template,
	}...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines, gc.HasLen, 6)

	m0 := machines[0] // has only provider addresses
	m1 := machines[1] // has only machine addresses
	m2 := machines[2] // has both machine and provider addresses (set in that order)
	m3 := machines[3] // has both provider and machine addresses (in that order)
	m4 := machines[4] // has neither
	m5 := machines[5] // is set to dead

	err = m0.SetProviderAddresses(network.NewAddress("8.8.0.8"))
	c.Assert(err, jc.ErrorIsNil)

	err = m1.SetMachineAddresses(network.NewAddress("8.8.1.8"))
	c.Assert(err, jc.ErrorIsNil)

	err = m2.SetMachineAddresses(network.NewAddress("10.0.2.1"))
	c.Assert(err, jc.ErrorIsNil)
	err = m2.SetProviderAddresses(network.NewAddresses("10.0.2.2", "8.8.2.8")...)
	c.Assert(err, jc.ErrorIsNil)

	err = m3.SetProviderAddresses(network.NewAddresses("10.0.3.2", "8.8.3.8")...)
	c.Assert(err, jc.ErrorIsNil)
	err = m3.SetMachineAddresses(network.NewAddress("10.0.3.1"))
	c.Assert(err, jc.ErrorIsNil)

	assertMachineAddresses(c, m4, "", "")
	assertMachineAddresses(c, m5, "", "")

	err = m5.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// Delete the preferred address fields.
	s.removeAllMachinesPreferredAddressFields(c, machines)
	return machines
}

func (s *upgradesSuite) TestAddPreferredAddressesToMachines(c *gc.C) {
	machines := s.createMachinesWithAddresses(c)
	s.addPreferredAddressesToMachinesAndAssertResults(c, machines)
}

func (s *upgradesSuite) addPreferredAddressesToMachinesAndAssertResults(c *gc.C, machines []*Machine) {
	err := AddPreferredAddressesToMachines(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertMachineAddresses(c, machines[0], "8.8.0.8", "8.8.0.8")
	assertMachineAddresses(c, machines[1], "8.8.1.8", "8.8.1.8")
	assertMachineAddresses(c, machines[2], "8.8.2.8", "10.0.2.2")
	assertMachineAddresses(c, machines[3], "8.8.3.8", "10.0.3.2")
	assertMachineAddresses(c, machines[4], "", "")
	assertMachineAddresses(c, machines[5], "", "")
}

func (s *upgradesSuite) TestAddPreferredAddressesToMachinesIdempotent(c *gc.C) {
	machines := s.createMachinesWithAddresses(c)
	s.addPreferredAddressesToMachinesAndAssertResults(c, machines)
	s.addPreferredAddressesToMachinesAndAssertResults(c, machines)
	s.addPreferredAddressesToMachinesAndAssertResults(c, machines)
}

func (s *upgradesSuite) TestAddPreferredAddressesToMachinesUpdatesExistingFields(c *gc.C) {
	machines := s.createMachinesWithAddresses(c)
	s.setAllMachinesPreferredAddressFields(c, machines, "1.1.2.2")

	for _, m := range machines {
		assertMachineAddresses(c, m, "1.1.2.2", "1.1.2.2")
	}

	err := AddPreferredAddressesToMachines(s.state)
	c.Assert(err, jc.ErrorIsNil)

	assertMachineAddresses(c, machines[0], "8.8.0.8", "8.8.0.8")
	assertMachineAddresses(c, machines[1], "8.8.1.8", "8.8.1.8")
	assertMachineAddresses(c, machines[2], "8.8.2.8", "10.0.2.2")
	assertMachineAddresses(c, machines[3], "8.8.3.8", "10.0.3.2")
	assertMachineAddresses(c, machines[4], "1.1.2.2", "1.1.2.2") // had none & none were set
	assertMachineAddresses(c, machines[5], "1.1.2.2", "1.1.2.2") // was dead => none were set
}

func (s *upgradesSuite) TestIPv6AddressesAreNeverSetAsPreferred(c *gc.C) {
	machine, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	assertMachineAddresses(c, machine, "", "") // nothing set by default

	providerIPv6Addrs := network.NewAddresses("2001:db8::1", "::1")
	err = machine.SetProviderAddresses(providerIPv6Addrs...)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineAddresses(c, machine, "", "")                              // IPv6 are ignored for preferred
	c.Assert(machine.ProviderAddresses(), jc.DeepEquals, providerIPv6Addrs) // but are still saved
	c.Assert(machine.Addresses(), jc.DeepEquals, providerIPv6Addrs)         // only have OriginProvider

	machineIPv6Addrs := network.NewAddresses("fc00:123::1", "::1")
	err = machine.SetMachineAddresses(machineIPv6Addrs...)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineAddresses(c, machine, "", "")                              // IPv6 still ignored
	c.Assert(machine.ProviderAddresses(), jc.DeepEquals, providerIPv6Addrs) // unchanged
	combinedAddrs := network.MergedAddresses(machineIPv6Addrs, providerIPv6Addrs)
	c.Assert(machine.Addresses(), jc.DeepEquals, combinedAddrs)

	providerMixedAddrs := network.NewAddresses("8.8.8.8", "fe80:1234::1")
	err = machine.SetProviderAddresses(providerMixedAddrs...)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineAddresses(c, machine, "8.8.8.8", "8.8.8.8")                 // only IPv4 used for both
	c.Assert(machine.ProviderAddresses(), jc.DeepEquals, providerMixedAddrs) // updated
	combinedAddrs = network.MergedAddresses(machineIPv6Addrs, providerMixedAddrs)
	c.Assert(machine.Addresses(), jc.DeepEquals, combinedAddrs)

	machineMixedAddrs := network.NewAddresses("0.1.2.3", "127.0.0.1", "::1")
	err = machine.SetMachineAddresses(machineMixedAddrs...)
	c.Assert(err, jc.ErrorIsNil)
	assertMachineAddresses(c, machine, "8.8.8.8", "8.8.8.8")                 // unchanged once set
	c.Assert(machine.ProviderAddresses(), jc.DeepEquals, providerMixedAddrs) // unchanged
	combinedAddrs = network.MergedAddresses(machineMixedAddrs, providerMixedAddrs)
	c.Assert(machine.Addresses(), jc.DeepEquals, combinedAddrs)
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
	s.assertAddFilesystemStatus(c, filesystem, status.StatusPending)
}

func (s *upgradesSuite) TestAddFilesystemStatusDoesNotOverwrite(c *gc.C) {
	_, _, filesystem, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.StatusDestroying,
		Message: "",
		Since:   &now,
	}
	err := filesystem.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	s.assertAddFilesystemStatus(c, filesystem, status.StatusDestroying)
}

func (s *upgradesSuite) TestAddFilesystemStatusProvisioned(c *gc.C) {
	_, _, filesystem, cleanup := setupMachineBoundStorageTests(c, s.state)
	defer cleanup()

	err := s.state.SetFilesystemInfo(filesystem.FilesystemTag(), FilesystemInfo{
		FilesystemId: "fs",
	})
	c.Assert(err, jc.ErrorIsNil)
	removeStatusDoc(c, s.state, filesystem)
	s.assertAddFilesystemStatus(c, filesystem, status.StatusAttaching)
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
	s.assertAddFilesystemStatus(c, filesystem, status.StatusAttached)
}

func (s *upgradesSuite) assertAddFilesystemStatus(c *gc.C, filesystem Filesystem, expect status.Status) {
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

	// Add a couple of test spaces
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
		// relation names
		"url":             "",
		"logging-dir":     "",
		"monitoring-port": "",
		"db":              "",
		"cache":           "",
		// extra-bindings
		"db-client": "",
		"admin-api": "",
		"foo-bar":   "",
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
			"db-client":       "",
			"admin-api":       "",
			"foo-bar":         "",
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
