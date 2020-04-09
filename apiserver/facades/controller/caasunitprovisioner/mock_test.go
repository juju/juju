// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type mockState struct {
	testing.Stub
	application         mockApplication
	applicationsWatcher *statetesting.MockStringsWatcher
	model               mockModel
	unit                mockUnit
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationsWatcher
}

func (st *mockState) Application(name string) (caasunitprovisioner.Application, error) {
	st.MethodCall(st, "Application", name)
	if name != st.application.tag.Id() {
		return nil, errors.NotFoundf("application %v", name)
	}
	return &st.application, st.NextErr()
}

func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	st.MethodCall(st, "FindEntity", tag)
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	switch tag.(type) {
	case names.ApplicationTag:
		return &st.application, nil
	case names.UnitTag:
		return &st.unit, nil
	default:
		return nil, errors.NotFoundf("%s", names.ReadableString(tag))
	}
}

func (st *mockState) ControllerConfig() (controller.Config, error) {
	st.MethodCall(st, "ControllerConfig")
	return coretesting.FakeControllerConfig(), nil
}

func (st *mockState) Model() (caasunitprovisioner.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return &st.model, nil
}

func (st *mockState) ResolveConstraints(cons constraints.Value) (constraints.Value, error) {
	st.MethodCall(st, "ResolveConstraints", cons)
	if err := st.NextErr(); err != nil {
		return constraints.Value{}, err
	}
	return cons, nil
}

func (m *mockState) AllSpaceInfos() (network.SpaceInfos, error) {
	m.MethodCall(m, "AllSpaceInfos")
	return network.SpaceInfos{}, nil
}

type mockModel struct {
	testing.Stub
	podSpecWatcher *statetesting.MockNotifyWatcher
	containers     []state.CloudContainer
	isRawK8sSpec   *bool
}

func (m *mockModel) ModelConfig() (*config.Config, error) {
	m.MethodCall(m, "ModelConfig")
	attrs := coretesting.FakeConfig()
	attrs["workload-storage"] = "k8s-storage"
	attrs["agent-version"] = jujuversion.Current.String()
	return config.New(config.UseDefaults, attrs)
}

func (m *mockModel) PodSpec(tag names.ApplicationTag) (string, error) {
	m.MethodCall(m, "PodSpec", tag)
	if err := m.NextErr(); err != nil {
		return "", err
	}
	if *m.isRawK8sSpec {
		return "", nil
	}
	return "spec(" + tag.Id() + ")", nil
}

func (m *mockModel) RawK8sSpec(tag names.ApplicationTag) (string, error) {
	m.MethodCall(m, "RawK8sSpec", tag)
	if err := m.NextErr(); err != nil {
		return "", err
	}
	if *m.isRawK8sSpec {
		return "raw spec(" + tag.Id() + ")", nil
	}
	return "", nil
}

