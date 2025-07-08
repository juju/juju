// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/controller"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	coretesting "github.com/juju/juju/testing"
)

type mockState struct {
	testing.Stub

	common.APIAddressAccessor
	model                        *mockModel
	applicationWatcher           *mockStringsWatcher
	app                          *mockApplication
	resource                     *mockResources
	operatorRepo                 string
	controllerConfigWatcher      *statetesting.MockNotifyWatcher
	apiHostPortsForAgentsWatcher *statetesting.MockNotifyWatcher
	isController                 bool
}

func newMockState() *mockState {
	st := &mockState{
		applicationWatcher: newMockStringsWatcher(),
	}
	st.model = &mockModel{state: st}
	return st
}

func (st *mockState) ApplyOperation(op state.ModelOperation) error {
	st.MethodCall(st, "AppyOperation")
	return nil
}

func (st *mockState) Unit(unit string) (caasapplicationprovisioner.Unit, error) {
	st.MethodCall(st, "Unit")
	return nil, nil
}

func (st *mockState) WatchControllerConfig() state.NotifyWatcher {
	st.MethodCall(st, "WatchControllerConfig")
	return st.controllerConfigWatcher
}

func (st *mockState) WatchApplications() state.StringsWatcher {
	st.MethodCall(st, "WatchApplications")
	return st.applicationWatcher
}

func (st *mockState) ControllerConfig() (controller.Config, error) {
	cfg := coretesting.FakeControllerConfig()
	cfg[controller.CAASImageRepo] = st.operatorRepo
	return cfg, nil
}

func (st *mockState) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
	st.MethodCall(st, "APIHostPortsForAgents")
	return []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1, "10.0.0.1"),
	}, nil
}

func (st *mockState) Application(appName string) (caasapplicationprovisioner.Application, error) {
	st.MethodCall(st, "Application", appName)
	if appName != "gitlab" {
		return nil, errors.NotFoundf("app %v", appName)
	}
	return st.app, nil
}

func (st *mockState) Model() (caasapplicationprovisioner.Model, error) {
	st.MethodCall(st, "Model")
	if err := st.NextErr(); err != nil {
		return nil, err
	}
	return st.model, nil
}

func (st *mockState) ModelUUID() string {
	st.MethodCall(st, "ModelUUID")
	return coretesting.ModelTag.Id()
}

func (st *mockState) ResolveConstraints(cons constraints.Value) (constraints.Value, error) {
	st.MethodCall(st, "ResolveConstraints", cons)
	if err := st.NextErr(); err != nil {
		return constraints.Value{}, err
	}
	return cons, nil
}

func (st *mockState) Resources() caasapplicationprovisioner.Resources {
	st.MethodCall(st, "Resources")
	return st.resource
}

func (st *mockState) IsController() bool {
	st.MethodCall(st, "IsController")
	return st.isController
}

func (st *mockState) WatchAPIHostPortsForAgents() state.NotifyWatcher {
	st.MethodCall(st, "WatchAPIHostPortsForAgents")
	return st.apiHostPortsForAgentsWatcher
}

type mockResources struct {
	caasapplicationprovisioner.Resources
	resource *resources.DockerImageDetails
}

func (m *mockResources) OpenResource(applicationID string, name string) (resources.Resource, io.ReadCloser, error) {
	out, err := json.Marshal(m.resource)
	return resources.Resource{}, io.NopCloser(bytes.NewBuffer(out)), err
}

type mockStorageRegistry struct {
	storage.ProviderRegistry
}

func (m *mockStorageRegistry) StorageProvider(p storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("provider %q", p)
}

type mockStoragePoolManager struct {
	testing.Stub
	poolmanager.PoolManager
}

