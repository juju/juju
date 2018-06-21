// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package devices

// DeviceType defines a device type.
type DeviceType string

// Constraints describes a set of device constraints.
type Constraints struct {

	// Type is the device type.
	// currently supported types are
	// - gpu
	// - nvidia.com/gpu
	// - amd.com/gpu
	Type DeviceType `bson:"type"`

	// Count is the number of devices that the user has asked for - count min and max are the
	// number of devices the charm requires.
	Count int64 `bson:"count"`
}

// TODO (ycliuhw): add parsers later in cmd flag card
