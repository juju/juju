// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"github.com/juju/clock"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/provider/common"
	"github.com/oracle/oci-go-sdk/v47/core"
)

var (
	InstanceTypes          = instanceTypes
	RefreshImageCache      = refreshImageCache
	ShapeSpecs             = shapeSpecs
	SetImageCache          = setImageCache
	NewInstance            = newInstance
	MaxPollIterations      = &maxPollIterations
	AllProtocols           = allProtocols
	OciStorageProviderType = ociStorageProviderType
	OciVolumeType          = ociVolumeType
	IscsiPool              = iscsiPool
)

func (e *Environ) SetClock(clock clock.Clock) {
	e.clock = clock
}

func NewInstanceWithConfigurator(
	raw core.Instance, env *Environ, factory func(string) common.InstanceConfigurator,
) (*ociInstance, error) {
	i, err := newInstance(raw, env)
	if err != nil {
		return nil, err
	}

	i.newInstanceConfigurator = factory
	return i, nil
}

func GetCloudInitConfig(env *Environ, series string, apiPort int, statePort int) (cloudinit.CloudConfig, error) {
	return env.getCloudInitConfig(series, apiPort, statePort)
}
