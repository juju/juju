// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/schema"
	jtesting "github.com/juju/testing"
	"github.com/juju/utils/v2"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	statestorage "github.com/juju/juju/state/storage"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type mockEnviron struct {
	environs.NetworkingEnviron

	stub      jtesting.Stub
	spaceInfo *environs.ProviderSpaceInfo
}

func (e *mockEnviron) ProviderSpaceInfo(space *network.SpaceInfo) (*environs.ProviderSpaceInfo, error) {
	e.stub.MethodCall(e, "ProviderSpaceInfo", space)
	return e.spaceInfo, e.stub.NextErr()
}

type mockNoNetworkEnviron struct {
	environs.Environ
}

type mockCharm struct {
	jtesting.Stub

	charm.Charm
	config     *charm.Config
	meta       *charm.Meta
	manifest   *charm.Manifest
	lxdProfile *charm.LXDProfile
}

func (c *mockCharm) Meta() *charm.Meta {
	c.MethodCall(c, "Meta")
	return c.meta
}

func (c *mockCharm) Manifest() *charm.Manifest {
	c.MethodCall(c, "Manifest")
	return c.manifest
}

func (c *mockCharm) Config() *charm.Config {
	c.MethodCall(c, "Config")
	c.PopNoErr()
	return c.config
}

func (c *mockCharm) LXDProfile() *charm.LXDProfile {
	c.MethodCall(c, "LXDProfile")
	return c.lxdProfile
}

func (c *mockCharm) URL() *charm.URL {
	c.MethodCall(c, "URL")
	return &charm.URL{}
}

func (c *mockCharm) String() string {
	c.MethodCall(c, "String")
	return ""
}

type mockApplication struct {
	jtesting.Stub
	application.Application

	bindings         map[string]string
	charm            *mockCharm
	charmOrigin      *state.CharmOrigin
	curl             *charm.URL
	endpoints        []state.Endpoint
	exposedEndpoints map[string]state.ExposedEndpoint
	name             string
	scale            int
	subordinate      bool
	series           string
	units            []*mockUnit
	addedUnit        mockUnit
	config           coreapplication.ConfigAttributes
	constraints      constraints.Value
	channel          csparams.Channel
	exposed          bool
	remote           bool
	agentTools       *tools.Tools
}

func (m *mockApplication) Name() string {
	m.MethodCall(m, "Name")
	return m.name
}

func (m *mockApplication) Channel() csparams.Channel {
	m.MethodCall(m, "Channel")
	return m.channel
}

func (m *mockApplication) Charm() (application.Charm, bool, error) {
	m.MethodCall(m, "Charm")
	return m.charm, true, nil
}

func (m *mockApplication) CharmURL() (curl *charm.URL, force bool) {
	m.MethodCall(m, "CharmURL")
	return m.curl, true
}

func (m *mockApplication) CharmConfig(branchName string) (charm.Settings, error) {
	m.MethodCall(m, "CharmConfig", branchName)
	return m.charm.config.DefaultSettings(), m.NextErr()
}

func (m *mockApplication) Constraints() (constraints.Value, error) {
	m.MethodCall(m, "Constraints")
	return m.constraints, nil
}

func (m *mockApplication) Endpoints() ([]state.Endpoint, error) {
	m.MethodCall(m, "Endpoints")
	return m.endpoints, nil
}

func (m *mockApplication) ExposedEndpoints() map[string]state.ExposedEndpoint {
	m.MethodCall(m, "ExposedEndpoints")
	return m.exposedEndpoints
}

func (m *mockApplication) EndpointBindings() (application.Bindings, error) {
	m.MethodCall(m, "EndpointBindings")
	return &mockBindings{bMap: m.bindings}, m.NextErr()
}

func (a *mockApplication) AllUnits() ([]application.Unit, error) {
	a.MethodCall(a, "AllUnits")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	units := make([]application.Unit, len(a.units))
	for i := range a.units {
		units[i] = a.units[i]
	}
	return units, nil
}

func (a *mockApplication) SetCharm(cfg state.SetCharmConfig) error {
	a.MethodCall(a, "SetCharm", cfg)
	return a.NextErr()
}