func (m *mockModel) WatchPodSpec(tag names.ApplicationTag) (state.NotifyWatcher, error) {
	m.MethodCall(m, "WatchPodSpec", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.podSpecWatcher, nil
}

func (m *mockModel) Containers(providerIds ...string) ([]state.CloudContainer, error) {
	m.MethodCall(m, "Containers", providerIds)
	return m.containers, nil
}

type mockApplication struct {
	testing.Stub
	life         state.Life
	scaleWatcher *statetesting.MockNotifyWatcher

	tag        names.Tag
	scale      int
	units      []caasunitprovisioner.Unit
	ops        *state.UpdateUnitsOperation
	providerId string
	addresses  []network.SpaceAddress
	charm      *mockCharm
}

func (a *mockApplication) Tag() names.Tag {
	return a.tag
}

func (a *mockApplication) Name() string {
	a.MethodCall(a, "Name")
	return a.tag.Id()
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) WatchScale() state.NotifyWatcher {
	a.MethodCall(a, "WatchScale")
	return a.scaleWatcher
}

func (a *mockApplication) GetScale() int {
	a.MethodCall(a, "GetScale")
	return a.scale
}

func (a *mockApplication) SetScale(scale int, generation int64, force bool) error {
	a.MethodCall(a, "SetScale", scale)
	a.scale = scale
	return nil
}

func (a *mockApplication) StorageConstraints() (map[string]state.StorageConstraints, error) {
	return map[string]state.StorageConstraints{
		"data": {
			Size:  100,
			Count: 1,
		},
		"logs": {
			Size:  200,
			Pool:  "rootfs",
			Count: 1,
		},
	}, nil
}

type mockCharm struct {
	meta charm.Meta
}

func (m *mockCharm) Meta() *charm.Meta {
	return &m.meta
}

func (a *mockApplication) Charm() (caasunitprovisioner.Charm, bool, error) {
	a.MethodCall(a, "Charm")
	return a.charm, false, nil
}

func (a *mockApplication) GetPlacement() string {
	a.MethodCall(a, "GetPlacement")
	return "placement"
}

func (a *mockApplication) ApplicationConfig() (application.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig")
	return application.ConfigAttributes{"foo": "bar"}, a.NextErr()
}

func (m *mockApplication) AllUnits() (units []caasunitprovisioner.Unit, err error) {
	return m.units, nil
}

func (m *mockApplication) UpdateUnits(ops *state.UpdateUnitsOperation) error {
	m.ops = ops
	return nil
}

func (m *mockApplication) DeviceConstraints() (map[string]state.DeviceConstraints, error) {
	return map[string]state.DeviceConstraints{
		"bitcoinminer": {Type: "nvidia.com/gpu",
			Count:      3,
			Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
		},
	}, nil
}

func (m *mockApplication) Constraints() (constraints.Value, error) {
	return constraints.MustParse("mem=64G"), nil
}

func (m *mockApplication) UpdateCloudService(providerId string, addresses []network.SpaceAddress) error {
	m.providerId = providerId
	m.addresses = addresses
	return nil
}

var addOp = &state.AddUnitOperation{}

func (m *mockApplication) AddOperation(props state.UnitUpdateProperties) *state.AddUnitOperation {
	m.MethodCall(m, "AddOperation", props)
	return addOp
}

func (m *mockApplication) SetOperatorStatus(sInfo status.StatusInfo) error {
	m.MethodCall(m, "SetOperatorStatus", sInfo)
	return nil
}

func (m *mockApplication) SetStatus(sInfo status.StatusInfo) error {
	m.MethodCall(m, "SetStatus", sInfo)
	return nil
}

type mockContainerInfo struct {
	state.CloudContainer
	providerId string
	unitName   string
}

func (m *mockContainerInfo) ProviderId() string {
	return m.providerId
}

func (m *mockContainerInfo) Unit() string {
	return m.unitName
}

type mockUnit struct {
	testing.Stub
	name          string
	life          state.Life
	containerInfo *mockContainerInfo
}

func (*mockUnit) Tag() names.Tag {
	panic("should not be called")
}

func (u *mockUnit) UnitTag() names.UnitTag {
	return names.NewUnitTag(u.name)
}

func (u *mockUnit) Life() state.Life {
	u.MethodCall(u, "Life")
	return u.life
}

func (m *mockUnit) Name() string {
	return m.name
}

func (m *mockUnit) ContainerInfo() (state.CloudContainer, error) {
	if m.containerInfo == nil {
		return nil, errors.NotFoundf("container info")
	}
	return m.containerInfo, nil
}

func (m *mockUnit) AgentStatus() (status.StatusInfo, error) {
	return status.StatusInfo{Status: status.Allocating}, nil
}

var updateOp = &state.UpdateUnitOperation{}

func (m *mockUnit) UpdateOperation(props state.UnitUpdateProperties) *state.UpdateUnitOperation {
	m.MethodCall(m, "UpdateOperation", props)
	return updateOp
}

var destroyOp = &state.DestroyUnitOperation{}

func (m *mockUnit) DestroyOperation() *state.DestroyUnitOperation {
	m.MethodCall(m, "DestroyOperation")
	return destroyOp
}

type mockStorage struct {
	testing.Stub
	storageFilesystems map[names.StorageTag]names.FilesystemTag
	storageVolumes     map[names.StorageTag]names.VolumeTag
	storageAttachments map[names.UnitTag]names.StorageTag
	backingVolume      names.VolumeTag
}

func (m *mockStorage) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	m.MethodCall(m, "StorageInstance", tag)
	return &mockStorageInstance{
		tag:   tag,
		owner: names.NewUserTag("fred"),
	}, nil
}

