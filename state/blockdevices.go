// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/storage"
)

// BlockDevice represents the state of a block device attached to a machine.
type BlockDevice interface {
	// Tag returns the tag for the block device.
	Tag() names.Tag

	// Name returns the unique name of the block device.
	Name() string

	// Machine returns the ID of the machine the block device is attached to.
	Machine() string

	// Info returns the block device's BlockDeviceInfo, or a NotProvisioned
	// error if the block device has not yet been provisioned.
	Info() (BlockDeviceInfo, error)

	// Params returns the parameters for provisioning the block device,
	// if it has not already been provisioned. Params returns true if the
	// returned parameters are usable for provisioning, otherwise false.
	Params() (BlockDeviceParams, bool)
}

type blockDevice struct {
	doc blockDeviceDoc
}

// blockDeviceDoc records information about a disk attached to a machine.
type blockDeviceDoc struct {
	DocID   string             `bson:"_id"`
	Name    string             `bson:"name"`
	EnvUUID string             `bson:"env-uuid"`
	Machine string             `bson:"machine"`
	Info    *BlockDeviceInfo   `bson:"info,omitempty"`
	Params  *BlockDeviceParams `bson:"params,omitempty"`
}

// BlockDeviceParams records parameters for provisioning a new block device.
type BlockDeviceParams struct {
	Size uint64 `bson:"size"`
}

// BlockDeviceInfo describes information about a block device.
type BlockDeviceInfo struct {
	DeviceName string `bson:"devicename,omitempty"`
	Label      string `bson:"label,omitempty"`
	UUID       string `bson:"uuid,omitempty"`
	Serial     string `bson:"serial,omitempty"`
	Size       uint64 `bson:"size"`
	InUse      bool   `bson:"inuse"`
}

func (b *blockDevice) Tag() names.Tag {
	return names.NewDiskTag(b.doc.Name)
}

func (b *blockDevice) Name() string {
	return b.doc.Name
}

func (b *blockDevice) Machine() string {
	return b.doc.Machine
}

func (b *blockDevice) Info() (BlockDeviceInfo, error) {
	if b.doc.Info == nil {
		return BlockDeviceInfo{}, errors.NotProvisionedf("block device %q", b.doc.Name)
	}
	return *b.doc.Info, nil
}

func (b *blockDevice) Params() (BlockDeviceParams, bool) {
	if b.doc.Params == nil {
		return BlockDeviceParams{}, false
	}
	return *b.doc.Params, true
}

// BlockDevice returns the BlockDevice with the specified name.
func (st *State) BlockDevice(diskName string) (BlockDevice, error) {
	blockDevices, cleanup := st.getCollection(blockDevicesC)
	defer cleanup()

	var d blockDevice
	err := blockDevices.FindId(diskName).One(&d.doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("block device %q", diskName)
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get block device details")
	}
	return &d, nil
}

// BlockDeviceParams converts a storage.Constraints and optional charm
// and store name pair into one or more BlockDeviceParams.
func (st *State) BlockDeviceParams(cons storage.Constraints, ch *Charm, store string) ([]BlockDeviceParams, error) {
	if ch != nil || store != "" {
		return nil, errors.NotImplementedf("charm storage metadata")
	}
	if cons.Pool != "" {
		return nil, errors.NotImplementedf("storage pools")
	}
	if cons.Size == 0 {
		return nil, errors.Errorf("invalid size %v", cons.Size)
	}
	if cons.Count == 0 {
		return nil, errors.Errorf("invalid count %v", cons.Count)
	}
	// TODO(axw) if specified, validate constraints against charm storage metadata.
	params := make([]BlockDeviceParams, cons.Count)
	for i := range params {
		params[i].Size = cons.Size
	}
	return params, nil
}

// newDiskName returns a unique disk name.
func newDiskName(st *State) (string, error) {
	seq, err := st.sequence("disk")
	if err != nil {
		return "", errors.Trace(err)
	}
	return fmt.Sprint(seq), nil
}

