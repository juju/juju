// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage/provider"
	"github.com/juju/juju/internal/tools"
	stateerrors "github.com/juju/juju/state/errors"
)

// unitAgentGlobalKey returns the global database key for the named unit.
func unitAgentGlobalKey(name string) string {
	return unitAgentGlobalKeyPrefix + name
}

// unitAgentGlobalKeyPrefix is the string we use to denote unit agent kind.
const unitAgentGlobalKeyPrefix = "u#"

// MachineRef is a reference to a machine, without being a full machine.
// This exists to allow us to use state functions without requiring a
// state.Machine, without having to require a real machine.
type MachineRef interface {
	DocID() string
	Id() string
	MachineTag() names.MachineTag
	Life() Life
	Clean() bool
	ContainerType() instance.ContainerType
	Base() Base
	AddPrincipal(string)
	FileSystems() []string
}

// unitDoc represents the internal state of a unit in MongoDB.
// Note the correspondence with UnitInfo in core/multiwatcher.
type unitDoc struct {
	DocID                  string `bson:"_id"`
	Name                   string `bson:"name"`
	ModelUUID              string `bson:"model-uuid"`
	Base                   Base   `bson:"base"`
	Application            string
	CharmURL               *string
	Principal              string
	Subordinates           []string
	StorageAttachmentCount int `bson:"storageattachmentcount"`
	MachineId              string
	Tools                  *tools.Tools `bson:",omitempty"`
	Life                   Life
	PasswordHash           string
}

// Unit represents the state of an application unit.
type Unit struct {
	st  *State
	doc unitDoc

	// Cache the model type as it is immutable as is referenced
	// during the lifecycle of the unit.
	modelType ModelType
}

func newUnit(st *State, modelType ModelType, udoc *unitDoc) *Unit {
	unit := &Unit{
		st:        st,
		doc:       *udoc,
		modelType: modelType,
	}
	return unit
}

// application returns the application.
func (u *Unit) application() (*Application, error) {
	return u.st.Application(u.doc.Application)
}

// applicationName returns the application name.
func (u *Unit) applicationName() string {
	return u.doc.Application
}

// base returns the deployed charm's base.
func (u *Unit) base() Base {
	return u.doc.Base
}

// name returns the unit name.
func (u *Unit) name() string {
	return u.doc.Name
}

// unitGlobalKey returns the global database key for the named unit.
func unitGlobalKey(name string) string {
	return "u#" + name + "#charm"
}

// globalAgentKey returns the global database key for the unit.
func (u *Unit) globalAgentKey() string {
	return unitAgentGlobalKey(u.doc.Name)
}

// globalKey returns the global database key for the unit.
func (u *Unit) globalKey() string {
	return unitGlobalKey(u.doc.Name)
}

// life returns whether the unit is Alive, Dying or Dead.
func (u *Unit) life() Life {
	return u.doc.Life
}

var unitHasNoSubordinates = bson.D{{
	"$or", []bson.D{
		{{"subordinates", bson.D{{"$size", 0}}}},
		{{"subordinates", bson.D{{"$exists", false}}}},
	},
}}

var unitHasNoStorageAttachments = bson.D{{
	"$or", []bson.D{
		{{"storageattachmentcount", 0}},
		{{"storageattachmentcount", bson.D{{"$exists", false}}}},
	},
}}

// EnsureDead sets the unit lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. If the unit has subordinates, it will
// return ErrUnitHasSubordinates; otherwise, if it has storage instances,
// it will return ErrUnitHasStorageInstances.
func (u *Unit) EnsureDead() (err error) {
	if u.doc.Life == Dead {
		return nil
	}
	defer func() {
		if err == nil {
			u.doc.Life = Dead
		}
	}()
	assert := append(notDeadDoc, bson.DocElem{
		"$and", []bson.D{
			unitHasNoSubordinates,
			unitHasNoStorageAttachments,
		},
	})
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: assert,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
	}}
	if err := u.st.db().RunTransaction(ops); err != txn.ErrAborted {
		return err
	}
	if notDead, err := isNotDead(u.st, unitsC, u.doc.DocID); err != nil {
		return err
	} else if !notDead {
		return nil
	}
	if err := u.refresh(); errors.Is(err, errors.NotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if len(u.doc.Subordinates) > 0 {
		return stateerrors.ErrUnitHasSubordinates
	}
	return stateerrors.ErrUnitHasStorageAttachments
}

