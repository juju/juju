// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// BlockDevice represents the state of a block device in the environment.
type BlockDevice interface {
	// Machine returns the ID of the machine the block device is attached to.
	Machine() string

	// Info returns the block device's BlockDeviceInfo.
	Info() BlockDeviceInfo
}

// blockDevicesDoc records information about a machine's block devices.
type blockDevicesDoc struct {
	DocID        string            `bson:"_id"`
	EnvUUID      string            `bson:"env-uuid"`
	Machine      string            `bson:"machineid"`
	BlockDevices []BlockDeviceInfo `bson:"blockdevices"`
}

// BlockDeviceInfo describes information about a block device.
type BlockDeviceInfo struct {
	DeviceName     string `bson:"devicename"`
	Label          string `bson:"label,omitempty"`
	UUID           string `bson:"uuid,omitempty"`
	Serial         string `bson:"serial,omitempty"`
	Size           uint64 `bson:"size"`
	FilesystemType string `bson:"fstype,omitempty"`
	InUse          bool   `bson:"inuse"`
	MountPoint     string `bson:"mountpoint,omitempty"`
}

// WatchBlockDevices returns a new NotifyWatcher watching for
// changes to block devices associated with the specified machine.
func (st *State) WatchBlockDevices(machine names.MachineTag) NotifyWatcher {
	return newBlockDevicesWatcher(st, machine.Id())
}

// BlockDevices returns the BlockDeviceInfo for the specified machine.
func (st *State) BlockDevices(machine names.MachineTag) ([]BlockDeviceInfo, error) {
	return st.blockDevices(machine.Id())
}

func (st *State) blockDevices(machineId string) ([]BlockDeviceInfo, error) {
	coll, cleanup := st.getCollection(blockDevicesC)
	defer cleanup()

	var d blockDevicesDoc
	err := coll.FindId(machineId).One(&d)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("block devices not found for machine %q", machineId)
	} else if err != nil {
		return nil, errors.Annotate(err, "cannot get block device details")
	}
	return d.BlockDevices, nil
}

// setMachineBlockDevices updates the blockdevices collection with the
// currently attached block devices. Previously recorded block devices
// not in the list will be removed.
func setMachineBlockDevices(st *State, machineId string, newInfo []BlockDeviceInfo) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		oldInfo, err := st.blockDevices(machineId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !blockDevicesChanged(oldInfo, newInfo) {
			return nil, jujutxn.ErrNoOperations
		}
		// TODO(axw) before the storage feature can come off,
		// we need to add an upgrade step to add a block
		// devices doc to existing machines.
		ops := []txn.Op{{
			C:      machinesC,
			Id:     machineId,
			Assert: isAliveDoc,
		}, {
			C:      blockDevicesC,
			Id:     machineId,
			Assert: bson.D{{"blockdevices", oldInfo}},
			Update: bson.D{{"$set", bson.D{{"blockdevices", newInfo}}}},
		}}
		return ops, nil
	}
	return st.run(buildTxn)
}

func createMachineBlockDevicesOp(machineId string) txn.Op {
	return txn.Op{
		C:      blockDevicesC,
		Id:     machineId,
		Insert: &blockDevicesDoc{Machine: machineId},
	}
}

func removeMachineBlockDevicesOp(machineId string) txn.Op {
	return txn.Op{
		C:      blockDevicesC,
		Id:     machineId,
		Remove: true,
	}
}

func blockDevicesChanged(oldDevices, newDevices []BlockDeviceInfo) bool {
	if len(oldDevices) != len(newDevices) {
		return true
	}
	for _, o := range oldDevices {
		var found bool
		for _, n := range newDevices {
			if o == n {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
}
