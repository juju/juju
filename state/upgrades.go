// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider"
)

var upgradesLogger = loggo.GetLogger("juju.state.upgrade")

type userDocBefore struct {
	Name           string    `bson:"_id"`
	LastConnection time.Time `bson:"lastconnection"`
}

func MigrateUserLastConnectionToLastLogin(st *State) error {
	var oldDocs []userDocBefore

	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	users, closer := st.getRawCollection(usersC)
	defer closer()
	err = users.Find(bson.D{{
		"lastconnection", bson.D{{"$exists", true}}}}).All(&oldDocs)
	if err != nil {
		return err
	}

	var zeroTime time.Time

	ops := []txn.Op{}
	for _, oldDoc := range oldDocs {
		upgradesLogger.Debugf("updating user %q", oldDoc.Name)
		var lastLogin *time.Time
		if oldDoc.LastConnection != zeroTime {
			lastLogin = &oldDoc.LastConnection
		}
		ops = append(ops,
			txn.Op{
				C:      usersC,
				Id:     oldDoc.Name,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"lastlogin", lastLogin}}},
					{"$unset", bson.D{{"lastconnection", nil}}},
					{"$unset", bson.D{{"_id_", nil}}},
				},
			})
	}

	return st.runRawTransaction(ops)
}

// AddStateUsersAsEnvironUsers loops through all users stored in state and
// adds them as environment users with a local provider.
func AddStateUsersAsEnvironUsers(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	var userSlice []userDoc
	users, closer := st.getRawCollection(usersC)
	defer closer()

	err = users.Find(nil).All(&userSlice)
	if err != nil {
		return errors.Trace(err)
	}

	for _, uDoc := range userSlice {
		user := &User{
			st:  st,
			doc: uDoc,
		}
		uTag := user.UserTag()

		_, err := st.EnvironmentUser(uTag)
		if err != nil && errors.IsNotFound(err) {
			_, err = st.AddEnvironmentUser(uTag, uTag, "")
			if err != nil {
				return errors.Trace(err)
			}
		} else {
			upgradesLogger.Infof("user '%s' already added to environment", uTag.Canonical())
		}

	}
	return nil
}

func validateUnitPorts(st *State, unit *Unit) (
	skippedRanges int,
	mergedRanges []network.PortRange,
	validRanges []PortRange,
) {
	// Collapse individual ports into port ranges.
	mergedRanges = network.CollapsePorts(networkPorts(unit.doc.Ports))
	upgradesLogger.Debugf("merged raw port ranges for unit %q: %v", unit, mergedRanges)

	skippedRanges = 0

	// Validate each merged range.
	for _, mergedRange := range mergedRanges {
		// Convert to state.PortRange, without validation.
		stateRange := PortRange{
			UnitName: unit.Name(),
			FromPort: mergedRange.FromPort,
			ToPort:   mergedRange.ToPort,
			Protocol: strings.ToLower(mergedRange.Protocol),
		}

		// Validate the constructed range.
		if err := stateRange.Validate(); err != nil {
			// Don't give up yet - log it, but try to sanitize first.
			upgradesLogger.Warningf(
				"merged port range %v invalid; trying to sanitize bounds",
				stateRange,
			)
			stateRange = stateRange.SanitizeBounds()
			upgradesLogger.Debugf(
				"merged range %v sanitized as %v",
				mergedRange, stateRange,
			)
			// Now try again.
			if err := stateRange.Validate(); err != nil {
				// Despite trying, the converted port range is still invalid,
				// just skip the migration and log it.
				upgradesLogger.Warningf(
					"cannot migrate unit %q's invalid ports %v: %v (skipping)",
					unit, stateRange, err,
				)
				skippedRanges++
				continue
			}
		}
		validRanges = append(validRanges, stateRange)
	}
	upgradesLogger.Debugf("unit %q valid merged ranges: %v", unit, validRanges)
	return skippedRanges, mergedRanges, validRanges
}

func beginUnitMigrationOps(st *State, unit *Unit, machineId string) (
	ops []txn.Op,
	machinePorts *Ports,
	err error,
) {
	// First ops ensure both the unit and its assigned machine are
	// not dead.
	ops = []txn.Op{{
		C:      machinesC,
		Id:     st.docID(machineId),
		Assert: notDeadDoc,
	}, {
		C:      unitsC,
		Id:     unit.doc.DocID,
		Assert: notDeadDoc,
	}}

	// TODO(dimitern) 2014-09-10 bug #1337804: network name is
	// hard-coded until multiple network support lands
	machinePorts, err = getOrCreatePorts(st, machineId, network.DefaultPublic)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return ops, machinePorts, nil
}

func filterUnitRangesToMigrate(
	unit *Unit,
	allMachineRanges map[network.PortRange]string,
	validRanges []PortRange,
) (
	rangesToMigrate map[PortRange]bool,
	skippedRanges int,
	err error,
) {
	// Process all existing machine port ranges to validate each
	// of the unit's already validated merged ranges against them.
	rangesToMigrate = make(map[PortRange]bool)
	for unitRange, unitName := range allMachineRanges {
		machineRange, err := NewPortRange(
			unitName,
			unitRange.FromPort,
			unitRange.ToPort,
			unitRange.Protocol,
		)
		if err != nil {
			return nil, 0, errors.Annotatef(
				err,
				"invalid existing ports %v for unit %q",
				unitRange, unitName,
			)
		}
		for _, unitRange := range validRanges {
			if err := machineRange.CheckConflicts(unitRange); err != nil {
				if unitName == unit.Name() {
					// The same unit has opened the same ports multiple times.
					// That's OK - just skip the duplicates.
					upgradesLogger.Debugf("ignoring conflict (%v) for same unit %q", err, unit)
					skippedRanges++
					rangesToMigrate[unitRange] = false
					continue
				}
				upgradesLogger.Warningf(
					"cannot migrate unit %q's ports %v: %v (skipping)",
					unit, unitRange, err,
				)
				skippedRanges++
				rangesToMigrate[unitRange] = false
				continue
			}
			shouldMigrate, exists := rangesToMigrate[unitRange]
			if shouldMigrate || !exists {
				// It's only OK to migrate if it wasn't skipped
				// earlier.
				rangesToMigrate[unitRange] = true
			}
		}
	}
	if len(allMachineRanges) == 0 {
		// No existing machine ranges, so use all valid unit
		// ranges.
		for _, validRange := range validRanges {
			rangesToMigrate[validRange] = true
		}
	}
	upgradesLogger.Debugf("unit %q port ranges to migrate: %v", unit, rangesToMigrate)
	return rangesToMigrate, skippedRanges, nil
}

func finishUnitMigrationOps(
	unit *Unit,
	rangesToMigrate map[PortRange]bool,
	portsId string,
	opsSoFar []txn.Op,
) (
	migratedPorts, migratedRanges int,
	ops []txn.Op,
) {
	ops = append([]txn.Op(nil), opsSoFar...)

	// Prepare ops for all ranges good to migrate.
	migratedPorts = 0
	migratedRanges = 0
	for portRange, shouldMigrate := range rangesToMigrate {
		if !shouldMigrate {
			continue
		}
		ops = append(ops, txn.Op{
			C:      openedPortsC,
			Id:     portsId,
			Update: bson.D{{"$addToSet", bson.D{{"ports", portRange}}}},
		})
		migratedPorts += portRange.Length()
		migratedRanges++
	}

	// Delete any remainging ports on the unit.
	ops = append(ops, txn.Op{
		C:      unitsC,
		Id:     unit.doc.DocID,
		Assert: txn.DocExists,
		Update: bson.D{{"$unset", bson.D{{"ports", nil}}}},
	})

	return migratedPorts, migratedRanges, ops
}