func (m *mockStoragePoolManager) Get(name string) (*storage.Config, error) {
	m.MethodCall(m, "Get", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return storage.NewConfig(name, k8sconstants.StorageProviderType, map[string]interface{}{"foo": "bar"})
}

type mockModel struct {
	testing.Stub
	state              *mockState
	modelConfigChanges *statetesting.MockNotifyWatcher
}

func (m *mockModel) UUID() string {
	m.MethodCall(m, "UUID")
	return coretesting.ModelTag.Id()
}

func (m *mockModel) ModelConfig() (*config.Config, error) {
	m.MethodCall(m, "ModelConfig")
	attrs := coretesting.FakeConfig()
	attrs["operator-storage"] = "k8s-storage"
	attrs["agent-version"] = "2.6-beta3.666"
	return config.New(config.UseDefaults, attrs)
}

func (m *mockModel) Containers(providerIds ...string) ([]state.CloudContainer, error) {
	m.MethodCall(m, "Containers", providerIds)
	if err := m.NextErr(); err != nil {
		return nil, err
	}

	providerIdMap := map[string]struct{}{}
	for _, v := range providerIds {
		providerIdMap[v] = struct{}{}
	}

	containers := []state.CloudContainer(nil)
	for _, u := range m.state.app.units {
		if u.containerInfo == nil {
			continue
		}
		if _, ok := providerIdMap[u.containerInfo.providerId]; !ok {
			continue
		}
		containers = append(containers, u.containerInfo)
	}

	return containers, nil
}

func (m *mockModel) WatchForModelConfigChanges() state.NotifyWatcher {
	m.MethodCall(m, "WatchForModelConfigChanges")
	return m.modelConfigChanges
}

type mockApplication struct {
	testing.Stub
	state.Authenticator
	life                 state.Life
	tag                  names.Tag
	password             string
	base                 state.Base
	charm                caasapplicationprovisioner.Charm
	units                []*mockUnit
	constraints          constraints.Value
	storageConstraints   map[string]state.StorageConstraints
	deviceConstraints    map[string]state.DeviceConstraints
	charmModifiedVersion int
	config               coreconfig.ConfigAttributes
	scale                int
	unitsWatcher         *statetesting.MockStringsWatcher
	unitsChanges         chan []string
	watcher              *statetesting.MockNotifyWatcher
	charmPending         bool
	provisioningState    *state.ApplicationProvisioningState
	unitAttachmentInfos  []state.UnitAttachmentInfo
}

func (a *mockApplication) CharmPendingToBeDownloaded() bool {
	a.MethodCall(a, "CharmPendingToBeDownloaded")
	return a.charmPending
}

func (a *mockApplication) Tag() names.Tag {
	a.MethodCall(a, "Tag")
	return a.tag
}

func (a *mockApplication) SetPassword(password string) error {
	a.MethodCall(a, "SetPassword", password)
	if err := a.NextErr(); err != nil {
		return err
	}
	a.password = password
	return nil
}

func (a *mockApplication) Life() state.Life {
	a.MethodCall(a, "Life")
	return a.life
}

func (a *mockApplication) Charm() (caasapplicationprovisioner.Charm, bool, error) {
	a.MethodCall(a, "Charm")
	if err := a.NextErr(); err != nil {
		return nil, false, err
	}
	return a.charm, false, nil
}

func (a *mockApplication) AllUnits() ([]caasapplicationprovisioner.Unit, error) {
	a.MethodCall(a, "AllUnits")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	units := []caasapplicationprovisioner.Unit(nil)
	for _, u := range a.units {
		units = append(units, u)
	}
	return units, nil
}

func (a *mockApplication) Constraints() (constraints.Value, error) {
	a.MethodCall(a, "Constraints")
	if err := a.NextErr(); err != nil {
		return constraints.Value{}, err
	}
	return a.constraints, nil
}

func (a *mockApplication) UpdateUnits(unitsOp *state.UpdateUnitsOperation) error {
	a.MethodCall(a, "UpdateUnits", unitsOp)
	return a.NextErr()
}

func (a *mockApplication) StorageConstraints() (map[string]state.StorageConstraints, error) {
	a.MethodCall(a, "StorageConstraints")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return a.storageConstraints, nil
}

func (a *mockApplication) DeviceConstraints() (map[string]state.DeviceConstraints, error) {
	a.MethodCall(a, "DeviceConstraints")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return a.deviceConstraints, nil
}

func (a *mockApplication) Name() string {
	a.MethodCall(a, "Name")
	return a.tag.Id()
}

func (a *mockApplication) Base() state.Base {
	a.MethodCall(a, "Base")
	return a.base
}

func (a *mockApplication) SetOperatorStatus(statusInfo status.StatusInfo) error {
	a.MethodCall(a, "SetOperatorStatus", statusInfo)
	return a.NextErr()
}

func (a *mockApplication) SetStatus(statusInfo status.StatusInfo) error {
	a.MethodCall(a, "SetStatus", statusInfo)
	return a.NextErr()
}

func (a *mockApplication) CharmModifiedVersion() int {
	a.MethodCall(a, "CharmModifiedVersion")
	return a.charmModifiedVersion
}

func (a *mockApplication) CharmURL() (curl *string, force bool) {
	a.MethodCall(a, "CharmURL")
	cURL := a.charm.URL()
	return &cURL, false
}

func (a *mockApplication) ApplicationConfig() (coreconfig.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig")
	return a.config, a.NextErr()
}

func (a *mockApplication) GetScale() int {
	a.MethodCall(a, "GetScale")
	return a.scale
}

func (a *mockApplication) ClearResources() error {
	a.MethodCall(a, "ClearResources")
	return a.NextErr()
}

func (a *mockApplication) WatchUnits() state.StringsWatcher {
	a.MethodCall(a, "WatchUnits")
	return a.unitsWatcher
}

func (a *mockApplication) Watch() state.NotifyWatcher {
	a.MethodCall(a, "Watch")
	return a.watcher
}

func (a *mockApplication) SetProvisioningState(ps state.ApplicationProvisioningState) error {
	a.MethodCall(a, "SetProvisioningState", ps)
	err := a.NextErr()
	if err == nil {
		a.provisioningState = &ps
	}
	return err
}

func (a *mockApplication) ProvisioningState() *state.ApplicationProvisioningState {
	a.MethodCall(a, "ProvisioningState")
	return a.provisioningState
}

func (a *mockApplication) GetUnitAttachmentInfos() ([]state.UnitAttachmentInfo, error) {
	a.MethodCall(a, "GetUnitAttachmentInfos")
	return a.unitAttachmentInfos, a.NextErr()
}

type mockCharm struct {
	meta     *charm.Meta
	manifest *charm.Manifest
	url      string
}

func (ch *mockCharm) Meta() *charm.Meta {
	return ch.meta
}

func (ch *mockCharm) Manifest() *charm.Manifest {
	return ch.manifest
}

func (ch *mockCharm) URL() string {
	return ch.url
}

type mockWatcher struct {
	testing.Stub
	tomb.Tomb
}

func (w *mockWatcher) Kill() {
	w.MethodCall(w, "Kill")
	w.Tomb.Kill(nil)
}

func (w *mockWatcher) Stop() error {
	w.MethodCall(w, "Stop")
	if err := w.NextErr(); err != nil {
		return err
	}
	w.Tomb.Kill(nil)
	return w.Tomb.Wait()
}

type mockStringsWatcher struct {
	mockWatcher
	changes chan []string
}

func newMockStringsWatcher() *mockStringsWatcher {
	w := &mockStringsWatcher{changes: make(chan []string, 1)}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return nil
	})
	return w
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	w.MethodCall(w, "Changes")
	return w.changes
}