// setMachineBlockDevices updates the blockdevices collection with the
// currently attached block devices. Previously recorded block devices not in
// the list will be removed.
//
// The Name field of each BlockDevice is ignored, if specified. Block devices
// are matched according to their identifying attributes (device name, UUID,
// etc.), and the existing Name will be retained.
func setMachineBlockDevices(st *State, machineId string, newInfo []BlockDeviceInfo) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		oldDevices, err := getMachineBlockDevices(st, machineId)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}

		ops := []txn.Op{{
			C:      machinesC,
			Id:     st.docID(machineId),
			Assert: isAliveDoc,
		}}

		// Create ops to update and remove existing block devices.
		found := make([]bool, len(newInfo))
		for _, oldDev := range oldDevices {
			oldInfo, err := oldDev.Info()
			if err != nil && errors.IsNotProvisioned(err) {
				// Leave unprovisioned block devices alone.
				continue
			} else if err != nil {
				return nil, errors.Trace(err)
			}
			var updated bool
			for j, newInfo := range newInfo {
				if found[j] {
					continue
				}
				if blockDevicesSame(oldInfo, newInfo) {
					// Merge the two structures by replacing the old document's
					// BlockDeviceInfo with the new one.
					if oldInfo != newInfo {
						ops = append(ops, txn.Op{
							C:      blockDevicesC,
							Id:     oldDev.doc.DocID,
							Assert: txn.DocExists,
							Update: bson.D{{"$set", bson.D{
								{"info", newInfo},
							}}},
						})
					}
					found[j] = true
					updated = true
					break
				}
			}
			if !updated {
				ops = append(ops, txn.Op{
					C:      blockDevicesC,
					Id:     oldDev.doc.DocID,
					Assert: txn.DocExists,
					Remove: true,
				})
			}
		}

		// Create ops to insert new block devices.
		for i, info := range newInfo {
			if found[i] {
				continue
			}
			name, err := newDiskName(st)
			if err != nil {
				return nil, errors.Annotate(err, "cannot generate disk name")
			}
			infoCopy := info // copy for the insert
			newDoc := blockDeviceDoc{
				Name:    name,
				Machine: machineId,
				EnvUUID: st.EnvironUUID(),
				DocID:   st.docID(name),
				Info:    &infoCopy,
			}
			ops = append(ops, txn.Op{
				C:      blockDevicesC,
				Id:     newDoc.DocID,
				Assert: txn.DocMissing,
				Insert: &newDoc,
			})
		}

		return ops, nil
	}
	return st.run(buildTxn)
}

// getMachineBlockDevices returns all of the block devices associated with the
// specified machine, including unprovisioned ones.
func getMachineBlockDevices(st *State, machineId string) ([]*blockDevice, error) {
	sel := bson.D{{"machine", machineId}}
	blockDevices, closer := st.getCollection(blockDevicesC)
	defer closer()

	var docs []blockDeviceDoc
	err := blockDevices.Find(sel).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	devices := make([]*blockDevice, len(docs))
	for i, doc := range docs {
		devices[i] = &blockDevice{doc}
	}
	return devices, nil
}

func removeMachineBlockDevicesOps(st *State, machineId string) ([]txn.Op, error) {
	sel := bson.D{{"machine", machineId}}
	blockDevices, closer := st.getCollection(blockDevicesC)
	defer closer()

	iter := blockDevices.Find(sel).Select(bson.D{{"_id", 1}}).Iter()
	defer iter.Close()
	var ops []txn.Op
	var doc blockDeviceDoc
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      blockDevicesC,
			Id:     doc.DocID,
			Remove: true,
		})
	}
	return ops, errors.Trace(iter.Close())
}

// setProvisionedBlockDeviceInfo sets the initial info for newly
// provisioned block devices. If non-empty, machineId must be the
// machine ID associated with the block devices.
func setProvisionedBlockDeviceInfo(st *State, machineId string, blockDevices map[string]BlockDeviceInfo) error {
	ops := make([]txn.Op, 0, len(blockDevices))
	for name, info := range blockDevices {
		infoCopy := info
		assert := bson.D{
			{"info", bson.D{{"$exists", false}}},
			{"params", bson.D{{"$exists", true}}},
		}
		if machineId != "" {
			assert = append(assert, bson.DocElem{"machine", machineId})
		}
		ops = append(ops, txn.Op{
			C:      blockDevicesC,
			Id:     name,
			Assert: assert,
			Update: bson.D{
				{"$set", bson.D{{"info", &infoCopy}}},
				{"$unset", bson.D{{"params", nil}}},
			},
		})
	}
	if err := st.runTransaction(ops); err != nil {
		return errors.Errorf("cannot set provisioned block device info: already provisioned")
	}
	return nil
}

// createMachineBlockDeviceOps creates txn.Ops to create unprovisioned
// block device documents associated with the specified machine, with
// the given parameters.
func createMachineBlockDeviceOps(st *State, machineId string, params ...BlockDeviceParams) (ops []txn.Op, names []string, err error) {
	ops = make([]txn.Op, len(params))
	names = make([]string, len(params))
	for i, params := range params {
		params := params
		name, err := newDiskName(st)
		if err != nil {
			return nil, nil, errors.Annotate(err, "cannot generate disk name")
		}
		ops[i] = txn.Op{
			C:      blockDevicesC,
			Id:     name,
			Assert: txn.DocMissing,
			Insert: &blockDeviceDoc{
				Name:    name,
				Machine: machineId,
				Params:  &params,
			},
		}
		names[i] = name
	}
	return ops, names, nil
}

// blockDevicesSame reports whether or not two BlockDevices identify the
// same block device.
//
// In descending order of preference, we use: serial number, filesystem
// UUID, device name.
func blockDevicesSame(a, b BlockDeviceInfo) bool {
	if a.Serial != "" && b.Serial != "" {
		return a.Serial == b.Serial
	}
	if a.UUID != "" && b.UUID != "" {
		return a.UUID == b.UUID
	}
	return a.DeviceName != "" && a.DeviceName == b.DeviceName
}