// MigrateUnitPortsToOpenedPorts loops through all units stored in state and
// migrates any ports into the openedPorts collection.
func MigrateUnitPortsToOpenedPorts(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return errors.Trace(err)
	}

	var unitSlice []unitDoc
	units, closer := st.getRawCollection(unitsC)
	defer closer()

	// Get all units ordered by their service and name.
	err = units.Find(nil).Sort("service", "name").All(&unitSlice)
	if err != nil {
		return errors.Trace(err)
	}

	upgradesLogger.Infof("migrating legacy ports to port ranges for all %d units", len(unitSlice))
	for _, uDoc := range unitSlice {
		unit := &Unit{st: st, doc: uDoc}
		upgradesLogger.Infof("migrating ports for unit %q", unit)
		upgradesLogger.Debugf("raw ports for unit %q: %v", unit, uDoc.Ports)

		skippedRanges, mergedRanges, validRanges := validateUnitPorts(st, unit)

		// Get the unit's assigned machine.
		machineId, err := unit.AssignedMachineId()
		if errors.IsNotAssigned(err) {
			upgradesLogger.Infof("unit %q has no assigned machine; skipping migration", unit)
			continue
		} else if err != nil {
			return errors.Annotatef(err, "cannot get the assigned machine for unit %q", unit)
		}
		upgradesLogger.Debugf("unit %q assigned to machine %q", unit, machineId)

		ops, machinePorts, err := beginUnitMigrationOps(st, unit, machineId)
		if err != nil {
			return errors.Trace(err)
		}

		// Get all existing port ranges on the machine.
		allMachineRanges := machinePorts.AllPortRanges()
		upgradesLogger.Debugf(
			"existing port ranges for unit %q's machine %q: %v",
			unit.Name(), machineId, allMachineRanges,
		)

		rangesToMigrate, filteredRanges, err := filterUnitRangesToMigrate(unit, allMachineRanges, validRanges)
		if err != nil {
			return errors.Trace(err)
		}
		skippedRanges += filteredRanges

		migratedPorts, migratedRanges, ops := finishUnitMigrationOps(
			unit, rangesToMigrate, machinePorts.GlobalKey(), ops,
		)

		if err = st.runRawTransaction(ops); err != nil {
			upgradesLogger.Warningf("migration failed for unit %q: %v", unit, err)
		}

		if len(uDoc.Ports) > 0 {
			totalPorts := len(uDoc.Ports)
			upgradesLogger.Infof(
				"unit %q's ports (ranges) migrated: total %d(%d); ok %d(%d); skipped %d(%d)",
				unit,
				totalPorts, len(mergedRanges),
				migratedPorts, migratedRanges,
				totalPorts-migratedPorts, skippedRanges,
			)
		} else {
			upgradesLogger.Infof("no ports to migrate for unit %q", unit)
		}
	}
	upgradesLogger.Infof("legacy unit ports migrated to machine port ranges")

	return nil
}

// CreateUnitMeterStatus creates documents in the meter status collection for all existing units.
func CreateUnitMeterStatus(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return errors.Trace(err)
	}

	var unitSlice []unitDoc
	units, closer := st.getRawCollection(unitsC)
	defer closer()

	meterStatuses, closer := st.getRawCollection(meterStatusC)
	defer closer()

	// Get all units ordered by their service and name.
	err = units.Find(nil).Sort("service", "_id").All(&unitSlice)
	if err != nil {
		return errors.Trace(err)
	}

	upgradesLogger.Infof("creating meter status entries for all %d units", len(unitSlice))
	for _, uDoc := range unitSlice {
		unit := &Unit{st: st, doc: uDoc}
		upgradesLogger.Infof("creating meter status doc for unit %q", unit)
		cnt, err := meterStatuses.FindId(unit.globalKey()).Count()
		if err != nil {
			return errors.Trace(err)
		}
		if cnt == 1 {
			upgradesLogger.Infof("meter status doc already exists for unit %q", unit)
			continue
		}
		ops := []txn.Op{
			createMeterStatusOp(st, unit.globalMeterStatusKey(), &meterStatusDoc{Code: MeterNotSet.String()}),
		}
		if err = st.runRawTransaction(ops); err != nil {
			upgradesLogger.Warningf("migration failed for unit %q: %v", unit, err)
		}
	}
	upgradesLogger.Infof("meter status docs created for all units")
	return nil
}

// AddEnvironmentUUIDToStateServerDoc adds environment uuid to state server doc.
func AddEnvironmentUUIDToStateServerDoc(st *State) error {
	env, err := st.Environment()
	if err != nil {
		return errors.Annotate(err, "failed to load environment")
	}
	upgradesLogger.Debugf("adding env uuid %q", env.UUID())

	ops := []txn.Op{{
		C:      stateServersC,
		Id:     environGlobalKey,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"env-uuid", env.UUID()},
		}}},
	}}

	return st.runRawTransaction(ops)
}

// AddEnvUUIDToEnvUsersDoc adds environment uuid to state server doc.
func AddEnvUUIDToEnvUsersDoc(st *State) error {
	envUsers, closer := st.getRawCollection(envUsersC)
	defer closer()

	var ops []txn.Op
	var doc bson.M
	iter := envUsers.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {

		if _, ok := doc["env-uuid"]; !ok || doc["env-uuid"] == "" {
			ops = append(ops, txn.Op{
				C:      envUsersC,
				Id:     doc["_id"],
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"env-uuid", doc["envuuid"]}}},
					{"$unset", bson.D{{"envuuid", nil}}},
				},
			})
		}
	}
	if err := iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// AddLifeFieldOfIPAddresses creates the Life field for all IP addresses