// Remove removes the unit from state, and may remove its application as well, if
// the application is Dying and no other references to it exist. It will fail if
// the unit is not Dead.
func (u *Unit) Remove(store objectstore.ObjectStore) error {
	return nil
}

// RemoveWithForce removes the unit from state similar to the unit.Remove() but
// it ignores errors.
// In addition, this function also returns all non-fatal operational errors
// encountered.
func (u *Unit) RemoveWithForce(store objectstore.ObjectStore, force bool, maxWait time.Duration) ([]error, error) {
	return nil, nil
}

// isPrincipal returns whether the unit is deployed in its own container,
// and can therefore have subordinate applications deployed alongside it.
func (u *Unit) isPrincipal() bool {
	return u.doc.Principal == ""
}

// machine returns the unit's machine.
//
// machine is part of the machineAssignable interface.
func (u *Unit) machine() (*Machine, error) {
	id, err := u.AssignedMachineId()
	if err != nil {
		return nil, errors.Annotatef(err, "unit %v cannot get assigned machine", u)
	}
	m, err := u.st.Machine(id)
	if err != nil {
		return nil, errors.Annotatef(err, "unit %v misses machine id %v", u, id)
	}
	return m, nil
}

// noAssignedMachineOp is part of the machineAssignable interface.
func (u *Unit) noAssignedMachineOp() txn.Op {
	id := u.doc.DocID
	if u.doc.Principal != "" {
		id = u.doc.Principal
	}
	return txn.Op{
		C:      unitsC,
		Id:     id,
		Assert: bson.D{{"machineid", ""}},
	}
}

// refresh refreshes the contents of the Unit from the underlying
// state. It an error that satisfies errors.IsNotFound if the unit has
// been removed.
func (u *Unit) refresh() error {
	units, closer := u.st.db().GetCollection(unitsC)
	defer closer()

	err := units.FindId(u.doc.DocID).One(&u.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("unit %q", u)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh unit %q", u)
	}
	return nil
}

// charm returns the charm for the unit, or the application if the unit's charm
// has not been set yet.
func (u *Unit) charm() (CharmRefFull, error) {
	cURL := u.doc.CharmURL
	if cURL == nil {
		app, err := u.application()
		if err != nil {
			return nil, err
		}
		cURL, _ = app.charmURL()
	}

	if cURL == nil {
		return nil, errors.Errorf("missing charm URL for %q", u.name())
	}

	var ch CharmRefFull
	err := retry.Call(retry.CallArgs{
		Attempts: 20,
		Delay:    50 * time.Millisecond,
		Func: func() error {
			var err error
			ch, err = u.st.Charm(*cURL)
			return err
		},
		Clock: u.st.clock(),
		NotifyFunc: func(err error, attempt int) {
			logger.Warningf(context.TODO(), "error getting charm for unit %q. Retrying (attempt %d): %v", u.name(), attempt, err)
		},
	})

	return ch, errors.Annotatef(err, "getting charm for %s", u)
}

// assertCharmOps returns txn.Ops to assert the current charm of the unit.
// If the unit currently has no charm URL set, then the application's charm
// URL will be checked by the txn.Ops also.
func (u *Unit) assertCharmOps(ch CharmRefFull) []txn.Op {
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.Name,
		Assert: bson.D{{"charmurl", u.doc.CharmURL}},
	}}
	if u.doc.CharmURL != nil {
		appName := u.applicationName()
		ops = append(ops, txn.Op{
			C:      applicationsC,
			Id:     appName,
			Assert: bson.D{{"charmurl", ch.URL()}},
		})
	}
	return ops
}

// Tag returns a name identifying the unit.
// The returned name will be different from other Tag values returned by any
// other entities from the same state.
func (u *Unit) Tag() names.Tag {
	return u.unitTag()
}

// unitTag returns a names.UnitTag representing this Unit, unless the
// unit Name is invalid, in which case it will panic
func (u *Unit) unitTag() names.UnitTag {
	return names.NewUnitTag(u.name())
}

func unitNotAssignedError(u *Unit) error {
	msg := fmt.Sprintf("unit %q is not assigned to a machine", u)
	return errors.NewNotAssigned(nil, msg)
}

// AssignedMachineId returns the id of the assigned machine.
func (u *Unit) AssignedMachineId() (id string, err error) {
	if u.doc.MachineId == "" {
		return "", unitNotAssignedError(u)
	}
	return u.doc.MachineId, nil
}

