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

	users, closer := st.getCollection(usersC)
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

	return st.runTransaction(ops)
}

// AddStateUsersAsEnvironUsers loops through all users stored in state and
// adds them as environment users with a local provider.
func AddStateUsersAsEnvironUsers(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	var userSlice []userDoc
	users, closer := st.getCollection(usersC)
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
			_, err = st.AddEnvironmentUser(uTag, uTag)
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
	mergedRanges = network.CollapsePorts(unit.doc.Ports)
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
	portsId := portsGlobalKey(machineId, network.DefaultPublic)
	machinePorts, err = st.Ports(portsId)
	if errors.IsNotFound(err) {
		// No ports document on this machine yet, let's add ops to
		// create an empty one first.
		pdoc := portsDoc{
			Id:    portsId,
			Ports: []PortRange{},
		}
		ops = append(ops, txn.Op{
			C:      openedPortsC,
			Id:     portsId,
			Insert: pdoc,
		})
		machinePorts = &Ports{st, pdoc, true}
		upgradesLogger.Debugf(
			"created ports for machine %q, network %q",
			machineId, network.DefaultPublic,
		)
	} else if err != nil {
		return nil, nil, errors.Annotatef(
			err,
			"cannot get machine %q (of unit %q) ports",
			machineId, unit,
		)
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
	units, closer := st.getCollection(unitsC)
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
		if IsNotAssigned(err) {
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
			unit, rangesToMigrate, machinePorts.Id(), ops,
		)

		if err = st.runTransaction(ops); err != nil {
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
	units, closer := st.getCollection(unitsC)
	defer closer()

	meterStatuses, closer := st.getCollection(meterStatusC)
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

		msdoc := meterStatusDoc{
			Code: MeterNotSet,
		}
		ops := []txn.Op{createMeterStatusOp(st, unit.globalKey(), msdoc)}
		if err = st.runTransaction(ops); err != nil {
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

	return st.runTransaction(ops)
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
	err := st.runTransaction(ops)
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
	return st.runTransaction(ops)
}

// MigrateMachineInstanceIdToInstanceData migrates the deprecated "instanceid"
// machine field into "instanceid" in the instanceData doc.
func MigrateMachineInstanceIdToInstanceData(st *State) error {
	err := st.ResumeTransactions()
	if err != nil {
		return errors.Trace(err)
	}

	instDatas, closer := st.getCollection(instanceDataC)
	defer closer()
	machines, closer := st.getCollection(machinesC)
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
	return st.runTransaction(ops)
}

// AddEnvUUIDToServices prepends the environment UUID to the ID of
// all service docs and adds new "env-uuid" field.
func AddEnvUUIDToServices(st *State) error {
	return addEnvUUIDToEntityCollection(st, servicesC, "name")
}

// AddEnvUUIDToUnits prepends the environment UUID to the ID of all
// unit docs and adds new "env-uuid" field.
func AddEnvUUIDToUnits(st *State) error {
	return addEnvUUIDToEntityCollection(st, unitsC, "name")
}

// AddEnvUUIDToMachines prepends the environment UUID to the ID of
// all machine docs and adds new "env-uuid" field.
func AddEnvUUIDToMachines(st *State) error {
	return addEnvUUIDToEntityCollection(st, machinesC, "machineid")
}

// AddEnvUUIDToAnnotations prepends the environment UUID to the ID of
// all annotation docs and adds new "env-uuid" field.
func AddEnvUUIDToAnnotations(st *State) error {
	return addEnvUUIDToEntityCollection(st, annotationsC, "globalkey")
}

// AddEnvUUIDToStatuses prepends the environment UUID to the ID of
// all Statuses docs and adds new "env-uuid" field.
func AddEnvUUIDToStatuses(st *State) error {
	return addEnvUUIDToEntityCollection(st, statusesC, "")
}

// AddEnvUUIDToNetworks prepends the environment UUID to the ID of
// all network docs and adds new "env-uuid" field.
func AddEnvUUIDToNetworks(st *State) error {
	return addEnvUUIDToEntityCollection(st, networksC, "name")
}

// AddEnvUUIDToRequestedNetworks prepends the environment UUID to the ID of
// all requestedNetworks docs and adds new "env-uuid" field.
func AddEnvUUIDToRequestedNetworks(st *State) error {
	return addEnvUUIDToEntityCollection(st, requestedNetworksC, "requestednetworkid")
}

// AddEnvUUIDToNetworkInterfaces prepends adds a new "env-uuid" field to all
// networkInterfaces docs.
func AddEnvUUIDToNetworkInterfaces(st *State) error {
	coll, closer := st.getCollection(networkInterfacesC)
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
	return st.runTransaction(ops)
}

// AddEnvUUIDToCharms prepends the environment UUID to the ID of
// all charm docs and adds new "env-uuid" field.
func AddEnvUUIDToCharms(st *State) error {
	return addEnvUUIDToEntityCollection(st, charmsC, "url")
}

// AddEnvUUIDToMinUnits prepends the environment UUID to the ID of
// all minUnits docs and adds new "env-uuid" field.
func AddEnvUUIDToMinUnits(st *State) error {
	return addEnvUUIDToEntityCollection(st, minUnitsC, "servicename")
}

// AddEnvUUIDToSequences prepends the environment UUID to the ID of
// all sequence docs and adds new "env-uuid" field.
func AddEnvUUIDToSequences(st *State) error {
	return addEnvUUIDToEntityCollection(st, sequenceC, "name")
}

// AddEnvUUIDToReboots prepends the environment UUID to the ID of
// all reboot docs and adds new "env-uuid" field.
func AddEnvUUIDToReboots(st *State) error {
	return addEnvUUIDToEntityCollection(st, rebootC, "machineid")
}

// AddEnvUUIDToContainerRefs prepends the environment UUID to the ID of all
// containerRef docs and adds new "env-uuid" field.
func AddEnvUUIDToContainerRefs(st *State) error {
	return addEnvUUIDToEntityCollection(st, containerRefsC, "machineid")
}

// AddEnvUUIDToInstanceData prepends the environment UUID to the ID of
// all instanceData docs and adds new "env-uuid" field.
func AddEnvUUIDToInstanceData(st *State) error {
	return addEnvUUIDToEntityCollection(st, instanceDataC, "machineid")
}

// AddEnvUUIDToCleanups prepends the environment UUID to the ID of
// all cleanup docs and adds new "env-uuid" field.
func AddEnvUUIDToCleanups(st *State) error {
	return addEnvUUIDToEntityCollection(st, cleanupsC, "")
}

// AddEnvUUIDToConstraints prepends the environment UUID to the ID of
// all constraints docs and adds new "env-uuid" field.
func AddEnvUUIDToConstraints(st *State) error {
	return addEnvUUIDToEntityCollection(st, constraintsC, "")
}

// AddEnvUUIDToSettings prepends the environment UUID to the ID of
// all settings docs and adds new "env-uuid" field.
func AddEnvUUIDToSettings(st *State) error {
	return addEnvUUIDToEntityCollection(st, settingsC, "")
}

// AddEnvUUIDToSettingsRefs prepends the environment UUID to the ID of
// all settingRef docs and adds new "env-uuid" field.
func AddEnvUUIDToSettingsRefs(st *State) error {
	return addEnvUUIDToEntityCollection(st, settingsrefsC, "")
}

// AddEnvUUIDToRelations prepends the environment UUID to the ID of
// all relations docs and adds new "env-uuid" and "key" fields.
func AddEnvUUIDToRelations(st *State) error {
	return addEnvUUIDToEntityCollection(st, relationsC, "key")
}

// AddEnvUUIDToRelationScopes prepends the environment UUID to the ID of
// all relationscopes docs and adds new "env-uuid" field and "key" fields.
func AddEnvUUIDToRelationScopes(st *State) error {
	return addEnvUUIDToEntityCollection(st, relationScopesC, "key")
}

// AddEnvUUIDToMeterStatus prepends the environment UUID to the ID of
// all meterStatus docs and adds new "env-uuid" field and "id" fields.
func AddEnvUUIDToMeterStatus(st *State) error {
	return addEnvUUIDToEntityCollection(st, meterStatusC, "")
}

func addEnvUUIDToEntityCollection(st *State, collName, fieldForOldID string) error {
	env, err := st.Environment()
	if err != nil {
		return errors.Annotate(err, "failed to load environment")
	}

	coll, closer := st.getCollection(collName)
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
		if fieldForOldID != "" {
			doc[fieldForOldID] = oldID
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
	return st.runTransaction(ops)
}

// migrateJobManageNetworking adds the job JobManageNetworking to all
// machines except for:
//
// - machines in a MAAS environment,
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

	// Check for MAAS or manual (aka null) provider.
	if envType == provider.MAAS || provider.IsManual(envType) {
		// No job adding for these environment types.
		return nil
	}

	// Iterate over all machines and create operations.
	machinesCollection, closer := st.getCollection(machinesC)
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

	// Run transaction.
	return st.runTransaction(ops)
}