// that don't have this field. For addresses referencing live machines Life
// will be set to Alive, otherwise Life will be set to Dead.
func AddLifeFieldOfIPAddresses(st *State) error {
	addresses, iCloser := st.getCollection(ipaddressesC)
	defer iCloser()
	machines, mCloser := st.getCollection(machinesC)
	defer mCloser()

	var ops []txn.Op
	var address bson.M
	iter := addresses.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&address) {
		// if the address already has a Life field, then it has already been
		// upgraded.
		if _, ok := address["life"]; ok {
			continue
		}

		life := Alive
		allocatedState, ok := address["state"]

		var addressAllocated bool
		// if state was missing, we pretend the IP address is
		// unallocated. State can't be empty anyway, so this shouldn't
		// happen.
		if ok && allocatedState == string(AddressStateAllocated) {
			addressAllocated = true
		}

		// An IP address that has an allocated state but no machine ID
		// shouldn't be possible.
		if machineId, ok := address["machineid"]; addressAllocated && ok && machineId != "" {
			mDoc := &machineDoc{}
			err := machines.Find(bson.D{{"machineid", machineId}}).One(&mDoc)
			if err != nil || mDoc.Life != Alive {
				life = Dead
			}
		}
		logger.Debugf("setting life %q to address %q", life, address["value"])

		ops = append(ops, txn.Op{
			C:  ipaddressesC,
			Id: address["_id"],
			Update: bson.D{{"$set", bson.D{
				{"life", life},
			}}},
		})
		address = nil
	}
	if err := iter.Err(); err != nil {
		logger.Errorf("failed fetching IP addresses: %v", err)
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// AddInstanceIdFieldOfIPAddresses creates and populates the instance Id field
// for all IP addresses referencing a live machine with a provisioned instance.
func AddInstanceIdFieldOfIPAddresses(st *State) error {
	addresses, iCloser := st.getCollection(ipaddressesC)
	defer iCloser()
	instances, mCloser := st.getCollection(instanceDataC)
	defer mCloser()

	var ops []txn.Op
	var address bson.M
	iter := addresses.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&address) {
		// if the address already has a instance Id field, then it has already been
		// upgraded.
		logger.Tracef("AddInstanceField: processing address %s", address["value"])
		if _, ok := address["instanceid"]; ok {
			logger.Tracef("skipping address %s, already has instance id", address["value"])
			continue
		}

		fetchId := func(machineId interface{}) instance.Id {
			instanceId := instance.UnknownId
			iDoc := &instanceData{}
			err := instances.Find(bson.D{{"machineid", machineId}}).One(&iDoc)
			if err != nil {
				logger.Debugf("failed to find machine for address %s: %s", address["value"], err)
			} else {
				instanceId = instance.Id(iDoc.InstanceId)
				logger.Debugf("found instance id %q for address %s", instanceId, address["value"])
			}
			return instanceId
		}

		instanceId := instance.UnknownId
		allocatedState, ok := address["state"]
		// An unallocated address can't have an associated instance id.
		if ok && allocatedState == string(AddressStateAllocated) {
			if machineId, ok := address["machineid"]; ok && machineId != "" {
				instanceId = fetchId(machineId)
			} else {
				logger.Debugf("machine id not found for address %s", address["value"])
			}
		} else {
			logger.Debugf("address %s not allocated, setting unknown ID", address["value"])
		}
		logger.Debugf("setting instance id of %s to %q", address["value"], instanceId)

		ops = append(ops, txn.Op{
			C:  ipaddressesC,
			Id: address["_id"],
			Update: bson.D{{"$set", bson.D{
				{"instanceid", instanceId},
			}}},
		})
		address = nil
	}
	if err := iter.Err(); err != nil {
		logger.Errorf("failed fetching IP addresses: %v", err)
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// AddUUIDToIPAddresses creates and populates the UUID field
// for all IP addresses.
func AddUUIDToIPAddresses(st *State) error {
	addresses, iCloser := st.getCollection(ipaddressesC)
	defer iCloser()

	var ops []txn.Op
	var address bson.M
	iter := addresses.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&address) {
		logger.Tracef("AddUUIDToIPAddresses: processing address %s", address["value"])

		// Skip addresses which already have the UUID field.
		if _, ok := address["uuid"]; ok {
			logger.Tracef("skipping address %s, already has uuid", address["value"])
			continue
		}

		// Generate new UUID.
		uuid, err := utils.NewUUID()
		if err != nil {
			logger.Errorf("failed generating UUID for IP address: %v", err)
			return errors.Trace(err)
		}

		// Add op for setting the UUID.
		ops = append(ops, txn.Op{
			C:      ipaddressesC,
			Id:     address["_id"],
			Update: bson.D{{"$set", bson.D{{"uuid", uuid.String()}}}},
		})
		address = nil
	}
	if err := iter.Err(); err != nil {
		logger.Errorf("failed fetching IP addresses: %v", err)
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

func AddNameFieldLowerCaseIdOfUsers(st *State) error {
	users, closer := st.getCollection(usersC)
	defer closer()

	var ops []txn.Op
	var user bson.M
	iter := users.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&user) {
		// if the user already has a name field, then it has already been
		// upgraded.
		if name, ok := user["name"]; ok && name != "" {
			continue
		}

		// set name to old, case sensitive, _id and lowercase new _id.
		user["name"], user["_id"] = user["_id"], strings.ToLower(user["_id"].(string))

		if user["name"] != user["_id"] {
			// if we need to update the _id, remove old doc and add a new one with
			// lowercased _id.
			ops = append(ops, txn.Op{
				C:      usersC,
				Id:     user["name"],
				Remove: true,
			}, txn.Op{
				C:      usersC,
				Id:     user["_id"],
				Assert: txn.DocMissing,
				Insert: user,
			})
		} else {
			// otherwise, just update the name field.
			ops = append(ops, txn.Op{
				C:  usersC,
				Id: user["_id"],
				Update: bson.D{{"$set", bson.D{
					{"name", user["name"]},
				}}},
			})
		}
		user = nil
	}
	if err := iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

func AddPreferredAddressesToMachines(st *State) error {
	machines, err := st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}

	for _, machine := range machines {
		if machine.Life() == Dead {
			continue
		}
		// Setting the addresses is enough to trigger setting the preferred
		// addresses.
		err = machine.SetMachineAddresses(machine.MachineAddresses()...)
		if err != nil {
			return errors.Trace(err)
		}
		err := machine.SetProviderAddresses(machine.ProviderAddresses()...)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func LowerCaseEnvUsersID(st *State) error {
	users, closer := st.getRawCollection(envUsersC)
	defer closer()

	var ops []txn.Op
	var user bson.M
	iter := users.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&user) {
		oldId := user["_id"].(string)
		newId := strings.ToLower(oldId)

		if oldId != newId {
			user["_id"] = newId
			ops = append(ops, txn.Op{
				C:      envUsersC,
				Id:     oldId,
				Remove: true,
			}, txn.Op{
				C:      envUsersC,
				Id:     user["_id"],
				Assert: txn.DocMissing,
				Insert: user,
			})
		}
		user = nil
	}
	if err := iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

func AddUniqueOwnerEnvNameForEnvirons(st *State) error {
	environs, closer := st.getCollection(environmentsC)
	defer closer()

	ownerEnvNameMap := map[string]set.Strings{}
	ensureDoesNotExist := func(owner, envName string) error {
		if _, ok := ownerEnvNameMap[owner]; ok {
			if ownerEnvNameMap[owner].Contains(envName) {
				return errors.AlreadyExistsf("environment %q for %s", envName, owner)
			}
			ownerEnvNameMap[owner].Add(envName)
		} else {
			ownerEnvNameMap[owner] = set.NewStrings(envName)
		}
		return nil
	}

	var ops []txn.Op
	var env environmentDoc
	iter := environs.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&env) {
		if err := ensureDoesNotExist(env.Owner, env.Name); err != nil {
			return err
		}

		ops = append(ops, txn.Op{
			C:      userenvnameC,
			Id:     userEnvNameIndex(env.Owner, env.Name),
			Insert: bson.M{},
		})
	}
	if err := iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// AddCharmStoragePaths adds storagepath fields
// to the specified charms.
func AddCharmStoragePaths(st *State, storagePaths map[*charm.URL]string) error {
	var ops []txn.Op
	for curl, storagePath := range storagePaths {
		upgradesLogger.Debugf("adding storage path %q to %s", storagePath, curl)
		op := txn.Op{
			C:      charmsC,
			Id:     st.docID(curl.String()),
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"storagepath", storagePath}}},
				{"$unset", bson.D{{"bundleurl", nil}}},
			},
		}
		ops = append(ops, op)
	}
	err := st.runRawTransaction(ops)
	if err == txn.ErrAborted {
		return errors.NotFoundf("charms")
	}
	return err
}

// SetOwnerAndServerUUIDForEnvironment adds the environment uuid as the server
// uuid as well (it is the initial environment, so all good), and the owner to
// "admin@local", again all good as all existing environments have a user
// called "admin".
func SetOwnerAndServerUUIDForEnvironment(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	env, err := st.Environment()
	if err != nil {
		return errors.Annotate(err, "failed to load environment")
	}
	owner := names.NewLocalUserTag("admin")
	ops := []txn.Op{{
		C:      environmentsC,
		Id:     env.UUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{
			{"server-uuid", env.UUID()},
			{"owner", owner.Canonical()},
		}}},
	}}
	return st.runRawTransaction(ops)
}

// MigrateMachineInstanceIdToInstanceData migrates the deprecated "instanceid"
// machine field into "instanceid" in the instanceData doc.
func MigrateMachineInstanceIdToInstanceData(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return errors.Trace(err)
	}

	instDatas, closer := st.getRawCollection(instanceDataC)
	defer closer()
	machines, closer := st.getRawCollection(machinesC)
	defer closer()

	var ops []txn.Op
	var doc bson.M
	iter := machines.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		var instID interface{}
		mID := doc["_id"].(string)
		i, err := instDatas.FindId(mID).Count()
		if err != nil {
			return errors.Trace(err)
		}
		if i == 0 {
			var ok bool
			if instID, ok = doc["instanceid"]; !ok || instID == "" {
				upgradesLogger.Warningf("machine %q doc has no instanceid", mID)
				continue
			}

			// Insert instanceData doc.
			ops = append(ops, txn.Op{
				C:      instanceDataC,
				Id:     mID,
				Assert: txn.DocMissing,
				Insert: instanceData{
					DocID:      mID,
					MachineId:  mID,
					EnvUUID:    st.EnvironUUID(),
					InstanceId: instance.Id(instID.(string)),
				},
			})
		}

		// Remove instanceid field from machine doc.
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     mID,
			Assert: txn.DocExists,
			Update: bson.D{
				{"$unset", bson.D{{"instanceid", nil}}},
			},
		})
	}
	if err = iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// AddAvailabilityZoneToInstanceData sets the AvailZone field on
// instanceData docs that don't have it already.
func AddAvailabilityZoneToInstanceData(st *State, azFunc func(*State, instance.Id) (string, error)) error {
	err := st.ResumeTransactions()
	if err != nil {
		return errors.Trace(err)
	}

	instDatas, closer := st.getRawCollection(instanceDataC)
	defer closer()

	var ops []txn.Op
	// Using bson.M instead of a struct is important because we need to
	// know if the "availzone" key is set on the raw doc.
	var doc bson.M
	iter := instDatas.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		zone, ok := doc["availzone"]
		if ok || ParentId(doc["machineid"].(string)) != "" {
			continue
		}

		zone, err := azFunc(st, instance.Id(doc["instanceid"].(string)))
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}

			if !errors.IsNotSupported(err) {
				return errors.Trace(err)
			}
			zone = ""
		}

		// Set AvailZone.
		ops = append(ops, txn.Op{
			C:      instanceDataC,
			Id:     doc["_id"].(string),
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"availzone", zone}}},
			},
		})
	}
	if err = iter.Err(); err != nil {
		return errors.Trace(err)
	}
	err = st.runTransaction(ops)
	return errors.Trace(err)
}

// AddEnvUUIDToServices prepends the environment UUID to the ID of
// all service docs and adds new "env-uuid" field.
func AddEnvUUIDToServices(st *State) error {
	return addEnvUUIDToEntityCollection(st, servicesC, setOldID("name"))
}

// AddEnvUUIDToUnits prepends the environment UUID to the ID of all
// unit docs and adds new "env-uuid" field.
func AddEnvUUIDToUnits(st *State) error {
	return addEnvUUIDToEntityCollection(st, unitsC, setOldID("name"))
}

// AddEnvUUIDToMachines prepends the environment UUID to the ID of
// all machine docs and adds new "env-uuid" field.
func AddEnvUUIDToMachines(st *State) error {
	return addEnvUUIDToEntityCollection(st, machinesC, setOldID("machineid"))
}

