package oci

import "github.com/juju/utils/clock"

var (
	InstanceTypes     = instanceTypes
	RefreshImageCache = refreshImageCache
	FindInstanceSpec  = findInstanceSpec
	GetImageType      = getImageType
	ShapeSpecs        = shapeSpecs
	SetImageCache     = setImageCache
	NewInstance       = newInstance
	MaxPollIterations = &maxPollIterations
	PollTime          = &pollTime
	AllProtocols      = allProtocols
)

func (e *Environ) SetClock(clock clock.Clock) {
	e.clock = clock
}
