// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	"github.com/juju/retry"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/tools"
)

// unitAgentGlobalKey returns the global database key for the named unit.
func unitAgentGlobalKey(name string) string {
	return unitAgentGlobalKeyPrefix + name
}

// unitAgentGlobalKeyPrefix is the string we use to denote unit agent kind.
const unitAgentGlobalKeyPrefix = "u#"

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

// life returns whether the unit is Alive, Dying or Dead.
func (u *Unit) life() Life {
	return u.doc.Life
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

// ActionSpecsByName is a map of action names to their respective ActionSpec.
type ActionSpecsByName map[string]charm.ActionSpec

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
	unitDoc      *unitDoc
	containerDoc *cloudContainerDoc
}

// addUnitOps returns the operations required to add a unit to the units
// collection, along with all the associated expected other unit entries. This
// method is used by both the *Application.addUnitOpsWithCons method and the
// migration import code.
func addUnitOps(st *State, args addUnitOpsArgs) ([]txn.Op, error) {
	name := args.unitDoc.Name

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

	return append(prereqOps, txn.Op{
		C:      unitsC,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: args.unitDoc,
	}), nil
}