// AddEnvUUIDToOpenPorts prepends the environment UUID to the ID of
// all openPorts docs and adds new "env-uuid" field.
func AddEnvUUIDToOpenPorts(st *State) error {
	setNewFields := func(d bson.D) (bson.D, error) {
		id, err := readBsonDField(d, "_id")
		if err != nil {
			return nil, errors.Trace(err)
		}
		parts, err := extractPortsIdParts(id.(string))
		if err != nil {
			return nil, errors.Trace(err)
		}
		d, err = addBsonDField(d, "machine-id", parts[machineIdPart])
		if err != nil {
			return nil, errors.Trace(err)
		}
		d, err = addBsonDField(d, "network-name", parts[networkNamePart])
		if err != nil {
			return nil, errors.Trace(err)
		}
		return d, nil
	}
	return addEnvUUIDToEntityCollection(st, openedPortsC, setNewFields)
}

// AddEnvUUIDToAnnotations prepends the environment UUID to the ID of
// all annotation docs and adds new "env-uuid" field.
func AddEnvUUIDToAnnotations(st *State) error {
	return addEnvUUIDToEntityCollection(st, annotationsC, setOldID("globalkey"))
}

// AddEnvUUIDToStatuses prepends the environment UUID to the ID of
// all Statuses docs and adds new "env-uuid" field.
func AddEnvUUIDToStatuses(st *State) error {
	return addEnvUUIDToEntityCollection(st, statusesC)
}

// AddEnvUUIDToNetworks prepends the environment UUID to the ID of
// all network docs and adds new "env-uuid" field.
func AddEnvUUIDToNetworks(st *State) error {
	return addEnvUUIDToEntityCollection(st, networksC, setOldID("name"))
}

// AddEnvUUIDToRequestedNetworks prepends the environment UUID to the ID of
// all requestedNetworks docs and adds new "env-uuid" field.
func AddEnvUUIDToRequestedNetworks(st *State) error {
	return addEnvUUIDToEntityCollection(st, requestedNetworksC, setOldID("requestednetworkid"))
}

// AddEnvUUIDToNetworkInterfaces prepends adds a new "env-uuid" field to all
// networkInterfaces docs.
func AddEnvUUIDToNetworkInterfaces(st *State) error {
	coll, closer := st.getRawCollection(networkInterfacesC)
	defer closer()

	upgradesLogger.Debugf("adding the env uuid %q to the %s collection", st.EnvironUUID(), networkInterfacesC)
	iter := coll.Find(bson.D{{"env-uuid", bson.D{{"$exists", false}}}}).Iter()

	var doc bson.M
	ops := []txn.Op{}
	for iter.Next(&doc) {
		ops = append(ops,
			txn.Op{
				C:      networkInterfacesC,
				Id:     doc["_id"],
				Assert: txn.DocExists,
				Update: bson.D{{"$set", bson.D{{"env-uuid", st.EnvironUUID()}}}},
			})
		doc = nil // Force creation of new map for the next iteration
	}
	if err := iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// AddEnvUUIDToCharms prepends the environment UUID to the ID of
// all charm docs and adds new "env-uuid" field.
func AddEnvUUIDToCharms(st *State) error {
	return addEnvUUIDToEntityCollection(st, charmsC, setOldID("url"))
}

// AddEnvUUIDToMinUnits prepends the environment UUID to the ID of
// all minUnits docs and adds new "env-uuid" field.
func AddEnvUUIDToMinUnits(st *State) error {
	return addEnvUUIDToEntityCollection(st, minUnitsC, setOldID("servicename"))
}

// AddEnvUUIDToSequences prepends the environment UUID to the ID of
// all sequence docs and adds new "env-uuid" field.
func AddEnvUUIDToSequences(st *State) error {
	return addEnvUUIDToEntityCollection(st, sequenceC, setOldID("name"))
}

// AddEnvUUIDToReboots prepends the environment UUID to the ID of
// all reboot docs and adds new "env-uuid" field.
func AddEnvUUIDToReboots(st *State) error {
	return addEnvUUIDToEntityCollection(st, rebootC, setOldID("machineid"))
}

// AddEnvUUIDToContainerRefs prepends the environment UUID to the ID of all
// containerRef docs and adds new "env-uuid" field.
func AddEnvUUIDToContainerRefs(st *State) error {
	return addEnvUUIDToEntityCollection(st, containerRefsC, setOldID("machineid"))
}

// AddEnvUUIDToInstanceData prepends the environment UUID to the ID of
// all instanceData docs and adds new "env-uuid" field.
func AddEnvUUIDToInstanceData(st *State) error {
	return addEnvUUIDToEntityCollection(st, instanceDataC, setOldID("machineid"))
}

// AddEnvUUIDToCleanups prepends the environment UUID to the ID of
// all cleanup docs and adds new "env-uuid" field.
func AddEnvUUIDToCleanups(st *State) error {
	return addEnvUUIDToEntityCollection(st, cleanupsC)
}

// AddEnvUUIDToConstraints prepends the environment UUID to the ID of
// all constraints docs and adds new "env-uuid" field.
func AddEnvUUIDToConstraints(st *State) error {
	return addEnvUUIDToEntityCollection(st, constraintsC)
}

// AddEnvUUIDToSettings prepends the environment UUID to the ID of
// all settings docs and adds new "env-uuid" field.
func AddEnvUUIDToSettings(st *State) error {
	return addEnvUUIDToEntityCollection(st, settingsC)
}

// AddEnvUUIDToSettingsRefs prepends the environment UUID to the ID of
// all settingRef docs and adds new "env-uuid" field.
func AddEnvUUIDToSettingsRefs(st *State) error {
	return addEnvUUIDToEntityCollection(st, settingsrefsC)
}

// AddEnvUUIDToRelations prepends the environment UUID to the ID of
// all relations docs and adds new "env-uuid" and "key" fields.
func AddEnvUUIDToRelations(st *State) error {
	return addEnvUUIDToEntityCollection(st, relationsC, setOldID("key"))
}

// AddEnvUUIDToRelationScopes prepends the environment UUID to the ID of
// all relationscopes docs and adds new "env-uuid" field and "key" fields.
func AddEnvUUIDToRelationScopes(st *State) error {
	return addEnvUUIDToEntityCollection(st, relationScopesC, setOldID("key"))
}

// AddEnvUUIDToMeterStatus prepends the environment UUID to the ID of
// all meterStatus docs and adds new "env-uuid" field and "id" fields.
func AddEnvUUIDToMeterStatus(st *State) error {
	return addEnvUUIDToEntityCollection(st, meterStatusC)
}

func addEnvUUIDToEntityCollection(st *State, collName string, updates ...updateFunc) error {
	env, err := st.Environment()
	if err != nil {
		return errors.Annotate(err, "failed to load environment")
	}

	coll, closer := st.getRawCollection(collName)
	defer closer()

	upgradesLogger.Debugf("adding the env uuid %q to the %s collection", env.UUID(), collName)
	uuid := env.UUID()
	iter := coll.Find(bson.D{{"env-uuid", bson.D{{"$exists", false}}}}).Iter()
	defer iter.Close()
	ops := []txn.Op{}
	var doc bson.D
	for iter.Next(&doc) {
		oldID, err := readBsonDField(doc, "_id")
		if err != nil {
			return errors.Trace(err)
		}
		newID := st.docID(fmt.Sprint(oldID))

		if oldID == newID {
			// The _id already has the env UUID prefix. This shouldn't
			// really happen, except in the case of bugs, but it
			// should still be handled. Just set the env-uuid field.
			ops = append(ops,
				txn.Op{
					C:      collName,
					Id:     oldID,
					Assert: txn.DocExists,
					Update: bson.M{"$set": bson.M{"env-uuid": uuid}},
				})
		} else {
			// Collection specific updates.
			for _, update := range updates {
				var err error
				doc, err = update(doc)
				if err != nil {
					return errors.Trace(err)
				}
			}

			doc, err = addBsonDField(doc, "env-uuid", uuid)
			if err != nil {
				return errors.Trace(err)
			}

			// Note: there's no need to update _id on the document. Id
			// from the txn.Op takes precedence.

			ops = append(ops,
				[]txn.Op{{
					C:      collName,
					Id:     oldID,
					Assert: txn.DocExists,
					Remove: true,
				}, {
					C:      collName,
					Id:     newID,
					Assert: txn.DocMissing,
					Insert: doc,
				}}...)
		}
		doc = nil // Force creation of new doc for the next iteration
	}
	if err = iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

// readBsonDField returns the value of a given field in a bson.D.
func readBsonDField(d bson.D, name string) (interface{}, error) {
	for i := range d {
		field := &d[i]
		if field.Name == name {
			return field.Value, nil
		}
	}
	return nil, errors.NotFoundf("field %q", name)
}

// addBsonDField adds a new field to the end of a bson.D, returning
// the updated bson.D.
func addBsonDField(d bson.D, name string, value interface{}) (bson.D, error) {
	for i := range d {
		if d[i].Name == name {
			return nil, errors.AlreadyExistsf("field %q", name)
		}
	}
	return append(d, bson.DocElem{
		Name:  name,
		Value: value,
	}), nil
}

type updateFunc func(bson.D) (bson.D, error)

// setOldID returns an updateFunc which populates the doc's original ID
// in the named field.
func setOldID(name string) updateFunc {
	return func(d bson.D) (bson.D, error) {
		oldID, err := readBsonDField(d, "_id")
		if err != nil {
			return nil, errors.Trace(err)
		}
		return addBsonDField(d, name, oldID)
	}
}

// migrateJobManageNetworking adds the job JobManageNetworking to all
// machines except for:
//
// - machines in a MAAS environment,
// - machines in a Joyent environment,
// - machines in a manual environment,
// - bootstrap node (host machine) in a local environment, and
// - manually provisioned machines.
func MigrateJobManageNetworking(st *State) error {
	// Retrieve the provider.
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return errors.Annotate(err, "failed to read current config")
	}
	envType := envConfig.Type()

	// Check for MAAS, Joyent, or manual (aka null) provider.
	if envType == provider.MAAS ||
		envType == provider.Joyent ||
		provider.IsManual(envType) {
		// No job adding for these environment types.
		return nil
	}

	// Iterate over all machines and create operations.
	machinesCollection, closer := st.getRawCollection(machinesC)
	defer closer()

	iter := machinesCollection.Find(nil).Iter()
	defer iter.Close()

	ops := []txn.Op{}
	mdoc := machineDoc{}

	for iter.Next(&mdoc) {
		// Check possible exceptions.
		localID := st.localID(mdoc.Id)
		if localID == "0" && envType == provider.Local {
			// Skip machine 0 in local environment.
			continue
		}
		if strings.HasPrefix(mdoc.Nonce, manualMachinePrefix) {
			// Skip manually provisioned machine in non-manual environments.
			continue
		}
		if hasJob(mdoc.Jobs, JobManageNetworking) {
			// Should not happen during update, but just to
			// prevent double entries.
			continue
		}
		// Everything fine, now add job.
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     mdoc.DocID,
			Update: bson.D{{"$addToSet", bson.D{{"jobs", JobManageNetworking}}}},
		})
	}

	return st.runRawTransaction(ops)
}

