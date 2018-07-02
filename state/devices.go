// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// DeviceType defines a device type.
type DeviceType string

// DeviceConstraints contains the user-specified constraints for allocating
// device instances for an application unit.
type DeviceConstraints struct {

	// Type is the device type or device-class.
	// currently supported types are
	// - gpu
	// - nvidia.com/gpu
	// - amd.com/gpu
	Type DeviceType `bson:"type"`

	// Count is the number of devices that the user has asked for - count min and max are the
	// number of devices the charm requires.
	Count int64 `bson:"count"`

	// Attributes is a collection of key value pairs device related (node affinity labels/tags etc.).
	Attributes map[string]string `bson:"attributes"`
}

// TODO(ycliuhw): DeviceConstraintsDoc etc will be added later, here add DeviceConstraints for testing client