func (a *mockApplication) DestroyOperation() *state.DestroyApplicationOperation {
	a.MethodCall(a, "DestroyOperation")
	return &state.DestroyApplicationOperation{}
}

func (a *mockApplication) AddUnit(args state.AddUnitParams) (application.Unit, error) {
	a.MethodCall(a, "AddUnit", args)
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	return &a.addedUnit, nil
}

func (a *mockApplication) GetScale() int {
	a.MethodCall(a, "GetScale")
	return a.scale
}

func (a *mockApplication) ChangeScale(scaleChange int) (int, error) {
	a.MethodCall(a, "ChangeScale", scaleChange)
	if err := a.NextErr(); err != nil {
		return a.scale, err
	}
	return a.scale + scaleChange, nil
}

func (a *mockApplication) SetScale(scale int, generation int64, force bool) error {
	a.MethodCall(a, "Scale", scale)
	if err := a.NextErr(); err != nil {
		return err
	}
	return nil
}

func (a *mockApplication) IsPrincipal() bool {
	a.MethodCall(a, "IsPrincipal")
	a.PopNoErr()
	return !a.subordinate
}

func (a *mockApplication) UpdateApplicationSeries(series string, force bool) error {
	a.MethodCall(a, "UpdateApplicationSeries", series, force)
	return a.NextErr()
}

func (a *mockApplication) Series() string {
	a.MethodCall(a, "Series")
	a.PopNoErr()
	return a.series
}

func (a *mockApplication) ApplicationConfig() (coreapplication.ConfigAttributes, error) {
	a.MethodCall(a, "ApplicationConfig")
	return a.config, a.NextErr()
}

func (a *mockApplication) UpdateApplicationConfig(
	changes coreapplication.ConfigAttributes,
	reset []string,
	extra environschema.Fields,
	defaults schema.Defaults,
) error {
	a.MethodCall(a, "UpdateApplicationConfig", changes, reset, extra, defaults)
	return a.NextErr()
}

func (a *mockApplication) UpdateCharmConfig(branchName string, settings charm.Settings) error {
	a.MethodCall(a, "UpdateCharmConfig", branchName, settings)
	return a.NextErr()
}

func (a *mockApplication) MergeExposeSettings(exposedEndpoints map[string]state.ExposedEndpoint) error {
	a.MethodCall(a, "MergeExposeSettings", exposedEndpoints)
	return a.NextErr()
}

func (a *mockApplication) UnsetExposeSettings(exposedEndpoints []string) error {
	a.MethodCall(a, "UnsetExposeSettings", exposedEndpoints)
	return a.NextErr()
}

func (a *mockApplication) IsExposed() bool {
	a.MethodCall(a, "IsExposed")
	return a.exposed
}

func (a *mockApplication) IsRemote() bool {
	a.MethodCall(a, "IsRemote")
	return a.remote
}

func (a *mockApplication) AgentTools() (*tools.Tools, error) {
	a.MethodCall(a, "AgentTools")
	return a.agentTools, a.NextErr()
}

func (a *mockApplication) Relations() ([]application.Relation, error) {
	a.MethodCall(a, "Relations")
	return []application.Relation{
		&mockRelation{},
	}, nil
}

func (a *mockApplication) CharmOrigin() *state.CharmOrigin {
	a.MethodCall(a, "CharmOrigin")
	return a.charmOrigin
}

type mockBindings struct {
	// A map of endpoint names to space names.
	bMap map[string]string
}

func (b *mockBindings) Map() map[string]string {
	return b.bMap
}

// TODO (stickupkid): This implementation is wrong, we should move to a newer
// gomock style setup.
func (b *mockBindings) MapWithSpaceNames(network.SpaceInfos) (map[string]string, error) {
	return b.bMap, nil
}

func (m *mockApplication) MergeBindings(bindings *state.Bindings, force bool) error {
	m.MethodCall(m, "MergeBindings", bindings, force)
	return m.NextErr()
}