// FixMinUnitsEnvUUID sets the env-uuid field on documents in the
// minUnits collection where the field is blank. This is needed
// because a code change was missed with the env UUID migration was
// done for this collection (in 1.21).
func FixMinUnitsEnvUUID(st *State) error {
	minUnits, closer := st.getRawCollection(minUnitsC)
	defer closer()

	iter := minUnits.Find(bson.D{{"env-uuid", ""}}).Select(bson.D{{"_id", 1}}).Iter()
	defer iter.Close()

	uuid := st.EnvironUUID()
	ops := []txn.Op{}
	var doc bson.M
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      minUnitsC,
			Id:     doc["_id"],
			Update: bson.D{{"$set", bson.D{{"env-uuid", uuid}}}},
			Assert: txn.DocExists,
		})
	}
	if err := iter.Err(); err != nil {
		return err
	}
	return st.runRawTransaction(ops)
}

// FixSequenceFields sets the env-uuid and name fields on documents in
// the sequence collection where these fields are blank. This is
// needed because code changes were missed with the env UUID migration
// was done for this collection (in 1.21).
func FixSequenceFields(st *State) error {
	sequence, closer := st.getRawCollection(sequenceC)
	defer closer()

	sel := bson.D{{"$or", []bson.D{
		{{"env-uuid", ""}},
		{{"name", ""}},
	}}}
	iter := sequence.Find(sel).Select(bson.D{{"_id", 1}}).Iter()
	defer iter.Close()

	uuid := st.EnvironUUID()
	ops := []txn.Op{}
	var doc bson.M
	for iter.Next(&doc) {
		docID, ok := doc["_id"].(string)
		if !ok {
			return errors.Errorf("unexpected sequence id: %v", doc["_id"])
		}
		name, err := st.strictLocalID(docID)
		if err != nil {
			return err
		}
		ops = append(ops, txn.Op{
			C:  sequenceC,
			Id: docID,
			Update: bson.D{{"$set", bson.D{
				{"env-uuid", uuid},
				{"name", name},
			}}},
			Assert: txn.DocExists,
		})
	}
	if err := iter.Err(); err != nil {
		return err
	}
	return st.runRawTransaction(ops)
}

// MoveServiceUnitSeqToSequence moves information from unitSeq value
// in the services documents and puts it into a new document in the
// sequence collection.
// The move happens in 3 stages:
// Insert: We insert the new sequence documents based on the values
// in the service collection. Any existing documents with the id we ignore
// Update: We update all the sequence documents with the correct UnitSeq.
// this phase overwrites any existing sequence documents that existed and
// were ignored during the install phase
// Unset: The last phase is to remove the unitseq from the service collection.
func MoveServiceUnitSeqToSequence(st *State) error {
	unitSeqDocs := []struct {
		Name    string `bson:"name"`
		UnitSeq int    `bson:"unitseq"`
	}{}
	servicesCollection, closer := st.getCollection(servicesC)
	defer closer()

	err := servicesCollection.Find(nil).All(&unitSeqDocs)
	if err != nil {
		return errors.Trace(err)
	}
	insertOps := make([]txn.Op, len(unitSeqDocs))
	updateOps := make([]txn.Op, len(unitSeqDocs))
	unsetOps := make([]txn.Op, len(unitSeqDocs))
	for i, svc := range unitSeqDocs {
		tag := names.NewServiceTag(svc.Name)
		insertOps[i] = txn.Op{
			C:  sequenceC,
			Id: st.docID(tag.String()),
			Insert: sequenceDoc{
				Name:    tag.String(),
				EnvUUID: st.EnvironUUID(),
				Counter: svc.UnitSeq,
			},
		}
		updateOps[i] = txn.Op{
			C:  sequenceC,
			Id: st.docID(tag.String()),
			Update: bson.M{
				"$set": bson.M{
					"name":     tag.String(),
					"env-uuid": st.EnvironUUID(),
					"counter":  svc.UnitSeq,
				},
			},
		}
		unsetOps[i] = txn.Op{
			C:  servicesC,
			Id: st.docID(svc.Name),
			Update: bson.M{
				"$unset": bson.M{
					"unitseq": 0},
			},
		}
	}
	ops := append(insertOps, updateOps...)
	ops = append(ops, unsetOps...)
	return st.runRawTransaction(ops)
}

// DropOldIndexesv123 drops old mongo indexes.
func DropOldIndexesv123(st *State) error {
	for collName, indexes := range oldIndexesv123 {
		c, closer := st.getRawCollection(collName)
		defer closer()
		for _, index := range indexes {
			if err := c.DropIndex(index...); err != nil {
				format := "error while dropping index: %s"
				// Failing to drop an index that does not exist does not
				// warrant raising an error.
				if err.Error() == "index not found" {
					upgradesLogger.Infof(format, err.Error())
				} else {
					upgradesLogger.Errorf(format, err.Error())
				}
			}
		}
	}
	return nil
}

var oldIndexesv123 = map[string][][]string{
	relationsC: {
		{"endpoints.relationname"},
		{"endpoints.servicename"},
	},
	unitsC: {
		{"service"},
		{"principal"},
		{"machineid"},
	},
	networksC: {
		{"providerid"},
	},
	networkInterfacesC: {
		{"interfacename", "machineid"},
		{"macaddress", "networkname"},
		{"networkname"},
		{"machineid"},
	},
	blockDevicesC: {
		{"machineid"},
	},
	subnetsC: {
		{"providerid"},
	},
	ipaddressesC: {
		{"state"},
		{"subnetid"},
	},
}

// AddLeadsershipSettingsDocs creates service leadership documents in
// the settings collection for all services in all environments.
func AddLeadershipSettingsDocs(st *State) error {
	environments, closer := st.getCollection(environmentsC)
	defer closer()

	var envDocs []bson.M
	err := environments.Find(nil).Select(bson.M{"_id": 1}).All(&envDocs)
	if err != nil {
		return errors.Annotate(err, "failed to read environments")
	}

	for _, envDoc := range envDocs {
		envUUID := envDoc["_id"].(string)
		envSt, err := st.ForEnviron(names.NewEnvironTag(envUUID))
		if err != nil {
			return errors.Annotatef(err, "failed to open environment %q", envUUID)
		}
		defer envSt.Close()

		services, err := envSt.AllServices()
		if err != nil {
			return errors.Annotatef(err, "failed to retrieve services for environment %q", envUUID)
		}

		for _, service := range services {
			// The error from this is intentionally ignored as the
			// transaction will fail if the service already has a
			// leadership settings doc.
			envSt.runTransaction([]txn.Op{
				addLeadershipSettingsOp(service.Name()),
			})
		}
	}
	return nil
}

