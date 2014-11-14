// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/errors"
	"github.com/juju/juju/storage"
)

// blockDevicesDoc records the block devices attached to a machine.
// There will be separate fields for each source of information
// about the devices (e.g. one for what the machine itself sees,
// and one for what has been allocated by the provider).
type blockDevicesDoc struct {
	Id                  string        `bson:"_id"`
	EnvUUID             string        `bson:"env-uuid"`
	MachineId           string        `bson:"machineid"`
	MachineBlockDevices []blockDevice `bson:"machineblockdevices"`
}

type blockDevice struct {
	DeviceName string `bson:"devicename,omitempty"`
	Label      string `bson:"label,omitempty"`
	UUID       string `bson:"uuid,omitempty"`
	Size       uint64 `bson:"size"`
	InUse      bool   `bson:"inuse"`
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
	if err := coll.FindId(st.docID(machineId)).One(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func setMachineBlockDevices(st *State, machineId string, devices []storage.BlockDevice) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		doc, err := getBlockDevicesDoc(st, machineId)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		var ops []txn.Op
		if err == nil {
			ops = []txn.Op{{
				C:      blockDevicesC,
				Id:     doc.Id,
				Assert: bson.D{{"machineblockdevices", doc.MachineBlockDevices}},
				Update: bson.D{{"$set", bson.D{{"machineblockdevices", fromBlockDevices(devices)}}}},
			}}
		} else if err == mgo.ErrNotFound {
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

func getBlockDevices(st *State, machineId string) ([]storage.BlockDevice, error) {
	coll, cleanup := st.getCollection(blockDevicesC)
	defer cleanup()

	var doc blockDevicesDoc
	err := coll.FindId(st.docID(machineId)).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return nil, errors.Annotate(err, "cannot get block devices")
	}
	return toBlockDevices(doc.MachineBlockDevices), nil
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
			DeviceName: subdoc.DeviceName,
			Label:      subdoc.Label,
			UUID:       subdoc.UUID,
			Size:       subdoc.Size,
			InUse:      subdoc.InUse,
		}
	}
	return devices
}