type mockRemoteApplication struct {
	jtesting.Stub
	name           string
	life           state.Life
	sourceModelTag names.ModelTag
	endpoints      []state.Endpoint
	bindings       map[string]string
	spaces         []state.RemoteSpace
	offerUUID      string
	offerURL       string
	status         status.Status
	mac            *macaroon.Macaroon
}

func (m *mockRemoteApplication) Name() string {
	return m.name
}

func (m *mockRemoteApplication) Life() state.Life {
	return m.life
}

func (m *mockRemoteApplication) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: m.status}, nil
}

func (m *mockRemoteApplication) SourceModel() names.ModelTag {
	return m.sourceModelTag
}

func (m *mockRemoteApplication) Endpoints() ([]state.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockRemoteApplication) Bindings() map[string]string {
	return m.bindings
}

func (m *mockRemoteApplication) Spaces() []state.RemoteSpace {
	return m.spaces
}

func (m *mockRemoteApplication) AddEndpoints(eps []charm.Relation) error {
	for _, ep := range eps {
		m.endpoints = append(m.endpoints, state.Endpoint{
			ApplicationName: m.name,
			Relation: charm.Relation{
				Name:      ep.Name,
				Interface: ep.Interface,
				Role:      ep.Role,
			},
		})
	}
	return nil
}

func (m *mockRemoteApplication) Destroy() error {
	m.MethodCall(m, "Destroy")
	return nil
}

func (m *mockRemoteApplication) DestroyOperation(force bool) *state.DestroyRemoteApplicationOperation {
	m.MethodCall(m, "DestroyOperation")
	return &state.DestroyRemoteApplicationOperation{
		ForcedOperation: state.ForcedOperation{Force: force},
	}
}

type mockBackend struct {
	jtesting.Stub
	application.Backend

	model                      *mockModel
	charm                      *mockCharm
	applications               map[string]*mockApplication
	remoteApplications         map[string]application.RemoteApplication
	endpoints                  *[]state.Endpoint
	relations                  map[int]*mockRelation
	offerConnections           map[string]application.OfferConnection
	unitStorageAttachments     map[string][]state.StorageAttachment
	storageInstances           map[string]*mockStorage
	storageInstanceFilesystems map[string]*mockFilesystem
	controllers                map[string]crossmodel.ControllerInfo
	machines                   map[string]*mockMachine
	generation                 *mockGeneration
	spaceInfos                 network.SpaceInfos
	controllerCfg              *controller.Config
}

type mockFilesystemAccess struct {
	storagecommon.FilesystemAccess
	*mockBackend
}

func (m *mockBackend) Model() (application.Model, error) {
	return m.model, nil
}

func (m *mockBackend) VolumeAccess() storagecommon.VolumeAccess {
	return nil
}

func (m *mockBackend) FilesystemAccess() storagecommon.FilesystemAccess {
	return &mockFilesystemAccess{mockBackend: m}
}

func (m *mockBackend) ControllerTag() names.ControllerTag {
	return coretesting.ControllerTag
}

func (m *mockBackend) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	return nil, false, nil
}

func (m *mockBackend) ControllerConfig() (controller.Config, error) {
	if m.controllerCfg != nil {
		return *m.controllerCfg, nil
	}
	return controller.NewConfig(coretesting.ControllerTag.Id(), coretesting.CACert, map[string]interface{}{})
}

func (m *mockBackend) Charm(curl *charm.URL) (application.Charm, error) {
	m.MethodCall(m, "Charm", curl)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if m.charm != nil {
		return m.charm, nil
	}
	return nil, errors.NotFoundf("charm %q", curl)
}