// AddDefaultBlockDevicesDocs creates block devices documents
// for all existing machines in all environments.
func AddDefaultBlockDevicesDocs(st *State) error {
	environments, closer := st.getCollection(environmentsC)
	defer closer()

	var envDocs []bson.M
	err := environments.Find(nil).Select(bson.M{"_id": 1}).All(&envDocs)
	if err != nil {
		return errors.Annotate(err, "failed to read environments")
	}

	for _, envDoc := range envDocs {
		envUUID := envDoc["_id"].(string)
		envSt, err := st.ForEnviron(names.NewEnvironTag(envUUID))
		if err != nil {
			return errors.Annotatef(err, "failed to open environment %q", envUUID)
		}
		defer envSt.Close()

		machines, err := envSt.AllMachines()
		if err != nil {
			return errors.Annotatef(err, "failed to retrieve machines for environment %q", envUUID)
		}

		for _, machine := range machines {
			// If a txn fails because the doc already exists, that's ok.
			if err := envSt.runTransaction([]txn.Op{
				createMachineBlockDevicesOp(machine.Id()),
			}); err != nil && err != txn.ErrAborted {
				return err
			}
		}
	}
	return nil
}

// SetHostedEnvironCount is an upgrade step that sets hostedEnvCountDoc.Count
// to the number of hosted environments.
func SetHostedEnvironCount(st *State) error {
	environments, closer := st.getCollection(environmentsC)
	defer closer()

	envCount, err := environments.Find(nil).Count()
	if err != nil {
		return errors.Annotate(err, "failed to read environments")
	}

	stateServers, closer := st.getCollection(stateServersC)
	defer closer()

	count, err := stateServers.FindId(hostedEnvCountKey).Count()
	if err != nil {
		return errors.Annotate(err, "failed to read state server")
	}

	hostedCount := envCount - 1 // -1 as we don't count the system environment
	op := txn.Op{
		C:  stateServersC,
		Id: hostedEnvCountKey,
	}
	if count == 0 {
		op.Assert = txn.DocMissing
		op.Insert = &hostedEnvCountDoc{hostedCount}
	} else {
		op.Update = bson.D{{"$set", bson.D{{"refcount", hostedCount}}}}
	}

	return st.runTransaction([]txn.Op{op})
}

type oldUserDoc struct {
	DocID     string     `bson:"_id"`
	EnvUUID   string     `bson:"env-uuid"`
	LastLogin *time.Time `bson:"lastlogin"`
}

type oldEnvUserDoc struct {
	DocID          string     `bson:"_id"`
	EnvUUID        string     `bson:"env-uuid"`
	UserName       string     `bson:"user"`
	LastConnection *time.Time `bson:"lastconnection"`
}

// MigrateLastLoginAndLastConnection is an upgrade step that separates out
// LastLogin from the userDoc into its own collection and removes the
// lastlogin field from the userDoc. It does the same for LastConnection in
// the envUserDoc.
func MigrateLastLoginAndLastConnection(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	// 0. setup
	users, closer := st.getRawCollection(usersC)
	defer closer()
	envUsers, closer := st.getRawCollection(envUsersC)
	defer closer()
	userLastLogins, closer := st.getRawCollection(userLastLoginC)
	defer closer()
	envUserLastConnections, closer := st.getRawCollection(envUserLastConnectionC)
	defer closer()

	var oldUserDocs []oldUserDoc
	if err = users.Find(bson.D{{
		"lastlogin", bson.D{{"$exists", true}}}}).All(&oldUserDocs); err != nil {
		return err
	}

	var oldEnvUserDocs []oldEnvUserDoc
	if err = envUsers.Find(bson.D{{
		"lastconnection", bson.D{{"$exists", true}}}}).All(&oldEnvUserDocs); err != nil {
		return err
	}

	// 1. collect data we need to move
	var lastLoginDocs []interface{}
	var lastConnectionDocs []interface{}

	for _, oldUser := range oldUserDocs {
		if oldUser.LastLogin == nil {
			continue
		}
		lastLoginDocs = append(lastLoginDocs, userLastLoginDoc{
			oldUser.DocID,
			oldUser.EnvUUID,
			*oldUser.LastLogin,
		})
	}

	for _, oldEnvUser := range oldEnvUserDocs {
		if oldEnvUser.LastConnection == nil {
			continue
		}
		lastConnectionDocs = append(lastConnectionDocs, envUserLastConnectionDoc{
			oldEnvUser.DocID,
			oldEnvUser.EnvUUID,
			oldEnvUser.UserName,
			*oldEnvUser.LastConnection,
		})
	}

	// 2. raw-write all that data to the new collections, overwriting
	// everything.
	//
	// If a user accesses the API during the upgrade, a lastLoginDoc could
	// already exist. In this is the case, we hit a duplicate key error, which
	// we ignore. The insert becomes a no-op, keeping the new lastLoginDoc
	// which will be more up-to-date than what's read in through the usersC
	// collection.
	for _, lastLoginDoc := range lastLoginDocs {
		if err := userLastLogins.Insert(lastLoginDoc); err != nil && !mgo.IsDup(err) {
			id := lastLoginDoc.(userLastLoginDoc).DocID
			logger.Debugf("failed to insert userLastLoginDoc with id %q. Got error: %v", id, err)
			return err
		}
	}

	for _, lastConnectionDoc := range lastConnectionDocs {
		if err := envUserLastConnections.Insert(lastConnectionDoc); err != nil && !mgo.IsDup(err) {
			id := lastConnectionDoc.(envUserLastConnectionDoc).ID
			logger.Debugf("failed to insert envUserLastConnectionDoc with id %q. Got error: %v", id, err)
			return err
		}
	}

	// 3. run txn operations to remove the old unwanted fields
	ops := []txn.Op{}

	for _, oldUser := range oldUserDocs {
		upgradesLogger.Debugf("updating lastlogin for user %q", oldUser.DocID)
		ops = append(ops,
			txn.Op{
				C:      usersC,
				Id:     oldUser.DocID,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$unset", bson.D{{"lastlogin", nil}}},
				},
			})
	}

	for _, oldEnvUser := range oldEnvUserDocs {
		upgradesLogger.Debugf("updating lastconnection for environment user %q", oldEnvUser.DocID)

		ops = append(ops,
			txn.Op{
				C:      envUsersC,
				Id:     oldEnvUser.DocID,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$unset", bson.D{{"lastconnection", nil}}},
				},
			})
	}
	return st.runRawTransaction(ops)
}

// AddMissingEnvUUIDOnStatuses populates the env-uuid field where it
// is missing due to LP #1474606.
func AddMissingEnvUUIDOnStatuses(st *State) error {
	statuses, closer := st.getRawCollection(statusesC)
	defer closer()

	sel := bson.M{"$or": []bson.M{
		{"env-uuid": bson.M{"$exists": false}},
		{"env-uuid": ""},
	}}
	var docs []bson.M
	err := statuses.Find(sel).Select(bson.M{"_id": 1}).All(&docs)
	if err != nil {
		return errors.Annotate(err, "failed to read statuses")
	}

	var ops []txn.Op
	for _, doc := range docs {
		id, ok := doc["_id"].(string)
		if !ok {
			return errors.Errorf("unexpected id: %v", doc["_id"])
		}

		idParts := strings.SplitN(id, ":", 2)
		if len(idParts) != 2 {
			return errors.Errorf("unexpected id format: %v", id)
		}

		ops = append(ops, txn.Op{
			C:      statusesC,
			Id:     id,
			Assert: txn.DocExists,
			Update: bson.M{"$set": bson.M{"env-uuid": idParts[0]}},
		})
	}

	if err := st.runRawTransaction(ops); err != nil {
		return errors.Annotate(err, "statuses update failed")
	}
	return nil
}

// runForAllEnvStates will run runner function for every env passing a state
// for that env.
func runForAllEnvStates(st *State, runner func(st *State) error) error {
	environments, closer := st.getCollection(environmentsC)
	defer closer()

	var envDocs []bson.M
	err := environments.Find(nil).Select(bson.M{"_id": 1}).All(&envDocs)
	if err != nil {
		return errors.Annotate(err, "failed to read environments")
	}

	for _, envDoc := range envDocs {
		envUUID := envDoc["_id"].(string)
		envSt, err := st.ForEnviron(names.NewEnvironTag(envUUID))
		if err != nil {
			return errors.Annotatef(err, "failed to open environment %q", envUUID)
		}
		defer envSt.Close()
		if err := runner(envSt); err != nil {
			return errors.Annotatef(err, "environment UUID %q", envUUID)
		}
	}
	return nil
}

