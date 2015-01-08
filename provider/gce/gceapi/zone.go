// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"code.google.com/p/google-api-go-client/compute/v1"
)

type AvailabilityZone struct {
	zone *compute.Zone
}

func (z AvailabilityZone) Name() string {
	return z.zone.Name
}

func (z AvailabilityZone) Status() string {
	return z.zone.Status
}

func (z AvailabilityZone) Available() bool {
	// https://cloud.google.com/compute/docs/reference/latest/zones#status
	return z.Status() == StatusUp
}
