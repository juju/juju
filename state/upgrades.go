// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v4"
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
			upgradesLogger.Infof("user '%s' already added to environment", uTag.Username())
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
			createMeterStatusOp(st, unit.globalKey(), &meterStatusDoc{Code: MeterNotSet}),
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
			{"owner", owner.Username()},
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
		if ok {
			continue
		}

		zone, err := azFunc(st, instance.Id(doc["instanceid"].(string)))
		if err != nil {
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
	setNewFields := func(b bson.M) error {
		parts, err := extractPortsIdParts(b["_id"].(string))
		if err != nil {
			return errors.Trace(err)
		}
		b["machine-id"] = parts[machineIdPart]
		b["network-name"] = parts[networkNamePart]
		return nil
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
	var doc bson.M
	for iter.Next(&doc) {
		oldID := doc["_id"]
		id := st.docID(fmt.Sprint(oldID))

		// collection specific updates
		for _, update := range updates {
			if err := update(doc); err != nil {
				return errors.Trace(err)
			}
		}
		doc["_id"] = id
		doc["env-uuid"] = uuid

		ops = append(ops,
			[]txn.Op{{
				C:      collName,
				Id:     oldID,
				Assert: txn.DocExists,
				Remove: true,
			}, {
				C:      collName,
				Id:     id,
				Assert: txn.DocMissing,
				Insert: doc,
			}}...)
		doc = nil // Force creation of new map for the next iteration
	}
	if err = iter.Err(); err != nil {
		return errors.Trace(err)
	}
	return st.runRawTransaction(ops)
}

type updateFunc func(bson.M) error

// setOldID returns an updateFunc which populates the doc's original ID
// in the named field.
func setOldID(name string) updateFunc {
	return func(b bson.M) error {
		b[name] = b["_id"]
		return nil
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