// AddMissingServiceStatuses creates all service status documents that do
// not already exist.
func AddMissingServiceStatuses(st *State) error {
	now := time.Now()

	environments, closer := st.getCollection(environmentsC)
	defer closer()

	var envDocs []bson.M
	err := environments.Find(nil).Select(bson.M{"_id": 1}).All(&envDocs)
	if err != nil {
		return errors.Annotate(err, "failed to read environments")
	}

	for _, envDoc := range envDocs {
		envUUID := envDoc["_id"].(string)
		envSt, err := st.ForEnviron(names.NewEnvironTag(envUUID))
		if err != nil {
			return errors.Annotatef(err, "failed to open environment %q", envUUID)
		}
		defer envSt.Close()

		services, err := envSt.AllServices()
		if err != nil {
			return errors.Annotatef(err, "failed to retrieve machines for environment %q", envUUID)
		}
		logger.Debugf("found %d services in environment %s", len(services), envUUID)

		for _, service := range services {
			_, err := service.Status()
			if cause := errors.Cause(err); errors.IsNotFound(cause) {
				logger.Debugf("service %s lacks status doc", service)
				statusDoc := statusDoc{
					EnvUUID:    envSt.EnvironUUID(),
					Status:     StatusUnknown,
					StatusInfo: MessageWaitForAgentInit,
					Updated:    now.UnixNano(),
					// This exists to preserve questionable unit-aggregation behaviour
					// while we work out how to switch to an implementation that makes
					// sense. It is also set in AddService.
					NeverSet: true,
				}
				probablyUpdateStatusHistory(envSt, service.globalKey(), statusDoc)
				err := envSt.runTransaction([]txn.Op{
					createStatusOp(envSt, service.globalKey(), statusDoc),
				})
				if err != nil && err != txn.ErrAborted {
					return err
				}
			} else if err != nil {
				return err
			}
		}
	}
	return nil
}

func addVolumeAttachmentCount(st *State) error {
	volumes, err := st.AllVolumes()
	if err != nil {
		return errors.Trace(err)
	}

	ops := make([]txn.Op, len(volumes))
	for i, volume := range volumes {
		volAttachments, err := st.VolumeAttachments(volume.VolumeTag())
		if err != nil {
			return errors.Trace(err)
		}
		ops[i] = txn.Op{
			C:      volumesC,
			Id:     volume.Tag().Id(),
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"attachmentcount", len(volAttachments)},
			}}},
		}
	}
	return st.runTransaction(ops)
}

// AddVolumeAttachmentCount adds volumeDoc.AttachmentCount and
// sets the right number to it.
func AddVolumeAttachmentCount(st *State) error {
	return runForAllEnvStates(st, addVolumeAttachmentCount)
}

func addFilesystemsAttachmentCount(st *State) error {
	filesystems, err := st.AllFilesystems()
	if err != nil {
		return errors.Trace(err)
	}

	ops := make([]txn.Op, len(filesystems))
	for i, fs := range filesystems {
		fsAttachments, err := st.FilesystemAttachments(fs.FilesystemTag())
		if err != nil {
			return errors.Trace(err)
		}
		ops[i] = txn.Op{
			C:      filesystemsC,
			Id:     fs.Tag().Id(),
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"attachmentcount", len(fsAttachments)},
			}}},
		}

	}
	return st.runTransaction(ops)
}

// AddFilesystemAttachmentCount adds filesystemDoc.AttachmentCount and
// sets the right number to it.
func AddFilesystemsAttachmentCount(st *State) error {
	return runForAllEnvStates(st, addFilesystemsAttachmentCount)
}

func getVolumeBinding(st *State, volume Volume) (string, error) {
	// first filesystem
	fs, err := st.VolumeFilesystem(volume.VolumeTag())
	if err == nil {
		return fs.FilesystemTag().String(), nil
	} else if !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}

	// then Volume.StorageInstance
	storageInstance, err := volume.StorageInstance()
	if err == nil {
		return storageInstance.String(), nil

	} else if !errors.IsNotAssigned(err) {
		return "", errors.Trace(err)
	}

	// then machine
	atts, err := st.VolumeAttachments(volume.VolumeTag())
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(atts) == 1 {
		return atts[0].Machine().String(), nil
	}
	return "", nil

}

func addBindingToVolume(st *State) error {
	volumes, err := st.AllVolumes()
	if err != nil {
		return errors.Trace(err)
	}

	ops := make([]txn.Op, len(volumes))
	for i, volume := range volumes {
		b, err := getVolumeBinding(st, volume)
		if err != nil {
			return errors.Annotatef(err, "cannot determine binding for %q", volume.Tag().String())
		}
		ops[i] = txn.Op{
			C:      volumesC,
			Id:     volume.Tag().Id(),
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"binding", b},
			}}},
		}
	}
	return st.runTransaction(ops)
}

// AddBindingToVolumes adds the binding field to volumesDoc and
// populates it.
func AddBindingToVolumes(st *State) error {
	return runForAllEnvStates(st, addBindingToVolume)
}

func getFilesystemBinding(st *State, filesystem Filesystem) (string, error) {
	storage, err := filesystem.Storage()
	if err == nil {
		return storage.String(), nil
	} else if !errors.IsNotAssigned(err) {
		return "", errors.Trace(err)
	}
	atts, err := st.FilesystemAttachments(filesystem.FilesystemTag())
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(atts) == 1 {
		return atts[0].Machine().String(), nil
	}
	return "", nil

}

func addBindingToFilesystems(st *State) error {
	filesystems, err := st.AllFilesystems()
	if err != nil {
		return errors.Trace(err)
	}

	ops := make([]txn.Op, len(filesystems))
	for i, filesystem := range filesystems {
		b, err := getFilesystemBinding(st, filesystem)
		if err != nil {
			return errors.Annotatef(err, "cannot determine binding for %q", filesystem.Tag().String())
		}
		ops[i] = txn.Op{
			C:      filesystemsC,
			Id:     filesystem.Tag().Id(),
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"binding", b},
			}}},
		}
	}
	return st.runTransaction(ops)
}

// AddBindingToFilesystems adds the binding field to filesystemDoc and populates it.
func AddBindingToFilesystems(st *State) error {
	return runForAllEnvStates(st, addBindingToFilesystems)
}

// ChangeStatusHistoryUpdatedType seeks for historicalStatusDoc records
// whose updated attribute is a time and converts them to int64.
func ChangeStatusHistoryUpdatedType(st *State) error {
	// Ensure all ids are using the new form.
	if err := runForAllEnvStates(st, changeIdsFromSeqToAuto); err != nil {
		return errors.Annotate(err, "cannot update ids of status history")
	}
	run := func(st *State) error { return changeUpdatedType(st, statusesHistoryC) }
	return runForAllEnvStates(st, run)
}

// ChangeStatusUpdatedType seeks for statusDoc records
// whose updated attribute is a time and converts them to int64.
func ChangeStatusUpdatedType(st *State) error {
	run := func(st *State) error { return changeUpdatedType(st, statusesC) }
	return runForAllEnvStates(st, run)
}

func changeIdsFromSeqToAuto(st *State) error {
	var docs []bson.M
	rawColl, closer := st.getRawCollection(statusesHistoryC)
	defer closer()

	coll, closer := st.getCollection(statusesHistoryC)
	defer closer()

	// Filtering is done by hand because the ids we are trying to modify
	// do not have uuid.
	if err := rawColl.Find(bson.M{"env-uuid": st.EnvironUUID()}).All(&docs); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return errors.Annotatef(err, "cannot find all docs for %q", statusesHistoryC)
	}

	writeableColl := coll.Writeable()
	for _, doc := range docs {
		id, ok := doc["_id"].(string)
		if !ok {
			if _, isObjectId := doc["_id"].(bson.ObjectId); isObjectId {
				continue
			}

			return errors.Errorf("unexpected id: %v", doc["_id"])
		}
		_, err := strconv.ParseInt(id, 10, 64)
		if err == nil {
			// _id will be automatically added by mongo upon insert.
			delete(doc, "_id")
			if err := writeableColl.Insert(doc); err != nil {
				return errors.Annotate(err, "cannot insert replacement doc without sequential id")
			}
			if err := rawColl.Remove(bson.M{"_id": id}); err != nil {
				return errors.Annotatef(err, "cannot migrate %q ids from sequences, current id is: %s", statusesHistoryC, id)
			}
		}
	}
	return nil

}

