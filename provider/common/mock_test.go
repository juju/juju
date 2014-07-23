// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"io"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools"
)

type allInstancesFunc func() ([]instance.Instance, error)
type startInstanceFunc func(string, constraints.Value, []string, tools.List, *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error)
type stopInstancesFunc func([]instance.Id) error
type getToolsSourcesFunc func() ([]simplestreams.DataSource, error)
type configFunc func() *config.Config
type setConfigFunc func(*config.Config) error

type mockEnviron struct {
	storage          storage.Storage
	allInstances     allInstancesFunc
	startInstance    startInstanceFunc
	stopInstances    stopInstancesFunc
	getToolsSources  getToolsSourcesFunc
	config           configFunc
	setConfig        setConfigFunc
	environs.Environ // stub out other methods with panics
}

func (*mockEnviron) SupportedArchitectures() ([]string, error) {
	return []string{"amd64", "arm64"}, nil
}

func (env *mockEnviron) Storage() storage.Storage {
	return env.storage
}

func (env *mockEnviron) AllInstances() ([]instance.Instance, error) {
	return env.allInstances()
}
func (env *mockEnviron) StartInstance(args environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {
	return env.startInstance(
		args.Placement,
		args.Constraints,
		args.MachineConfig.Networks,
		args.Tools,
		args.MachineConfig)
}

func (env *mockEnviron) StopInstances(ids ...instance.Id) error {
	return env.stopInstances(ids)
}

func (env *mockEnviron) Config() *config.Config {
	return env.config()
}

func (env *mockEnviron) SetConfig(cfg *config.Config) error {
	if env.setConfig != nil {
		return env.setConfig(cfg)
	}
	return nil
}

func (env *mockEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	if env.getToolsSources != nil {
		return env.getToolsSources()
	}
	datasource := storage.NewStorageSimpleStreamsDataSource("test cloud storage", env.Storage(), storage.BaseToolsPath)
	return []simplestreams.DataSource{datasource}, nil
}

func (env *mockEnviron) GetImageSources() ([]simplestreams.DataSource, error) {
	datasource := storage.NewStorageSimpleStreamsDataSource("test cloud storage", env.Storage(), storage.BaseImagesPath)
	return []simplestreams.DataSource{datasource}, nil
}

type availabilityZonesFunc func() ([]common.AvailabilityZone, error)
type instanceAvailabilityZoneNamesFunc func([]instance.Id) ([]string, error)

type mockZonedEnviron struct {
	mockEnviron
	availabilityZones             availabilityZonesFunc
	instanceAvailabilityZoneNames instanceAvailabilityZoneNamesFunc
}

func (env *mockZonedEnviron) AvailabilityZones() ([]common.AvailabilityZone, error) {
	return env.availabilityZones()
}

func (env *mockZonedEnviron) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	return env.instanceAvailabilityZoneNames(ids)
}

type mockInstance struct {
	id                string
	addresses         []network.Address
	addressesErr      error
	dnsName           string
	dnsNameErr        error
	instance.Instance // stub out other methods with panics
}

func (inst *mockInstance) Id() instance.Id {
	return instance.Id(inst.id)
}

func (inst *mockInstance) Addresses() ([]network.Address, error) {
	return inst.addresses, inst.addressesErr
}

type mockStorage struct {
	storage.Storage
	putErr       error
	removeAllErr error
}

func (stor *mockStorage) Put(name string, reader io.Reader, size int64) error {
	if stor.putErr != nil {
		return stor.putErr
	}
	return stor.Storage.Put(name, reader, size)
}

func (stor *mockStorage) RemoveAll() error {
	if stor.removeAllErr != nil {
		return stor.removeAllErr
	}
	return stor.Storage.RemoveAll()
}

type mockAvailabilityZone struct {
	name      string
	available bool
}

func (z *mockAvailabilityZone) Name() string {
	return z.name
}

func (z *mockAvailabilityZone) Available() bool {
	return z.available
}