func (m *mockStorage) AllFilesystems() ([]state.Filesystem, error) {
	m.MethodCall(m, "AllFilesystems")
	var result []state.Filesystem
	for _, fsTag := range m.storageFilesystems {
		result = append(result, &mockFilesystem{Stub: &m.Stub, tag: fsTag, volTag: m.backingVolume})
	}
	return result, nil
}

func (m *mockStorage) DestroyStorageInstance(tag names.StorageTag, destroyAttachments bool, force bool, maxWait time.Duration) (err error) {
	m.MethodCall(m, "DestroyStorageInstance", tag, destroyAttachments, force)
	return nil
}

func (m *mockStorage) DestroyFilesystem(tag names.FilesystemTag) (err error) {
	m.MethodCall(m, "DestroyFilesystem", tag)
	return nil
}

func (m *mockStorage) DestroyVolume(tag names.VolumeTag) (err error) {
	m.MethodCall(m, "DestroyVolume", tag)
	return nil
}

func (m *mockStorage) Filesystem(fsTag names.FilesystemTag) (state.Filesystem, error) {
	m.MethodCall(m, "Filesystem", fsTag)
	return &mockFilesystem{Stub: &m.Stub, tag: fsTag, volTag: m.backingVolume}, nil
}

func (m *mockStorage) StorageInstanceFilesystem(tag names.StorageTag) (state.Filesystem, error) {
	return &mockFilesystem{Stub: &m.Stub, tag: m.storageFilesystems[tag], volTag: m.backingVolume}, nil
}

func (m *mockStorage) UnitStorageAttachments(unit names.UnitTag) ([]state.StorageAttachment, error) {
	m.MethodCall(m, "UnitStorageAttachments", unit)
	return []state.StorageAttachment{
		&mockStorageAttachment{
			unit:    unit,
			storage: m.storageAttachments[unit],
		},
	}, nil
}

func (m *mockStorage) SetFilesystemInfo(fsTag names.FilesystemTag, fsInfo state.FilesystemInfo) error {
	m.MethodCall(m, "SetFilesystemInfo", fsTag, fsInfo)
	return nil
}

func (m *mockStorage) SetFilesystemAttachmentInfo(host names.Tag, fsTag names.FilesystemTag, info state.FilesystemAttachmentInfo) error {
	m.MethodCall(m, "SetFilesystemAttachmentInfo", host, fsTag, info)
	return nil
}

type mockDeviceBackend struct {
	testing.Stub
	devices            map[names.StorageTag]names.FilesystemTag
	storageAttachments map[names.UnitTag]names.StorageTag
}

func (d *mockDeviceBackend) DeviceConstraints(id string) (map[string]state.DeviceConstraints, error) {
	d.MethodCall(d, "DeviceConstraints", id)
	return map[string]state.DeviceConstraints{
		"bitcoinminer": {Type: "nvidia.com/gpu",
			Count:      3,
			Attributes: map[string]string{"gpu": "nvidia-tesla-p100"},
		}}, nil
}
func (m *mockStorage) Volume(volTag names.VolumeTag) (state.Volume, error) {
	m.MethodCall(m, "Volume", volTag)
	return &mockVolume{Stub: &m.Stub, tag: volTag}, nil
}