func changeUpdatedType(st *State, collection string) error {
	var docs []bson.M
	coll, closer := st.getCollection(collection)
	defer closer()
	err := coll.Find(nil).All(&docs)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "cannot find all docs for collection %q", collection)
	}

	wColl := coll.Writeable()
	for _, doc := range docs {
		id := doc["_id"]
		updated, ok := doc["updated"].(time.Time)
		if ok {
			if err := wColl.Update(bson.M{"_id": id}, bson.M{"$set": bson.M{"updated": updated.UTC().UnixNano()}}); err != nil {
				return errors.Annotatef(err, "cannot change %v updated from time to int64", id)
			}
		}
	}
	return nil
}

// ChangeStatusHistoryEntityId renames entityId field to globalkey.
func ChangeStatusHistoryEntityId(st *State) error {
	// Ensure all ids are using the new form.
	if err := runForAllEnvStates(st, changeIdsFromSeqToAuto); err != nil {
		return errors.Annotate(err, "cannot update ids of status history")
	}
	return runForAllEnvStates(st, changeStatusHistoryEntityId)
}

func changeStatusHistoryEntityId(st *State) error {
	statusHistory, closer := st.getRawCollection(statusesHistoryC)
	defer closer()

	var docs []bson.M
	err := statusHistory.Find(bson.D{{
		"entityid", bson.D{{"$exists", true}}}}).All(&docs)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Annotate(err, "cannot get entity ids")
	}
	for _, doc := range docs {
		id := doc["_id"]
		entityId, ok := doc["entityid"].(string)
		if !ok {
			return errors.Errorf("unexpected entity id: %v", doc["entityid"])
		}
		err := statusHistory.Update(bson.M{"_id": id}, bson.M{
			"$set":   bson.M{"globalkey": entityId},
			"$unset": bson.M{"entityid": nil}})
		if err != nil {
			return errors.Annotatef(err, "cannot update %q entityid to globalkey", id)
		}
	}
	return nil
}

// AddVolumeStatus ensures each volume has a status doc.
func AddVolumeStatus(st *State) error {
	return runForAllEnvStates(st, func(st *State) error {
		volumes, err := st.AllVolumes()
		if err != nil {
			return errors.Trace(err)
		}
		var ops []txn.Op
		for _, volume := range volumes {
			_, err := volume.Status()
			if err == nil {
				continue
			}
			if !errors.IsNotFound(err) {
				return errors.Annotate(err, "getting status")
			}
			status, err := upgradingVolumeStatus(st, volume)
			if err != nil {
				return errors.Annotate(err, "deciding volume status")
			}
			ops = append(ops, createStatusOp(st, volume.globalKey(), statusDoc{
				Status:  status,
				Updated: time.Now().UnixNano(),
			}))
		}
		if len(ops) > 0 {
			return errors.Trace(st.runTransaction(ops))
		}
		return nil
	})
}

// If the volume has not been provisioned, then it should be Pending;
// if it has been provisioned, but there is an unprovisioned attachment,
// then it should be Attaching; otherwise it is Attached.
func upgradingVolumeStatus(st *State, volume Volume) (Status, error) {
	if _, err := volume.Info(); errors.IsNotProvisioned(err) {
		return StatusPending, nil
	}
	attachments, err := st.VolumeAttachments(volume.VolumeTag())
	if err != nil {
		return "", errors.Trace(err)
	}
	for _, attachment := range attachments {
		_, err := attachment.Info()
		if errors.IsNotProvisioned(err) {
			return StatusAttaching, nil
		}
	}
	return StatusAttached, nil
}

// AddFilesystemStatus ensures each filesystem has a status doc.
func AddFilesystemStatus(st *State) error {
	return runForAllEnvStates(st, func(st *State) error {
		filesystems, err := st.AllFilesystems()
		if err != nil {
			return errors.Trace(err)
		}
		var ops []txn.Op
		for _, filesystem := range filesystems {
			_, err := filesystem.Status()
			if err == nil {
				continue
			}
			if !errors.IsNotFound(err) {
				return errors.Annotate(err, "getting status")
			}
			status, err := upgradingFilesystemStatus(st, filesystem)
			if err != nil {
				return errors.Annotate(err, "deciding filesystem status")
			}
			ops = append(ops, createStatusOp(st, filesystem.globalKey(), statusDoc{
				Status:  status,
				Updated: time.Now().UnixNano(),
			}))
		}
		if len(ops) > 0 {
			return errors.Trace(st.runTransaction(ops))
		}
		return nil
	})
}

// If the filesystem has not been provisioned, then it should be Pending;
// if it has been provisioned, but there is an unprovisioned attachment, then
// it should be Attaching; otherwise it is Attached.
func upgradingFilesystemStatus(st *State, filesystem Filesystem) (Status, error) {
	if _, err := filesystem.Info(); errors.IsNotProvisioned(err) {
		return StatusPending, nil
	}
	attachments, err := st.FilesystemAttachments(filesystem.FilesystemTag())
	if err != nil {
		return "", errors.Trace(err)
	}
	for _, attachment := range attachments {
		_, err := attachment.Info()
		if errors.IsNotProvisioned(err) {
			return StatusAttaching, nil
		}
	}
	return StatusAttached, nil
}

// MigrateSettingsSchema migrates the schema of the settings collection,
// moving non-reserved keys at the top-level into a subdoc, and introducing
// a top-level "version" field with the initial value matching txn-revno.
//
// This migration takes place both before and after env-uuid migration,
// to get the correct txn-revno value.
func MigrateSettingsSchema(st *State) error {
	coll, closer := st.getRawCollection(settingsC)
	defer closer()

	upgradesLogger.Debugf("migrating schema of the %s collection", settingsC)
	iter := coll.Find(nil).Iter()
	defer iter.Close()

	var ops []txn.Op
	var doc bson.M
	for iter.Next(&doc) {
		if !settingsDocNeedsMigration(doc) {
			continue
		}

		id := doc["_id"]
		txnRevno := doc["txn-revno"].(int64)

		// Remove reserved attributes; we'll move the remaining
		// ones to the "settings" subdoc.
		delete(doc, "env-uuid")
		delete(doc, "_id")
		delete(doc, "txn-revno")
		delete(doc, "txn-queue")

		// If there exists a setting by the name "settings",
		// we must remove it first, or it will collide with
		// the dotted-notation $sets.
		if _, ok := doc["settings"]; ok {
			ops = append(ops, txn.Op{
				C:      settingsC,
				Id:     id,
				Assert: txn.DocExists,
				Update: bson.D{{"$unset", bson.D{{"settings", 1}}}},
			})
		}

		var update bson.D
		for key, value := range doc {
			if key != "settings" && key != "version" {
				// Don't try to unset these fields,
				// as we've unset "settings" above
				// already, and we'll overwrite
				// "version" below.
				update = append(update, bson.DocElem{
					"$unset", bson.D{{key, 1}},
				})
			}
			update = append(update, bson.DocElem{
				"$set", bson.D{{"settings." + key, value}},
			})
		}
		if len(update) == 0 {
			// If there are no settings, then we need
			// to add an empty "settings" map so we
			// can tell for next time that migration
			// is complete, and don't move the "version"
			// field we add.
			update = bson.D{{
				"$set", bson.D{{"settings", bson.M{}}},
			}}
		}
		update = append(update, bson.DocElem{
			"$set", bson.D{{"version", txnRevno}},
		})

		ops = append(ops, txn.Op{
			C:      settingsC,
			Id:     id,
			Assert: txn.DocExists,
			Update: update,
		})
	}
	if err := iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

func settingsDocNeedsMigration(doc bson.M) bool {
	// It is not possible for there to exist a settings value
	// with type bson.M, so we know that it is the new settings
	// field and not just a setting with the name "settings".
	if _, ok := doc["settings"].(bson.M); ok {
		return false
	}
	return true
}

func addDefaultBindingsToServices(st *State) error {
	services, err := st.AllServices()
	if err != nil {
		return errors.Trace(err)
	}

	ops := make([]txn.Op, len(services))
	for i, service := range services {
		ch, _, err := service.Charm()
		if err != nil && !errors.IsNotFound(err) {
			return errors.Annotatef(err, "cannot get charm for service %q", service.Name())
		}
		if ch == nil {
			// Nothing to do if charm is not set yet, as bindings will be
			// populated when set.
			continue
		}
		// Passing nil for the bindings map will use the defaults.
		ops[i], err = endpointBindingsForCharmOp(st, service.globalKey(), nil, ch.Meta())
		if err != nil {
			return errors.Annotatef(err, "setting default endpoint bindings for service %q", service.Name())
		}
	}
	return st.runTransaction(ops)
}

// AddDefaultEndpointBindingsToServices adds default endpoint bindings for each
// service. As long as the service has a charm URL set, each charm endpoint will
// be bound to the default space.
func AddDefaultEndpointBindingsToServices(st *State) error {
	return runForAllEnvStates(st, addDefaultBindingsToServices)
}