var (
	machineNotCleanErr = errors.New("machine is dirty")
	alreadyAssignedErr = errors.New("unit is already assigned to a machine")
	inUseErr           = errors.New("machine is not unused")
)

// assignToMachine is the internal version of AssignToMachine.
func (u *Unit) assignToMachine(
	m MachineRef,
) (err error) {
	defer assignContextf(&err, u.name(), fmt.Sprintf("machine %s", m))
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		u, m := u, m // don't change outer vars
		if attempt > 0 {
			var err error
			u, err = u.st.Unit(u.name())
			if err != nil {
				return nil, errors.Trace(err)
			}
			m, err = u.st.Machine(m.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		return u.assignToMachineOps(m, false)
	}
	if err := u.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	u.doc.MachineId = m.Id()
	m.AddPrincipal(u.doc.Name)
	return nil
}

// assignToMachineOps returns txn.Ops to assign a unit to a machine.
// assignToMachineOps returns specific errors in some cases:
// - machineNotAliveErr when the machine is not alive.
// - unitNotAliveErr when the unit is not alive.
// - alreadyAssignedErr when the unit has already been assigned
// - inUseErr when the machine already has a unit assigned (if unused is true)
func (u *Unit) assignToMachineOps(
	m MachineRef,
	unused bool,
) ([]txn.Op, error) {
	if u.life() != Alive {
		return nil, unitNotAliveErr
	}
	if u.doc.MachineId != "" {
		if u.doc.MachineId != m.Id() {
			return nil, alreadyAssignedErr
		}
		return nil, jujutxn.ErrNoOperations
	}
	if unused && !m.Clean() {
		return nil, inUseErr
	}
	storageParams, err := u.storageParams()
	if err != nil {
		return nil, errors.Trace(err)
	}
	sb, err := NewStorageConfigBackend(u.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storagePools, err := storagePools(sb, storageParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateUnitMachineAssignment(
		u.st, m, u.doc.Base, u.doc.Principal != "", storagePools,
	); err != nil {
		return nil, errors.Trace(err)
	}
	storageOps, volumesAttached, filesystemsAttached, err := sb.hostStorageOps(m.Id(), storageParams)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// addMachineStorageAttachmentsOps will add a txn.Op that ensures
	// that no filesystems were concurrently added to the machine if
	// any of the filesystems being attached specify a location.
	attachmentOps, err := addMachineStorageAttachmentsOps(
		u.st, m, volumesAttached, filesystemsAttached,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageOps = append(storageOps, attachmentOps...)

	assert := append(isAliveDoc, bson.D{{
		// The unit's subordinates must not change while we're
		// assigning it to a machine, to ensure machine storage
		// is created for subordinate units.
		"subordinates", u.doc.Subordinates,
	}, {
		"$or", []bson.D{
			{{"machineid", ""}},
			{{"machineid", m.Id()}},
		},
	}}...)
	massert := isAliveDoc
	if unused {
		massert = append(massert, bson.D{{"clean", bson.D{{"$ne", false}}}}...)
	}
	ops := []txn.Op{{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: assert,
		Update: bson.D{{"$set", bson.D{{"machineid", m.Id()}}}},
	}, {
		C:      machinesC,
		Id:     m.DocID(),
		Assert: massert,
		Update: bson.D{{"$addToSet", bson.D{{"principals", u.doc.Name}}}, {"$set", bson.D{{"clean", false}}}},
	},
		removeStagedAssignmentOp(u.doc.DocID),
	}
	ops = append(ops, storageOps...)
	return ops, nil
}

// validateUnitMachineAssignment validates the parameters for assigning a unit
// to a specified machine.
func validateUnitMachineAssignment(
	st *State,
	m MachineRef,
	base Base,
	isSubordinate bool,
	storagePools set.Strings,
) (err error) {
	if m.Life() != Alive {
		return machineNotAliveErr
	}
	if isSubordinate {
		return fmt.Errorf("unit is a subordinate")
	}
	if !base.compatibleWith(m.Base()) {
		return fmt.Errorf("base does not match: unit has %q, machine has %q", base.DisplayString(), m.Base().DisplayString())
	}
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}
	if err := validateDynamicMachineStoragePools(sb, m, storagePools); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// validateDynamicMachineStorageParams validates that the provided machine
// storage parameters are compatible with the specified machine.
func validateDynamicMachineStorageParams(
	m *Machine,
	params *storageParams,
) error {
	sb, err := NewStorageConfigBackend(m.st)
	if err != nil {
		return errors.Trace(err)
	}
	pools, err := storagePools(sb, params)
	if err != nil {
		return err
	}
	if err := validateDynamicMachineStoragePools(sb.storageBackend, m, pools); err != nil {
		return err
	}
	// Validate the volume/filesystem attachments for the machine.
	for volumeTag := range params.volumeAttachments {
		volume, err := getVolumeByTag(sb.mb, volumeTag)
		if err != nil {
			return errors.Trace(err)
		}
		if !volume.Detachable() && volume.doc.HostId != m.Id() {
			return errors.Errorf(
				"storage is non-detachable (bound to machine %s)",
				volume.doc.HostId,
			)
		}
	}
	for filesystemTag := range params.filesystemAttachments {
		filesystem, err := getFilesystemByTag(sb.mb, filesystemTag)
		if err != nil {
			return errors.Trace(err)
		}
		if !filesystem.Detachable() && filesystem.doc.HostId != m.Id() {
			host := storageAttachmentHost(filesystem.doc.HostId)
			return errors.Errorf(
				"storage is non-detachable (bound to %s)",
				names.ReadableString(host),
			)
		}
	}
	return nil
}

// storagePools returns the names of storage pools in each of the
// volume, filesystem and attachments in the machine storage parameters.
func storagePools(sb *storageConfigBackend, params *storageParams) (set.Strings, error) {
	pools := make(set.Strings)
	for _, v := range params.volumes {
		v, err := sb.volumeParamsWithDefaults(v.Volume)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pools.Add(v.Pool)
	}
	for _, f := range params.filesystems {
		f, err := sb.filesystemParamsWithDefaults(f.Filesystem)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pools.Add(f.Pool)
	}
	for volumeTag := range params.volumeAttachments {
		volume, err := sb.Volume(volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if params, ok := volume.Params(); ok {
			pools.Add(params.Pool)
		} else {
			info, err := volume.Info()
			if err != nil {
				return nil, errors.Trace(err)
			}
			pools.Add(info.Pool)
		}
	}
	for filesystemTag := range params.filesystemAttachments {
		filesystem, err := sb.Filesystem(filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if params, ok := filesystem.Params(); ok {
			pools.Add(params.Pool)
		} else {
			info, err := filesystem.Info()
			if err != nil {
				return nil, errors.Trace(err)
			}
			pools.Add(info.Pool)
		}
	}
	return pools, nil
}

// validateDynamicMachineStoragePools validates that all of the specified
// storage pools support dynamic storage provisioning. If any provider doesn't
// support dynamic storage, then an IsNotSupported error is returned.
func validateDynamicMachineStoragePools(sb *storageBackend, m MachineRef, pools set.Strings) error {
	if pools.IsEmpty() {
		return nil
	}
	return validateDynamicStoragePools(sb, pools, m.ContainerType())
}

// validateDynamicStoragePools validates that all of the specified storage
// providers support dynamic storage provisioning. If any provider doesn't
// support dynamic storage, then an IsNotSupported error is returned.
func validateDynamicStoragePools(sb *storageBackend, pools set.Strings, containerType instance.ContainerType) error {
	for pool := range pools {
		providerType, p, _, err := poolStorageProvider(sb, pool)
		if err != nil {
			return errors.Trace(err)
		}
		if containerType != "" && !provider.AllowedContainerProvider(providerType) {
			// TODO(axw) later we might allow *any* storage, and
			// passthrough/bindmount storage. That would imply either
			// container creation time only, or requiring containers
			// to be restarted to pick up new configuration.
			return errors.NotSupportedf("adding storage of type %q to %s container", providerType, containerType)
		}
		if !p.Dynamic() {
			return errors.NewNotSupported(err, fmt.Sprintf(
				"%q storage provider does not support dynamic storage",
				providerType,
			))
		}
	}
	return nil
}

func assignContextf(err *error, unitName string, target string) {
	if *err != nil {
		*err = errors.Annotatef(*err,
			"cannot assign unit %q to %s",
			unitName, target,
		)
	}
}

// assignToNewMachineOps returns txn.Ops to assign the unit to a machine
// created according to the supplied params, with the supplied constraints.
func (u *Unit) assignToNewMachineOps(
	template MachineTemplate,
	parentId string,
	containerType instance.ContainerType,
) (*Machine, []txn.Op, error) {

	if u.life() != Alive {
		return nil, nil, unitNotAliveErr
	}
	if u.doc.MachineId != "" {
		return nil, nil, alreadyAssignedErr
	}

	template.principals = []string{u.doc.Name}
	template.Dirty = true

	var (
		mdoc *machineDoc
		ops  []txn.Op
		err  error
	)
	switch {
	case parentId == "" && containerType == "":
		mdoc, ops, err = u.st.addMachineOps(template)
	case parentId == "":
		if containerType == "" {
			return nil, nil, errors.New("assignToNewMachine called without container type (should never happen)")
		}
		// The new parent machine is clean and only hosts units,
		// regardless of its child.
		parentParams := template
		mdoc, ops, err = u.st.addMachineInsideNewMachineOps(template, parentParams, containerType)
	default:
		mdoc, ops, err = u.st.addMachineInsideMachineOps(template, parentId, containerType)
	}
	if err != nil {
		return nil, nil, err
	}

	// Ensure the host machine is really clean.
	if parentId != "" {
		mparent, err := u.st.Machine(parentId)
		if err != nil {
			return nil, nil, err
		}
		if !mparent.Clean() {
			return nil, nil, machineNotCleanErr
		}
		containers, err := mparent.Containers()
		if err != nil {
			return nil, nil, err
		}
		if len(containers) > 0 {
			return nil, nil, machineNotCleanErr
		}
		parentDocId := u.st.docID(parentId)
		ops = append(ops, txn.Op{
			C:      machinesC,
			Id:     parentDocId,
			Assert: bson.D{{"clean", true}},
		}, txn.Op{
			C:      containerRefsC,
			Id:     parentDocId,
			Assert: bson.D{hasNoContainersTerm},
		})
	}

	// The unit's subordinates must not change while we're
	// assigning it to a machine, to ensure machine storage
	// is created for subordinate units.
	subordinatesUnchanged := bson.D{{"subordinates", u.doc.Subordinates}}
	isUnassigned := bson.D{{"machineid", ""}}
	asserts := append(isAliveDoc, isUnassigned...)
	asserts = append(asserts, subordinatesUnchanged...)

	ops = append(ops, txn.Op{
		C:      unitsC,
		Id:     u.doc.DocID,
		Assert: asserts,
		Update: bson.D{{"$set", bson.D{{"machineid", mdoc.Id}}}},
	},
		removeStagedAssignmentOp(u.doc.DocID),
	)
	return &Machine{u.st, *mdoc}, ops, nil
}

// constraints returns the unit's deployment constraints.
func (u *Unit) constraints() (*constraints.Value, error) {
	cons, err := readConstraints(u.st, u.globalAgentKey())
	if errors.Is(err, errors.NotFound) {
		// Lack of constraints indicates lack of unit.
		return nil, errors.NotFoundf("unit")
	} else if err != nil {
		return nil, err
	}
	if !cons.HasArch() && !cons.HasInstanceType() {
		app, err := u.application()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if origin := app.charmOrigin(); origin != nil && origin.Platform != nil {
			if origin.Platform.Architecture != "" {
				cons.Arch = &origin.Platform.Architecture
			}
		}
		if !cons.HasArch() {
			a := constraints.ArchOrDefault(cons, nil)
			cons.Arch = &a
		}
	}
	return &cons, nil
}

// assignToNewMachine assigns the unit to a new machine with the
// optional placement directive, with constraints determined according
// to the application and model constraints at the time of unit creation.
func (u *Unit) assignToNewMachine(placement string) error {
	if u.doc.Principal != "" {
		return fmt.Errorf("unit is a subordinate")
	}
	var m *Machine
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var err error
		u := u // don't change outer var
		if attempt > 0 {
			u, err = u.st.Unit(u.name())
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		cons, err := u.constraints()
		if err != nil {
			return nil, err
		}
		var containerType instance.ContainerType
		if cons.HasContainer() {
			containerType = *cons.Container
		}
		storageParams, err := u.storageParams()
		if err != nil {
			return nil, errors.Trace(err)
		}
		template := MachineTemplate{
			Base:                  u.doc.Base,
			Constraints:           *cons,
			Placement:             placement,
			Dirty:                 placement != "",
			Volumes:               storageParams.volumes,
			VolumeAttachments:     storageParams.volumeAttachments,
			Filesystems:           storageParams.filesystems,
			FilesystemAttachments: storageParams.filesystemAttachments,
		}
		// Get the ops necessary to create a new machine, and the
		// machine doc that will be added with those operations
		// (which includes the machine id).
		var ops []txn.Op
		m, ops, err = u.assignToNewMachineOps(template, "", containerType)
		return ops, err
	}
	if err := u.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	u.doc.MachineId = m.doc.Id
	return nil
}

type byStorageInstance []StorageAttachment

func (b byStorageInstance) Len() int { return len(b) }

func (b byStorageInstance) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

func (b byStorageInstance) Less(i, j int) bool {
	return b[i].StorageInstance().String() < b[j].StorageInstance().String()
}

// storageParams returns parameters for creating volumes/filesystems
// and volume/filesystem attachments when a unit is instantiated.
func (u *Unit) storageParams() (*storageParams, error) {
	params, err := unitStorageParams(u)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, name := range u.doc.Subordinates {
		sub, err := u.st.Unit(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subParams, err := unitStorageParams(sub)
		if err != nil {
			return nil, errors.Trace(err)
		}
		params = combineStorageParams(params, subParams)
	}
	return params, nil
}

func unitStorageParams(u *Unit) (*storageParams, error) {
	sb, err := NewStorageBackend(u.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageAttachments, err := sb.UnitStorageAttachments(u.unitTag())
	if err != nil {
		return nil, errors.Annotate(err, "getting storage attachments")
	}
	ch, err := u.charm()
	if err != nil {
		return nil, errors.Annotate(err, "getting charm")
	}

	// Sort storage attachments so the volume ids are consistent (for testing).
	sort.Sort(byStorageInstance(storageAttachments))

	var storageInstances []*storageInstance
	for _, storageAttachment := range storageAttachments {
		storage, err := sb.storageInstance(storageAttachment.StorageInstance())
		if err != nil {
			return nil, errors.Annotatef(err, "getting storage instance")
		}
		storageInstances = append(storageInstances, storage)
	}
	return storageParamsForUnit(sb, storageInstances, u.unitTag(), u.base(), ch.Meta())
}

func storageParamsForUnit(
	sb *storageBackend, storageInstances []*storageInstance, tag names.UnitTag, base Base, chMeta *charm.Meta,
) (*storageParams, error) {

	var volumes []HostVolumeParams
	var filesystems []HostFilesystemParams
	volumeAttachments := make(map[names.VolumeTag]VolumeAttachmentParams)
	filesystemAttachments := make(map[names.FilesystemTag]FilesystemAttachmentParams)
	for _, storage := range storageInstances {
		storageParams, err := storageParamsForStorageInstance(
			sb, chMeta, base.OS, storage,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}

		volumes = append(volumes, storageParams.volumes...)
		for k, v := range storageParams.volumeAttachments {
			volumeAttachments[k] = v
		}

		filesystems = append(filesystems, storageParams.filesystems...)
		for k, v := range storageParams.filesystemAttachments {
			filesystemAttachments[k] = v
		}
	}
	result := &storageParams{
		volumes,
		volumeAttachments,
		filesystems,
		filesystemAttachments,
	}
	return result, nil
}

// storageParamsForStorageInstance returns parameters for creating
// volumes/filesystems and volume/filesystem attachments for a host that
// the unit will be assigned to. These parameters are based on a given storage
// instance.
func storageParamsForStorageInstance(
	sb *storageBackend,
	charmMeta *charm.Meta,
	osName string,
	storage *storageInstance,
) (*storageParams, error) {

	charmStorage := charmMeta.Storage[storage.StorageName()]

	var volumes []HostVolumeParams
	var filesystems []HostFilesystemParams
	volumeAttachments := make(map[names.VolumeTag]VolumeAttachmentParams)
	filesystemAttachments := make(map[names.FilesystemTag]FilesystemAttachmentParams)

	switch storage.Kind() {
	case StorageKindFilesystem:
		location, err := FilesystemMountPoint(charmStorage, storage.StorageTag(), osName)
		if err != nil {
			return nil, errors.Annotatef(
				err, "getting filesystem mount point for storage %s",
				storage.StorageName(),
			)
		}
		filesystemAttachmentParams := FilesystemAttachmentParams{
			locationAutoGenerated: charmStorage.Location == "", // auto-generated location
			Location:              location,
			ReadOnly:              charmStorage.ReadOnly,
		}
		var volumeBacked bool
		if filesystem, err := sb.StorageInstanceFilesystem(storage.StorageTag()); err == nil {
			// The filesystem already exists, so just attach it.
			// When creating ops to attach the storage to the
			// machine, we will check if the attachment already
			// exists, and whether the storage can be attached to
			// the machine.
			if !charmStorage.Shared {
				// The storage is not shared, so make sure that it is
				// not currently attached to any other machine. If it
				// is, it should be in the process of being detached.
				existing, err := sb.FilesystemAttachments(filesystem.FilesystemTag())
				if err != nil {
					return nil, errors.Trace(err)
				}
				if len(existing) > 0 {
					return nil, errors.Errorf(
						"%s is attached to %s",
						names.ReadableString(filesystem.FilesystemTag()),
						names.ReadableString(existing[0].Host()),
					)
				}
			}
			filesystemAttachments[filesystem.FilesystemTag()] = filesystemAttachmentParams
			if _, err := filesystem.Volume(); err == nil {
				// The filesystem is volume-backed, so make sure we attach the volume too.
				volumeBacked = true
			}
		} else if errors.Is(err, errors.NotFound) {
			filesystemParams := FilesystemParams{
				storage: storage.StorageTag(),
				Pool:    storage.doc.Constraints.Pool,
				Size:    storage.doc.Constraints.Size,
			}
			filesystems = append(filesystems, HostFilesystemParams{
				filesystemParams, filesystemAttachmentParams,
			})
		} else {
			return nil, errors.Annotatef(err, "getting filesystem for storage %q", storage.Tag().Id())
		}

		if !volumeBacked {
			break
		}
		// Fall through to attach the volume that backs the filesystem.
		fallthrough

	case StorageKindBlock:
		volumeAttachmentParams := VolumeAttachmentParams{
			charmStorage.ReadOnly,
		}
		if volume, err := sb.StorageInstanceVolume(storage.StorageTag()); err == nil {
			// The volume already exists, so just attach it. When
			// creating ops to attach the storage to the machine,
			// we will check if the attachment already exists, and
			// whether the storage can be attached to the machine.
			if !charmStorage.Shared {
				// The storage is not shared, so make sure that it is
				// not currently attached to any other machine. If it
				// is, it should be in the process of being detached.
				existing, err := sb.VolumeAttachments(volume.VolumeTag())
				if err != nil {
					return nil, errors.Trace(err)
				}
				if len(existing) > 0 {
					return nil, errors.Errorf(
						"%s is attached to %s",
						names.ReadableString(volume.VolumeTag()),
						names.ReadableString(existing[0].Host()),
					)
				}
			}
			volumeAttachments[volume.VolumeTag()] = volumeAttachmentParams
		} else if errors.Is(err, errors.NotFound) {
			volumeParams := VolumeParams{
				storage: storage.StorageTag(),
				Pool:    storage.doc.Constraints.Pool,
				Size:    storage.doc.Constraints.Size,
			}
			volumes = append(volumes, HostVolumeParams{
				volumeParams, volumeAttachmentParams,
			})
		} else {
			return nil, errors.Annotatef(err, "getting volume for storage %q", storage.Tag().Id())
		}
	default:
		return nil, errors.Errorf("invalid storage kind %v", storage.Kind())
	}
	result := &storageParams{
		volumes,
		volumeAttachments,
		filesystems,
		filesystemAttachments,
	}
	return result, nil
}

var hasNoContainersTerm = bson.DocElem{
	"$or", []bson.D{
		{{"children", bson.D{{"$size", 0}}}},
		{{"children", bson.D{{"$exists", false}}}},
	}}

// ActionSpecsByName is a map of action names to their respective ActionSpec.
type ActionSpecsByName map[string]charm.ActionSpec

// PrepareActionPayload returns the payload to use in creating an action for this unit.
// Note that the use of spec.InsertDefaults mutates payload.
func (u *Unit) PrepareActionPayload(name string, payload map[string]interface{}, parallel *bool, executionGroup *string) (map[string]interface{}, bool, string, error) {
	if len(name) == 0 {
		return nil, false, "", errors.New("no action name given")
	}

	// If the action is predefined inside juju, get spec from map
	spec, ok := actions.PredefinedActionsSpec[name]
	if !ok {
		specs, err := u.ActionSpecs()
		if err != nil {
			return nil, false, "", err
		}
		spec, ok = specs[name]
		if !ok {
			return nil, false, "", errors.Errorf("action %q not defined on unit %q", name, u.name())
		}
	}
	// Reject bad payloads before attempting to insert defaults.
	err := spec.ValidateParams(payload)
	if err != nil {
		return nil, false, "", errors.Trace(err)
	}
	payloadWithDefaults, err := spec.InsertDefaults(payload)
	if err != nil {
		return nil, false, "", errors.Trace(err)
	}

	runParallel := spec.Parallel
	if parallel != nil {
		runParallel = *parallel
	}
	runExecutionGroup := spec.ExecutionGroup
	if executionGroup != nil {
		runExecutionGroup = *executionGroup
	}
	return payloadWithDefaults, runParallel, runExecutionGroup, nil
}

// ActionSpecs gets the ActionSpec map for the Unit's charm.
func (u *Unit) ActionSpecs() (ActionSpecsByName, error) {
	none := ActionSpecsByName{}
	ch, err := u.charm()
	if err != nil {
		return none, errors.Trace(err)
	}
	chActions := ch.Actions()
	if chActions == nil || len(chActions.ActionSpecs) == 0 {
		return none, errors.Errorf("no actions defined on charm %q", ch.URL())
	}
	return chActions.ActionSpecs, nil
}

// CancelAction removes a pending Action from the queue for this
// ActionReceiver and marks it as cancelled.
func (u *Unit) CancelAction(action Action) (Action, error) {
	return action.Finish(ActionResults{Status: ActionCancelled})
}

// WatchActionNotifications starts and returns a StringsWatcher that
// notifies when actions with Id prefixes matching this Unit are added
func (u *Unit) WatchActionNotifications() StringsWatcher {
	return u.st.watchActionNotificationsFilteredBy(u)
}

// WatchPendingActionNotifications is part of the ActionReceiver interface.
func (u *Unit) WatchPendingActionNotifications() StringsWatcher {
	return u.st.watchEnqueuedActionsFilteredBy(u)
}

// Actions returns a list of actions pending or completed for this unit.
func (u *Unit) Actions() ([]Action, error) {
	return u.st.matchingActions(u)
}

// CompletedActions returns a list of actions that have finished for
// this unit.
func (u *Unit) CompletedActions() ([]Action, error) {
	return u.st.matchingActionsCompleted(u)
}

// PendingActions returns a list of actions pending for this unit.
func (u *Unit) PendingActions() ([]Action, error) {
	return u.st.matchingActionsPending(u)
}

// RunningActions returns a list of actions running on this unit.
func (u *Unit) RunningActions() ([]Action, error) {
	return u.st.matchingActionsRunning(u)
}

// storageConstraints returns the unit's storage constraints.
func (u *Unit) storageConstraints() (map[string]StorageConstraints, error) {
	if u.doc.CharmURL == nil {
		app, err := u.st.Application(u.doc.Application)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return app.StorageConstraints()
	}
	key := applicationStorageConstraintsKey(u.doc.Application, u.doc.CharmURL)
	cons, err := readStorageConstraints(u.st, key)
	if errors.Is(err, errors.NotFound) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return cons, nil
}

type addUnitOpsArgs struct {
	unitDoc           *unitDoc
	containerDoc      *cloudContainerDoc
	agentStatusDoc    statusDoc
	workloadStatusDoc *statusDoc
}

// addUnitOps returns the operations required to add a unit to the units
// collection, along with all the associated expected other unit entries. This
// method is used by both the *Application.addUnitOpsWithCons method and the
// migration import code.
func addUnitOps(st *State, args addUnitOpsArgs) ([]txn.Op, error) {
	name := args.unitDoc.Name
	agentGlobalKey := unitAgentGlobalKey(name)

	// TODO: consider the constraints op
	// TODO: consider storageOps
	var prereqOps []txn.Op
	if args.containerDoc != nil {
		prereqOps = append(prereqOps, txn.Op{
			C:      cloudContainersC,
			Id:     args.containerDoc.Id,
			Insert: args.containerDoc,
			Assert: txn.DocMissing,
		})
	}
	prereqOps = append(prereqOps,
		createStatusOp(st, agentGlobalKey, args.agentStatusDoc),
		createStatusOp(st, unitGlobalKey(name), *args.workloadStatusDoc),
	)

	return append(prereqOps, txn.Op{
		C:      unitsC,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: args.unitDoc,
	}), nil
}
