// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

import (
	"github.com/juju/go-oracle-cloud/common"
)

// StorageAttachment is a storage attachment is an association
// between a storage volume and an instance. You can associate
// a volume to only one instance at a time. You can detach a
// volume from an instance by deleting the relevant storage attachment
type StorageAttachment struct {

	// Account shows the default account for your identity domain
	Account *string `json:"account,omitempty"`

	// Hypervisor type to which this volume was attached to
	Hypervisor *string `json:"hypervisor,omitempty"`

	// Index number for the volume.
	// The allowed range is 1-10.
	// The index determines the device name by which the
	// volume is exposed to the instance. Index 0 is allocated
	// to the temporary boot disk, /dev/xvda An attachment with
	// index 1 is exposed to the instance as /dev/xvdb,
	// an attachment with index 2 is exposed as /dev/xvdc, and so on
	Index common.Index `json:"index"`

	// Instance_name multipart name of the instance
	// to which you want to attach the volume
	Instance_name string `json:"instance_name"`

	// Storage_volume_name is the name of the storage volume
	Storage_volume_name string `json:"storage_volume_name"`

	Name string `json:"name"`

	// Readonly when set to true, it indicates
	// that the volume is a read-only storage volume
	Readonly bool `json:"readonly"`

	// State specifies one of the following states of the storage attachment:
	// common.StateAttaching: The storage attachment is in
	// the process of attaching to the instance.
	// common.StateAttached: The storage attachment is
	// attached to the instance
	// common.StateDetaching: The storage attachment is
	// in the process of detaching from the instance
	// common.stateUnavailable: The storage attachment is unavailable
	// common.StateUnknown: The state of the storage attachment is not known
	State common.StateStorage `json:"state"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`
}

type AllStorageAttachments struct {
	Result []StorageAttachment `json:"result,omitempty"`
}
