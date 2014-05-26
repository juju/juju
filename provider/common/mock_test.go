// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"io"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/tools"
)

type allInstancesFunc func() ([]instance.Instance, error)
type startInstanceFunc func(string, constraints.Value, []string, []string, tools.List, *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error)
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

func (*mockEnviron) Name() string {
	return "mock environment"
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
		args.MachineConfig.IncludeNetworks,
		args.MachineConfig.ExcludeNetworks,
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

type mockInstance struct {
	id                string
	addresses         []instance.Address
	addressesErr      error
	dnsName           string
	dnsNameErr        error
	instance.Instance // stub out other methods with panics
}

func (inst *mockInstance) Id() instance.Id {
	return instance.Id(inst.id)
}

func (inst *mockInstance) Addresses() ([]instance.Address, error) {
	return inst.addresses, inst.addressesErr
}

func (inst *mockInstance) DNSName() (string, error) {
	return inst.dnsName, inst.dnsNameErr
}

func (inst *mockInstance) WaitDNSName() (string, error) {
	return common.WaitDNSName(inst)
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