type mockUnit struct {
	testing.Stub
	destroyOp           *state.DestroyUnitOperation
	containerInfo       *mockCloudContainer
	status              status.StatusInfo
	tag                 names.Tag
	updateUnitOperation *state.UpdateUnitOperation
}

func (u *mockUnit) Tag() names.Tag {
	return u.tag
}

func (u *mockUnit) DestroyOperation() *state.DestroyUnitOperation {
	u.MethodCall(u, "DestroyOperation")
	return u.destroyOp
}

func (u *mockUnit) Status() (status.StatusInfo, error) {
	u.MethodCall(u, "Status")
	return u.status, u.NextErr()
}

func (u *mockUnit) EnsureDead() error {
	u.MethodCall(u, "EnsureDead")
	return u.NextErr()
}

func (u *mockUnit) ContainerInfo() (state.CloudContainer, error) {
	u.MethodCall(u, "ContainerInfo")
	return u.containerInfo, u.NextErr()
}

func (u *mockUnit) UpdateOperation(props state.UnitUpdateProperties) *state.UpdateUnitOperation {
	u.MethodCall(u, "UpdateOperation", props)
	return u.updateUnitOperation
}

type mockCloudContainer struct {
	testing.Stub
	unit       string
	providerId string
}

func (c *mockCloudContainer) Unit() string {
	return c.unit
}

func (c *mockCloudContainer) ProviderId() string {
	return c.providerId
}

func (c *mockCloudContainer) Address() *network.SpaceAddress {
	return nil
}

func (c *mockCloudContainer) Ports() []string {
	return nil
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

func (m *mockStorage) DestroyFilesystem(tag names.FilesystemTag, force bool) (err error) {
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

type mockResourceOpener struct {
	testing.Stub

	appName   string
	resources *mockResources
}

func (ro *mockResourceOpener) OpenResource(name string) (resources.Opened, error) {
	ro.MethodCall(ro, "OpenResource", name)
	r, rio, err := ro.resources.OpenResource(ro.appName, name)
	if err != nil {
		return resources.Opened{}, err
	}
	return resources.Opened{Resource: r, ReadCloser: rio}, nil
}
