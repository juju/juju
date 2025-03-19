// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	jujustorage "github.com/juju/juju/internal/storage"
)

type allInstancesFunc func(ctx context.Context) ([]instances.Instance, error)
type instancesFunc func(context.Context, []instance.Id) ([]instances.Instance, error)
type startInstanceFunc func(envcontext.ProviderCallContext, environs.StartInstanceParams) (instances.Instance, *instance.HardwareCharacteristics, network.InterfaceInfos, error)
type stopInstancesFunc func(envcontext.ProviderCallContext, []instance.Id) error
type getToolsSourcesFunc func() ([]simplestreams.DataSource, error)
type configFunc func() *config.Config
type setConfigFunc func(*config.Config) error

type mockEnviron struct {
	storage          storage.Storage
	allInstances     allInstancesFunc
	instances        instancesFunc
	startInstance    startInstanceFunc
	stopInstances    stopInstancesFunc
	getToolsSources  getToolsSourcesFunc
	config           configFunc
	setConfig        setConfigFunc
	storageProviders jujustorage.StaticProviderRegistry
	modelRules       firewall.IngressRules
	environs.Environ // stub out other methods with panics
}

func (env *mockEnviron) Storage() storage.Storage {
	return env.storage
}

func (env *mockEnviron) AllInstances(ctx context.Context) ([]instances.Instance, error) {
	return env.allInstances(ctx)
}

func (env *mockEnviron) AllRunningInstances(ctx context.Context) ([]instances.Instance, error) {
	return env.allInstances(ctx)
}

func (env *mockEnviron) Instances(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
	return env.instances(ctx, ids)
}

func (env *mockEnviron) StartInstance(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	inst, hw, networkInfo, err := env.startInstance(ctx, args)
	if err != nil {
		return nil, err
	}
	return &environs.StartInstanceResult{
		Instance:    inst,
		Hardware:    hw,
		NetworkInfo: networkInfo,
	}, nil
}

func (env *mockEnviron) StopInstances(ctx envcontext.ProviderCallContext, ids ...instance.Id) error {
	return env.stopInstances(ctx, ids)
}

func (env *mockEnviron) Config() *config.Config {
	return env.config()
}

func (env *mockEnviron) SetConfig(_ context.Context, cfg *config.Config) error {
	if env.setConfig != nil {
		return env.setConfig(cfg)
	}
	return nil
}

func (env *mockEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	if env.getToolsSources != nil {
		return env.getToolsSources()
	}
	datasource := storage.NewStorageSimpleStreamsDataSource("test cloud storage", env.Storage(), storage.BaseToolsPath, simplestreams.SPECIFIC_CLOUD_DATA, false)
	return []simplestreams.DataSource{datasource}, nil
}

func (env *mockEnviron) StorageProviderTypes() ([]jujustorage.ProviderType, error) {
	return env.storageProviders.StorageProviderTypes()
}

func (env *mockEnviron) StorageProvider(t jujustorage.ProviderType) (jujustorage.Provider, error) {
	return env.storageProviders.StorageProvider(t)
}

func (env *mockEnviron) OpenModelPorts(_ envcontext.ProviderCallContext, rules firewall.IngressRules) error {
	env.modelRules = append(env.modelRules, rules...)
	return nil
}

func (env *mockEnviron) CloseModelPorts(_ envcontext.ProviderCallContext, _ firewall.IngressRules) error {
	return fmt.Errorf("mock method not implemented")
}

func (env *mockEnviron) ModelIngressRules(_ envcontext.ProviderCallContext) (firewall.IngressRules, error) {
	return nil, fmt.Errorf("mock method not implemented")
}

type availabilityZonesFunc func(ctx context.Context) (network.AvailabilityZones, error)
type instanceAvailabilityZoneNamesFunc func(envcontext.ProviderCallContext, []instance.Id) (map[instance.Id]string, error)
type deriveAvailabilityZonesFunc func(envcontext.ProviderCallContext, environs.StartInstanceParams) ([]string, error)

type mockZonedEnviron struct {
	mockEnviron
	availabilityZones             availabilityZonesFunc
	instanceAvailabilityZoneNames instanceAvailabilityZoneNamesFunc
	deriveAvailabilityZones       deriveAvailabilityZonesFunc
}

func (env *mockZonedEnviron) AvailabilityZones(ctx context.Context) (network.AvailabilityZones, error) {
	return env.availabilityZones(ctx)
}

func (env *mockZonedEnviron) InstanceAvailabilityZoneNames(ctx envcontext.ProviderCallContext, ids []instance.Id) (map[instance.Id]string, error) {
	return env.instanceAvailabilityZoneNames(ctx, ids)
}

func (env *mockZonedEnviron) DeriveAvailabilityZones(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) ([]string, error) {
	return env.deriveAvailabilityZones(ctx, args)
}

type mockInstance struct {
	id                 string
	addresses          network.ProviderAddresses
	addressesErr       error
	status             instance.Status
	instances.Instance // stub out other methods with panics
}

func (inst *mockInstance) Id() instance.Id {
	return instance.Id(inst.id)
}

func (inst *mockInstance) Status(envcontext.ProviderCallContext) instance.Status {
	return inst.status
}

func (inst *mockInstance) Addresses(envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
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
