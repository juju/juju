// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import "github.com/juju/juju/core/devices"

func makeDeviceCons(t devices.DeviceType, count int64, attributes map[string]string) devices.Constraints {
	return devices.Constraints{Type: t, Count: count, Attributes: attributes}
}