func (m *mockStorage) StorageInstanceVolume(tag names.StorageTag) (state.Volume, error) {
	return &mockVolume{Stub: &m.Stub, tag: m.storageVolumes[tag]}, nil
}

func (m *mockStorage) SetVolumeInfo(volTag names.VolumeTag, volInfo state.VolumeInfo) error {
	m.MethodCall(m, "SetVolumeInfo", volTag, volInfo)
	return nil
}

func (m *mockStorage) SetVolumeAttachmentInfo(host names.Tag, volTag names.VolumeTag, info state.VolumeAttachmentInfo) error {
	m.MethodCall(m, "SetVolumeAttachmentInfo", host, volTag, info)
	return nil
}

type mockStorageInstance struct {
	state.StorageInstance
	tag   names.StorageTag
	owner names.Tag
}

func (a *mockStorageInstance) Kind() state.StorageKind {
	return state.StorageKindFilesystem
}

func (a *mockStorageInstance) Tag() names.Tag {
	return a.tag
}

func (a *mockStorageInstance) StorageName() string {
	id := a.tag.Id()
	return strings.Split(id, "/")[0]
}

type mockStorageAttachment struct {
	state.StorageAttachment
	testing.Stub
	unit    names.UnitTag
	storage names.StorageTag
}

func (a *mockStorageAttachment) StorageInstance() names.StorageTag {
	return a.storage
}

type mockFilesystem struct {
	*testing.Stub
	state.Filesystem
	tag    names.FilesystemTag
	volTag names.VolumeTag
}

func (f *mockFilesystem) Tag() names.Tag {
	return f.FilesystemTag()
}

func (f *mockFilesystem) FilesystemTag() names.FilesystemTag {
	return f.tag
}

func (f *mockFilesystem) Volume() (names.VolumeTag, error) {
	if f.volTag.Id() == "" {
		return f.volTag, state.ErrNoBackingVolume
	}
	return f.volTag, nil
}

func (f *mockFilesystem) SetStatus(statusInfo status.StatusInfo) error {
	f.MethodCall(f, "SetStatus", statusInfo)
	return nil
}

func (f *mockFilesystem) Info() (state.FilesystemInfo, error) {
	return state.FilesystemInfo{}, errors.NotProvisionedf("filesystem")
}

type mockVolume struct {
	*testing.Stub
	state.Volume
	tag names.VolumeTag
}

func (v *mockVolume) Tag() names.Tag {
	return v.VolumeTag()
}

func (v *mockVolume) VolumeTag() names.VolumeTag {
	return v.tag
}

func (v *mockVolume) SetStatus(statusInfo status.StatusInfo) error {
	v.MethodCall(v, "SetStatus", statusInfo)
	return nil
}

func (v *mockVolume) Info() (state.VolumeInfo, error) {
	return state.VolumeInfo{}, errors.NotProvisionedf("volume")
}

type mockStoragePoolManager struct {
	testing.Stub
	poolmanager.PoolManager
}

func (m *mockStoragePoolManager) Get(name string) (*storage.Config, error) {
	m.MethodCall(m, "Get", name)
	if name == "rootfs" {
		return nil, errors.NotFoundf("pool %q", name)
	}
	return storage.NewConfig(name, provider.K8s_ProviderType, map[string]interface{}{"foo": "bar"})
}

type mockStorageRegistry struct {
	testing.Stub
	storage.ProviderRegistry
}

type mockProvider struct {
	storage.Provider
}

func (m *mockProvider) Supports(kind storage.StorageKind) bool {
	return kind == storage.StorageKindFilesystem
}

func (m *mockStorageRegistry) StorageProvider(p storage.ProviderType) (storage.Provider, error) {
	if p != "rootfs" {
		return nil, errors.NotFoundf("provider %q", p)
	}
	return &mockProvider{}, nil
}
