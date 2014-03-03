// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"io"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/tools"
)

type allInstancesFunc func() ([]instance.Instance, error)
type startInstanceFunc func(constraints.Value, tools.List, *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error)
type stopInstancesFunc func([]instance.Instance) error
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

func (env *mockEnviron) Storage() storage.Storage {
	return env.storage
}

func (env *mockEnviron) AllInstances() ([]instance.Instance, error) {
	return env.allInstances()
}
func (env *mockEnviron) StartInstance(
	cons constraints.Value, possibleTools tools.List, mcfg *cloudinit.MachineConfig,
) (
	instance.Instance, *instance.HardwareCharacteristics, error,
) {
	return env.startInstance(cons, possibleTools, mcfg)
}

func (env *mockEnviron) StopInstances(instances []instance.Instance) error {
	return env.stopInstances(instances)
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

type mockInstance struct {
	id                string
	instance.Instance // stub out other methods with panics
}

func (inst *mockInstance) Id() instance.Id {
	return instance.Id(inst.id)
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
