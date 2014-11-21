// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/errors"
	"github.com/juju/juju/storage"
	"github.com/juju/names"
)

// diskDoc records information about a disk attached to a machine.
type diskDoc struct {
	DocID   string `bson:"_id"`
	Name    string `bson:"name"`
	EnvUUID string `bson:"env-uuid"`
	Machine string `bson:"machine"`
}

type blockDevicesDoc struct {
	Id                  string        `bson:"_id"`
	EnvUUID             string        `bson:"env-uuid"`
	MachineId           string        `bson:"machineid"`
	MachineBlockDevices []blockDevice `bson:"machineblockdevices"`
	Seqno               int           `bson:"seqno"`
}

type blockDevice struct {
	Id          string `bson:"id"`
	DeviceName  string `bson:"devicename,omitempty"`
	Label       string `bson:"label,omitempty"`
	UUID        string `bson:"uuid,omitempty"`
	Serial      string `bson:"serial,omitempty"`
	Size        uint64 `bson:"size"`
	InUse       bool   `bson:"inuse"`
	DatastoreId string `bson:"datastoreid,omitempty"`
}

func BlockDeviceDatastore(st *State, diskName string) (storage.Datastore, error) {
	if !names.IsValidDisk(diskName) {
		return storage.Datastore{}, errors.Errorf("%q is not a valid disk name")
	}
	doc, err := getBlockDevicesDoc(st, names.DiskMachine(diskName))
	if err != nil {
		return storage.Datastore{}, errors.Annotate(err, "cannot get block devices")
	}
	// TODO(axw) convert machineblockdevices to map to speed this up.
	for _, dev := range doc.MachineBlockDevices {
		if dev.Id == diskName {

		}
	}
	//datastoreId, err :=
}

func newBlockDevicesDoc(st *State, machineId string) *blockDevicesDoc {
	return &blockDevicesDoc{
		Id:        st.docID(machineId),
		EnvUUID:   st.EnvironUUID(),
		MachineId: machineId,
	}
}

func getBlockDevicesDoc(st *State, machineId string) (*blockDevicesDoc, error) {
	coll, cleanup := st.getCollection(blockDevicesC)
	defer cleanup()

	var doc blockDevicesDoc
	if err := coll.FindId(machineId).One(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// newDiskName returns the next disk name for a machine.
func newDiskName(st *State, machineId string) (string, error) {
	blockDevices, closer := st.getCollection(blockDevicesC)
	defer closer()

	change := mgo.Change{Update: bson.D{{"$inc", bson.D{{"seqno", 1}}}}}
	var doc blockDevicesDoc
	_, err := blockDevices.Find(bson.D{{"_id", st.docID(machineId)}}).Apply(change, &doc)
	if err == mgo.ErrNotFound {
		// FIXME(axw) we need to ensure the doc exists so we can get sequences.
		// We can just do it as an upgrade step, rather than the insert-or-update
		// in setMachineBlockDevices.
		panic("TODO")
	} else if err != nil {
		return "", errors.Annotate(err, "cannot increment block device ID sequence")
	}
	return fmt.Sprintf("%s#%d", machineId, doc.Seqno), nil
}

func setMachineBlockDevices(st *State, machineId string, devices []storage.BlockDevice) error {
	// TODO(axw) ensure devices do not have Id fields set.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		doc, err := getBlockDevicesDoc(st, machineId)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}

		// Merge new and old knowledge. Carry over old IDs, and
		// generate new IDs for previously unseen block devices.
		var oldDevices []storage.BlockDevice
		if doc != nil {
			oldDevices = toBlockDevices(doc.MachineBlockDevices)
		}
		newDevices := fromBlockDevices(devices)
		for i, dev := range newDevices {
			for j, old := range oldDevices {
				if storage.BlockDevicesSame(old, devices[i]) {
					dev = mergeBlockDevices(doc.MachineBlockDevices[j], dev)
					dev.Id = string(old.Id)
					break
				}
			}
			if dev.Id == "" {
				name, err := newDiskName(st, machineId)
				if err != nil {
					return nil, errors.Annotate(err, "cannot generate block device ID")
				}
				dev.Id = name
			}
			newDevices[i] = dev
		}

		var ops []txn.Op
		if err == nil {
			ops = []txn.Op{{
				C:      blockDevicesC,
				Id:     doc.Id,
				Assert: bson.D{{"machineblockdevices", doc.MachineBlockDevices}},
				Update: bson.D{{"$set", bson.D{{"machineblockdevices", newDevices}}}},
			}}
		} else {
			doc = newBlockDevicesDoc(st, machineId)
			doc.MachineBlockDevices = fromBlockDevices(devices)
			ops = []txn.Op{{
				C:      blockDevicesC,
				Id:     doc.Id,
				Assert: txn.DocMissing,
				Insert: doc,
			}}
		}
		return ops, nil
	}
	return st.run(buildTxn)
}

func getBlockDevices(st *State, machineId string) ([]blockDevice, error) {
	coll, cleanup := st.getCollection(blockDevicesC)
	defer cleanup()

	var doc blockDevicesDoc
	err := coll.FindId(machineId).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Annotate(err, "cannot get block devices")
	}
	return doc.MachineBlockDevices, nil
}

func removeBlockDevicesOp(st *State, machineId string) txn.Op {
	return txn.Op{
		C:      blockDevicesC,
		Id:     st.docID(machineId),
		Remove: true,
	}
}

func fromBlockDevices(devices []storage.BlockDevice) []blockDevice {
	subdocs := make([]blockDevice, len(devices))
	for i, device := range devices {
		subdocs[i] = blockDevice{
			Id:         string(device.Id),
			DeviceName: device.DeviceName,
			Label:      device.Label,
			UUID:       device.UUID,
			Size:       device.Size,
			InUse:      device.InUse,
		}
	}
	return subdocs
}

func toBlockDevices(subdocs []blockDevice) []storage.BlockDevice {
	devices := make([]storage.BlockDevice, len(subdocs))
	for i, subdoc := range subdocs {
		devices[i] = storage.BlockDevice{
			Id:         storage.BlockDeviceId(subdoc.Id),
			DeviceName: subdoc.DeviceName,
			Label:      subdoc.Label,
			UUID:       subdoc.UUID,
			Size:       subdoc.Size,
			InUse:      subdoc.InUse,
		}
	}
	return devices
}

func blockDevicesEqual(a, b []blockDevice) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// mergeBlockDevices merges the information of two block device structures.
//
// If both structures have information for a field, the second structure's
// is preferred. Size and InUse are always taken from the second structure.
func mergeBlockDevices(a, b blockDevice) blockDevice {
	if b.DeviceName != "" {
		a.DeviceName = b.DeviceName
	}
	if b.Label != "" {
		a.Label = b.Label
	}
	if b.UUID != "" {
		a.UUID = b.UUID
	}
	if b.Serial != "" {
		a.Serial = b.Serial
	}
	if b.DatastoreId != "" {
		a.DatastoreId = b.DatastoreId
	}
	a.Size = b.Size
	a.InUse = b.InUse
	return a
}
