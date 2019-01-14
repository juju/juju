// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import "github.com/juju/clock"

var (
	InstanceTypes          = instanceTypes
	RefreshImageCache      = refreshImageCache
	FindInstanceSpec       = findInstanceSpec
	GetImageType           = getImageType
	ShapeSpecs             = shapeSpecs
	SetImageCache          = setImageCache
	NewInstance            = newInstance
	MaxPollIterations      = &maxPollIterations
	PollTime               = &pollTime
	AllProtocols           = allProtocols
	OciStorageProviderType = ociStorageProviderType
	OciVolumeType          = ociVolumeType
	IscsiPool              = iscsiPool
)

func (e *Environ) SetClock(clock clock.Clock) {
	e.clock = clock
}
