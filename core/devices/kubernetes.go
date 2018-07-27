// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package devices

// KubernetesDeviceParams is a fully specified set of parameters for devices allocation,
// derived from one or more of user-specified device constraints, a
// device definition, and charm device metadata.
type KubernetesDeviceParams struct {
	// Type is the device type or device-class.
	Type DeviceType `bson:"type"`

	// Count is the number of devices that the user has asked for - count min and max are the
	// number of devices the charm requires.
	Count int64 `bson:"count"`

	// Attributes is a collection of key value pairs device related (node affinity labels/tags etc.).
	Attributes map[string]string `bson:"attributes"`
}