func (m *mockBackend) Unit(name string) (application.Unit, error) {
	m.MethodCall(m, "Unit", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	var unitApp *mockApplication
	for appName, app := range m.applications {
		if strings.HasPrefix(name, appName+"/") {
			unitApp = app
			break
		}
	}
	if unitApp != nil {
		for _, u := range unitApp.units {
			if u.tag.Id() == name {
				return u, nil
			}
		}
	}
	return nil, errors.NotFoundf("unit %q", name)
}

func (m *mockBackend) UnitsInError() ([]application.Unit, error) {
	return []application.Unit{
		m.applications["postgresql"].units[0],
	}, nil
}

func (m *mockBackend) Machine(id string) (application.Machine, error) {
	m.MethodCall(m, "Machine", id)
	for machineId, machine := range m.machines {
		if id == machineId {
			return machine, nil
		}
	}
	return nil, errors.NotFoundf("machine %q", id)
}

func (m *mockBackend) AllSpaceInfos() (network.SpaceInfos, error) {
	m.MethodCall(m, "AllSpaceInfos")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.spaceInfos, nil
}

func (m *mockBackend) Space(_ string) (*state.Space, error) {
	return nil, nil
}

type mockLeadership struct{}

func (mockLeadership) Leaders() (map[string]string, error) {
	return map[string]string{
		"postgresql": "postgresql/0",
	}, nil
}

func newMockModel() mockModel {
	return mockModel{
		uuid:      utils.MustNewUUID().String(),
		modelType: state.ModelTypeIAAS,
		cfg: map[string]interface{}{
			"operator-storage": "k8s-operator-storage",
			"workload-storage": "k8s-storage",
			"agent-version":    "2.6.0",
		},
	}
}

type mockModel struct {
	application.Model
	jtesting.Stub

	uuid      string
	modelType state.ModelType
	cfg       map[string]interface{}
}

func (m *mockModel) UUID() string {
	return m.uuid
}

func (m *mockModel) ModelTag() names.ModelTag {
	return names.NewModelTag(m.UUID())
}

func (m *mockModel) Type() state.ModelType {
	return m.modelType
}

func (m *mockModel) ModelConfig() (*config.Config, error) {
	m.MethodCall(m, "ModelConfig")
	attrs := coretesting.FakeConfig().Merge(m.cfg)
	return config.New(config.UseDefaults, attrs)
}

func (m *mockModel) AgentVersion() (version.Number, error) {
	m.MethodCall(m, "AgentVersion")
	cfg, err := m.ModelConfig()
	if err != nil {
		return version.Number{}, err
	}
	ver, ok := cfg.AgentVersion()
	if !ok {
		return version.Number{}, errors.NotFoundf("agent version")
	}
	return ver, nil
}

func (m *mockModel) OpenedPortRangesForMachine(machineID string) (state.MachinePortRanges, error) {
	return mockMachinePortRanges{machineID}, nil
}

type mockMachinePortRanges struct {
	machineId string
}

func (m mockMachinePortRanges) Changes() state.ModelOperation {
	panic("not implemented")
}

func (m mockMachinePortRanges) MachineID() string {
	return m.machineId
}

func (m mockMachinePortRanges) UniquePortRanges() []network.PortRange {
	return new(mockUnitPortRanges).UniquePortRanges()
}

func (mockMachinePortRanges) ByUnit() map[string]state.UnitPortRanges {
	panic("not implemented")
}

func (mockMachinePortRanges) ForUnit(unitName string) state.UnitPortRanges {
	return mockUnitPortRanges{unitName}
}

type mockUnitPortRanges struct {
	unitName string
}

func (m mockUnitPortRanges) UnitName() string { return m.unitName }

func (mockUnitPortRanges) ForEndpoint(endpointName string) []network.PortRange {
	if endpointName == "" {
		return []network.PortRange{
			network.MustParsePortRange("100-102/tcp"),
		}
	}
	return nil
}

func (mockUnitPortRanges) ByEndpoint() network.GroupedPortRanges {
	return network.GroupedPortRanges{
		"": {
			network.MustParsePortRange("100-102/tcp"),
		},
	}
}

func (m mockUnitPortRanges) UniquePortRanges() []network.PortRange {
	return m.ForEndpoint("")
}

func (mockUnitPortRanges) Open(endpoint string, portRange network.PortRange) {
	panic("not implemented")
}

func (mockUnitPortRanges) Close(endpoint string, portRange network.PortRange) {
	panic("not implemented")
}

func (mockUnitPortRanges) Changes() state.ModelOperation {
	panic("not implemented")
}

type mockMachine struct {
	jtesting.Stub

	id string
}

func (m *mockMachine) IsLockedForSeriesUpgrade() (bool, error) {
	m.MethodCall(m, "IsLockedForSeriesUpgrade")
	return false, m.NextErr()
}

func (m *mockMachine) IsParentLockedForSeriesUpgrade() (bool, error) {
	m.MethodCall(m, "IsParentLockedForSeriesUpgrade")
	return false, m.NextErr()
}

func (m *mockMachine) Id() string {
	m.MethodCall(m, "Id")
	return m.id
}

func (m *mockMachine) PublicAddress() (network.SpaceAddress, error) {
	return network.SpaceAddress{
		MachineAddress: network.MachineAddress{Value: "10.0.0.1"},
	}, nil
}

func (m *mockBackend) InferEndpoints(endpoints ...string) ([]state.Endpoint, error) {
	m.MethodCall(m, "InferEndpoints", endpoints)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if m.endpoints != nil {
		return *m.endpoints, nil
	}
	return nil, errors.Errorf("no relations found")
}

func (m *mockBackend) EndpointsRelation(endpoints ...state.Endpoint) (application.Relation, error) {
	m.MethodCall(m, "EndpointsRelation", endpoints)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if rel, ok := m.relations[123]; ok {
		return rel, nil
	}
	return nil, errors.NotFoundf("relation")
}

func (m *mockBackend) Relation(id int) (application.Relation, error) {
	m.MethodCall(m, "Relation", id)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if rel, ok := m.relations[id]; ok {
		return rel, nil
	}
	return nil, errors.NotFoundf("relation")
}

type mockOfferConnection struct {
	application.OfferConnection
}

func (m *mockBackend) OfferConnectionForRelation(key string) (application.OfferConnection, error) {
	m.MethodCall(m, "OfferConnectionForRelation", key)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	if oc, ok := m.offerConnections[key]; ok {
		return oc, nil
	}
	return nil, errors.NotFoundf("offer connection for relation")
}

func (m *mockBackend) UnitStorageAttachments(tag names.UnitTag) ([]state.StorageAttachment, error) {
	m.MethodCall(m, "UnitStorageAttachments", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.unitStorageAttachments[tag.Id()], nil
}

func (m *mockBackend) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	m.MethodCall(m, "StorageInstance", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	s, ok := m.storageInstances[tag.Id()]
	if !ok {
		return nil, errors.NotFoundf("storage %s", tag.Id())
	}
	return s, nil
}

func (m *mockFilesystemAccess) StorageInstanceFilesystem(tag names.StorageTag) (state.Filesystem, error) {
	m.MethodCall(m, "StorageInstanceFilesystem", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	f, ok := m.storageInstanceFilesystems[tag.Id()]
	if !ok {
		return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
	}
	return f, nil
}

func (m *mockBackend) AddRemoteApplication(args state.AddRemoteApplicationParams) (application.RemoteApplication, error) {
	m.MethodCall(m, "AddRemoteApplication", args)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	app := &mockRemoteApplication{
		name:           args.Name,
		sourceModelTag: args.SourceModel,
		offerUUID:      args.OfferUUID,
		offerURL:       args.URL,
		bindings:       args.Bindings,
		mac:            args.Macaroon,
		status:         status.Active,
	}
	for _, ep := range args.Endpoints {
		app.endpoints = append(app.endpoints, state.Endpoint{
			ApplicationName: app.name,
			Relation: charm.Relation{
				Name:      ep.Name,
				Interface: ep.Interface,
				Role:      ep.Role,
			},
		})
	}
	for _, sp := range args.Spaces {
		remoteSpaceInfo := state.RemoteSpace{
			CloudType:          sp.CloudType,
			Name:               string(sp.Name),
			ProviderId:         string(sp.ProviderId),
			ProviderAttributes: sp.ProviderAttributes,
		}
		for _, sn := range sp.Subnets {
			remoteSpaceInfo.Subnets = append(remoteSpaceInfo.Subnets, state.RemoteSubnet{
				CIDR:              sn.CIDR,
				VLANTag:           sn.VLANTag,
				ProviderId:        string(sn.ProviderId),
				ProviderNetworkId: string(sn.ProviderNetworkId),
				AvailabilityZones: sn.AvailabilityZones,
			})
		}
		app.spaces = append(app.spaces, remoteSpaceInfo)
	}
	m.remoteApplications[app.name] = app
	return app, nil
}

func (m *mockBackend) RemoteApplication(name string) (application.RemoteApplication, error) {
	m.MethodCall(m, "RemoteApplication", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	app, ok := m.remoteApplications[name]
	if !ok {
		return nil, errors.NotFoundf("saas application %q", name)
	}
	return app, nil
}

func (m *mockBackend) Application(name string) (application.Application, error) {
	m.MethodCall(m, "Application", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	app, ok := m.applications[name]
	if !ok {
		return nil, errors.NotFoundf("application %q", name)
	}
	return app, nil
}

func (m *mockBackend) ApplyOperation(op state.ModelOperation) error {
	m.MethodCall(m, "ApplyOperation", op)
	return m.NextErr()
}

func (m *mockBackend) SaveController(controllerInfo crossmodel.ControllerInfo, modelUUID string) (application.ExternalController, error) {
	m.controllers[modelUUID] = controllerInfo
	return &mockExternalController{controllerInfo.ControllerTag.Id(), controllerInfo}, nil
}

func (m *mockBackend) Branch(branchName string) (application.Generation, error) {
	if branchName != "new-branch" {
		return nil, errors.NotFoundf("branch %q", branchName)
	}
	if m.generation == nil {
		m.generation = &mockGeneration{}
	}
	return m.generation, nil
}

type mockExternalController struct {
	uuid string
	info crossmodel.ControllerInfo
}

func (m *mockExternalController) Id() string {
	return m.uuid
}

func (m *mockExternalController) ControllerInfo() crossmodel.ControllerInfo {
	return m.info
}

type mockBlockChecker struct {
	jtesting.Stub
}

func (c *mockBlockChecker) ChangeAllowed() error {
	c.MethodCall(c, "ChangeAllowed")
	return c.NextErr()
}

func (c *mockBlockChecker) RemoveAllowed() error {
	c.MethodCall(c, "RemoveAllowed")
	return c.NextErr()
}

type mockRelation struct {
	application.Relation
	jtesting.Stub

	tag             names.Tag
	status          status.Status
	message         string
	suspended       bool
	suspendedReason string
}

func (r *mockRelation) Tag() names.Tag {
	return r.tag
}

func (r *mockRelation) Endpoints() []state.Endpoint {
	r.MethodCall(r, "Endpoints")
	return []state.Endpoint{{
		ApplicationName: "postgresql",
	}, {
		ApplicationName: "gitlab",
	}}
}

func (r *mockRelation) RelatedEndpoints(appName string) ([]state.Endpoint, error) {
	r.MethodCall(r, "RelatedEndpoints")
	if appName != "postgresql" {
		return nil, errors.NotFoundf("app %q", appName)
	}
	return []state.Endpoint{{
		ApplicationName: "gitlab",
		Relation:        charm.Relation{Name: "server"},
	}}, nil
}

func (r *mockRelation) ApplicationSettings(appName string) (map[string]interface{}, error) {
	r.MethodCall(r, "ApplicationSettings")
	return map[string]interface{}{"app-" + appName: "setting"}, nil
}

func (r *mockRelation) AllRemoteUnits(appName string) ([]application.RelationUnit, error) {
	r.MethodCall(r, "AllRemoteUnits")
	return []application.RelationUnit{
		mockRelationUnit{appName + "/2"},
	}, nil
}

func (r *mockRelation) Unit(name string) (application.RelationUnit, error) {
	r.MethodCall(r, "Unit")
	return mockRelationUnit{name}, nil
}

func (r *mockRelation) Endpoint(name string) (state.Endpoint, error) {
	r.MethodCall(r, "Endpoint")
	if name != "postgresql" {
		return state.Endpoint{}, errors.NotFoundf("endpoint for %q", name)
	}
	return state.Endpoint{
		ApplicationName: "postgresql",
		Relation:        charm.Relation{Name: "db"},
	}, nil
}

func (r *mockRelation) SetStatus(status status.StatusInfo) error {
	r.MethodCall(r, "SetStatus")
	r.status = status.Status
	r.message = status.Message
	return r.NextErr()
}

func (r *mockRelation) SetSuspended(suspended bool, reason string) error {
	r.MethodCall(r, "SetSuspended")
	r.suspended = suspended
	r.suspendedReason = reason
	return r.NextErr()
}

func (r *mockRelation) Suspended() bool {
	r.MethodCall(r, "Suspended")
	return r.suspended
}

func (r *mockRelation) SuspendedReason() string {
	r.MethodCall(r, "SuspendedReason")
	return r.suspendedReason
}

func (r *mockRelation) Destroy() error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

func (r *mockRelation) DestroyWithForce(force bool, maxWait time.Duration) ([]error, error) {
	r.MethodCall(r, "DestroyWithForce", force, maxWait)
	return nil, r.NextErr()
}

type mockRelationUnit struct {
	unit string
}

func (m mockRelationUnit) UnitName() string {
	return m.unit
}

func (mockRelationUnit) InScope() (bool, error) {
	return true, nil
}

func (m mockRelationUnit) Settings() (map[string]interface{}, error) {
	return map[string]interface{}{m.unit: m.unit + "-setting"}, nil
}

type mockUnit struct {
	application.Unit
	jtesting.Stub
	tag        names.UnitTag
	machineId  string
	name       string
	agentTools *tools.Tools
}

func (u *mockUnit) Tag() names.Tag {
	return u.tag
}

func (u *mockUnit) UnitTag() names.UnitTag {
	return u.tag
}

func (u *mockUnit) IsPrincipal() bool {
	u.MethodCall(u, "IsPrincipal")
	u.PopNoErr()
	return true
}

func (u *mockUnit) DestroyOperation() *state.DestroyUnitOperation {
	u.MethodCall(u, "DestroyOperation")
	return &state.DestroyUnitOperation{}
}

func (u *mockUnit) AssignWithPolicy(policy state.AssignmentPolicy) error {
	u.MethodCall(u, "AssignWithPolicy", policy)
	return u.NextErr()
}

func (u *mockUnit) AssignWithPlacement(placement *instance.Placement) error {
	u.MethodCall(u, "AssignWithPlacement", placement)
	return u.NextErr()
}

func (u *mockUnit) Resolve(retryHooks bool) error {
	u.MethodCall(u, "Resolve", retryHooks)
	return u.NextErr()
}

func (u *mockUnit) AssignedMachineId() (string, error) {
	u.MethodCall(u, "AssignedMachineId")
	return u.machineId, u.NextErr()
}

func (u *mockUnit) Name() string {
	u.MethodCall(u, "Name")
	return u.name
}

func (u *mockUnit) ApplicationName() string {
	u.MethodCall(u, "ApplicationName")
	appName, _ := names.UnitApplication(u.name)
	return appName
}

func (u *mockUnit) WorkloadVersion() (string, error) {
	u.MethodCall(u, "WorkloadVersion")
	return "666", nil
}

func (u *mockUnit) ContainerInfo() (state.CloudContainer, error) {
	return mockCloudContainer{}, nil
}

func (u *mockUnit) AgentTools() (*tools.Tools, error) {
	u.MethodCall(u, "AgentTools")
	return u.agentTools, u.NextErr()
}

type mockCloudContainer struct {
	state.CloudContainer
}

func (mockCloudContainer) ProviderId() string {
	return "provider-id"
}

func (mockCloudContainer) Address() *network.SpaceAddress {
	return &network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "192.168.1.1",
		},
	}
}

type mockStorageAttachment struct {
	state.StorageAttachment
	jtesting.Stub
	unit    names.UnitTag
	storage names.StorageTag
}

func (a *mockStorageAttachment) Unit() names.UnitTag {
	return a.unit
}

func (a *mockStorageAttachment) StorageInstance() names.StorageTag {
	return a.storage
}

type mockStorage struct {
	state.StorageInstance
	jtesting.Stub
	tag   names.StorageTag
	owner names.Tag
}

func (a *mockStorage) Kind() state.StorageKind {
	return state.StorageKindFilesystem
}

func (a *mockStorage) StorageTag() names.StorageTag {
	return a.tag
}

func (a *mockStorage) Owner() (names.Tag, bool) {
	return a.owner, a.owner != nil
}

type mockFilesystem struct {
	state.Filesystem
	detachable bool
}

func (f *mockFilesystem) Detachable() bool {
	return f.detachable
}

type blobs struct {
	sync.Mutex
	m map[string]bool // maps path to added (true), or deleted (false)
}

// Add adds a path to the list of known paths.
func (b *blobs) Add(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = true
}

// Remove marks a path as deleted, even if it was not previously Added.
func (b *blobs) Remove(path string) {
	b.Lock()
	defer b.Unlock()
	b.check()
	b.m[path] = false
}

func (b *blobs) check() {
	if b.m == nil {
		b.m = make(map[string]bool)
	}
}

type recordingStorage struct {
	statestorage.Storage
	putBarrier *sync.WaitGroup
	blobs      *blobs
}

func (s *recordingStorage) Put(path string, r io.Reader, size int64) error {
	if s.putBarrier != nil {
		// This goroutine has gotten to Put() so mark it Done() and
		// wait for the other goroutines to get to this point.
		s.putBarrier.Done()
		s.putBarrier.Wait()
	}
	if err := s.Storage.Put(path, r, size); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Add(path)
	return nil
}

func (s *recordingStorage) Remove(path string) error {
	if err := s.Storage.Remove(path); err != nil {
		return errors.Trace(err)
	}
	s.blobs.Remove(path)
	return nil
}

type mockStoragePoolManager struct {
	jtesting.Stub
	poolmanager.PoolManager
	storageType storage.ProviderType
}

func (m *mockStoragePoolManager) Get(name string) (*storage.Config, error) {
	m.MethodCall(m, "Get", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return storage.NewConfig(name, m.storageType, map[string]interface{}{"foo": "bar"})
}

type mockStorageRegistry struct {
	jtesting.Stub
	storage.ProviderRegistry
}

type mockProvider struct {
	storage.Provider
}

func (m *mockProvider) Supports(kind storage.StorageKind) bool {
	return kind == storage.StorageKindFilesystem
}

func (m *mockStorageRegistry) StorageProvider(p storage.ProviderType) (storage.Provider, error) {
	if p == provider.RootfsProviderType {
		return &mockProvider{}, nil
	}
	return nil, errors.NotFoundf("provider type %q", p)
}

type mockCaasBroker struct {
	jtesting.Stub
	caas.StorageValidator
	caas.ClusterVersionGetter
}

func (m *mockCaasBroker) ValidateStorageClass(config map[string]interface{}) error {
	m.MethodCall(m, "ValidateStorageClass", config)
	return m.NextErr()
}

func (m *mockCaasBroker) Version() (*version.Number, error) {
	m.MethodCall(m, "Version")
	ver := version.MustParse("1.15.0")
	return &ver, nil
}

type mockGeneration struct {
	jtesting.Stub
}

func (g *mockGeneration) AssignApplication(appName string) error {
	g.MethodCall(g, "AssignApplication", appName)
	return g.NextErr()
}

type mockRepo struct {
	application.Repository
	*jtesting.CallMocker
	revisions map[string]int
}

func (m *mockRepo) DownloadCharm(resourceURL, _ string) (*charm.CharmArchive, error) {
	results := m.MethodCall(m, "DownloadCharm", resourceURL)
	if results == nil {
		return nil, errors.NotFoundf(`cannot retrieve %q: charm`, resourceURL)
	}
	return results[0].(*charm.CharmArchive), jtesting.TypeAssertError(results[1])
}

type mockUpdateSeries struct {
	jtesting.Stub
}

func (m *mockUpdateSeries) UpdateSeries(tag string, series string, force bool) error {
	m.MethodCall(m, "UpdateSeries", tag, series, force)
	return nil
}
