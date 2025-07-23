// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/schema"
	"github.com/juju/version/v2"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

// Backend defines the state functionality required by the application
// facade. For details on the methods, see the methods on state.State
// with the same names.
type Backend interface {
	AllModelUUIDs() ([]string, error)
	Application(string) (Application, error)
	ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error)
	ApplyOperation(state.ModelOperation) error
	AddApplication(state.AddApplicationArgs) (Application, error)
	AddPendingResource(string, resource.Resource) (string, error)
	RemovePendingResources(applicationID string, pendingIDs map[string]string) error
	AddCharmMetadata(info state.CharmInfo) (Charm, error)
	RemoteApplication(string) (RemoteApplication, error)
	AddRemoteApplication(state.AddRemoteApplicationParams) (RemoteApplication, error)
	AddRelation(...state.Endpoint) (Relation, error)
	Charm(string) (Charm, error)
	Relation(int) (Relation, error)
	InferEndpoints(...string) ([]state.Endpoint, error)
	InferActiveRelation(...string) (Relation, error)
	Machine(string) (Machine, error)
	Model() (Model, error)
	Unit(string) (Unit, error)
	UnitsInError() ([]Unit, error)
	SaveController(info crossmodel.ControllerInfo, modelUUID string) (ExternalController, error)
	ControllerTag() names.ControllerTag
	ControllerConfig() (controller.Config, error)
	Resources() Resources
	OfferConnectionForRelation(string) (OfferConnection, error)
	SaveEgressNetworks(relationKey string, cidrs []string) (state.RelationNetworks, error)
	Branch(string) (Generation, error)
	state.EndpointBinding
	ModelConstraints() (constraints.Value, error)
	services.StateBackend
}

// SecretsState defines the functionality for accessing secrets.
type SecretsState interface {
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
	ListSecrets(filter state.SecretsFilter) ([]*secrets.SecretMetadata, error)
	ListSecretRevisions(uri *secrets.URI) ([]*secrets.SecretRevisionMetadata, error)
}

// BlockChecker defines the block-checking functionality required by
// the application facade. This is implemented by
// apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed() error
	RemoveAllowed() error
}

// Application defines a subset of the functionality provided by the
// state.Application type, as required by the application facade. For
// details on the methods, see the methods on state.Application with
// the same names.
type Application interface {
	Name() string
	AddUnit(state.AddUnitParams) (Unit, error)
	AllUnits() ([]Unit, error)
	ApplicationConfig() (coreconfig.ConfigAttributes, error)
	ApplicationTag() names.ApplicationTag
	Charm() (Charm, bool, error)
	CharmURL() (*string, bool)
	CharmOrigin() *state.CharmOrigin
	ClearExposed() error
	CharmConfig(string) (charm.Settings, error)
	Constraints() (constraints.Value, error)
	Destroy() error
	DestroyOperation() *state.DestroyApplicationOperation
	EndpointBindings() (Bindings, error)
	ExposedEndpoints() map[string]state.ExposedEndpoint
	Endpoints() ([]state.Endpoint, error)
	IsExposed() bool
	IsPrincipal() bool
	IsRemote() bool
	Life() state.Life
	SetCharm(state.SetCharmConfig) error
	SetConstraints(constraints.Value) error
	MergeExposeSettings(map[string]state.ExposedEndpoint) error
	UnsetExposeSettings([]string) error
	SetMetricCredentials([]byte) error
	SetMinUnits(int) error
	UpdateApplicationBase(state.Base, bool) error
	UpdateCharmConfig(string, charm.Settings) error
	UpdateApplicationConfig(coreconfig.ConfigAttributes, []string, environschema.Fields, schema.Defaults) error
	SetScale(int, int64, bool) error
	ChangeScale(int, []names.StorageTag) (int, error)
	AgentTools() (*tools.Tools, error)
	MergeBindings(*state.Bindings, bool) error
	Relations() ([]Relation, error)
}

// Bindings defines a subset of the functionality provided by the
// state.Bindings type, as required by the application facade. For
// details on the methods, see the methods on state.Bindings with
// the same names.
type Bindings interface {
	Map() map[string]string
	MapWithSpaceNames(network.SpaceInfos) (map[string]string, error)
}

// Charm defines a subset of the functionality provided by the
// state.Charm type, as required by the application facade. For
// details on the methods, see the methods on state.Charm with
// the same names.
type Charm interface {
	CharmMeta
	Config() *charm.Config
	Metrics() *charm.Metrics
	Actions() *charm.Actions
	Revision() int
	IsUploaded() bool
}

// CharmMeta describes methods that inform charm operation.
type CharmMeta interface {
	Manifest() *charm.Manifest
	Meta() *charm.Meta
}

// Machine defines a subset of the functionality provided by the
// state.Machine type, as required by the application facade. For
// details on the methods, see the methods on state.Machine with
// the same names.
type Machine interface {
	Base() state.Base
	HardwareCharacteristics() (*instance.HardwareCharacteristics, error)
	Id() string
	PublicAddress() (network.SpaceAddress, error)
	IsLockedForSeriesUpgrade() (bool, error)
	IsParentLockedForSeriesUpgrade() (bool, error)
}

// Relation defines a subset of the functionality provided by the
// state.Relation type, as required by the application facade. For
// details on the methods, see the methods on state.Relation with
// the same names.
type Relation interface {
	status.StatusSetter
	Tag() names.Tag
	Destroy() error
	DestroyWithForce(bool, time.Duration) ([]error, error)
	Id() int
	Endpoints() []state.Endpoint
	RelatedEndpoints(applicationname string) ([]state.Endpoint, error)
	ApplicationSettings(appName string) (map[string]interface{}, error)
	AllRemoteUnits(appName string) ([]RelationUnit, error)
	Unit(string) (RelationUnit, error)
	Endpoint(string) (state.Endpoint, error)
	SetSuspended(bool, string) error
	Suspended() bool
	SuspendedReason() string
}

type RelationUnit interface {
	UnitName() string
	InScope() (bool, error)
	Settings() (map[string]interface{}, error)
}

// Unit defines a subset of the functionality provided by the
// state.Unit type, as required by the application facade. For
// details on the methods, see the methods on state.Unit with
// the same names.
type Unit interface {
	Name() string
	Tag() names.Tag
	UnitTag() names.UnitTag
	ApplicationName() string
	Destroy() error
	DestroyOperation() *state.DestroyUnitOperation
	IsPrincipal() bool
	Life() state.Life
	Resolve(retryHooks bool) error
	AgentTools() (*tools.Tools, error)

	AssignedMachineId() (string, error)
	WorkloadVersion() (string, error)
	AssignWithPolicy(state.AssignmentPolicy) error
	AssignWithPlacement(*instance.Placement) error
	ContainerInfo() (state.CloudContainer, error)
}

// Model defines a subset of the functionality provided by the
// state.Model type, as required by the application facade. For
// details on the methods, see the methods on state.Model with
// the same names.
type Model interface {
	ModelTag() names.ModelTag
	Name() string
	Owner() names.UserTag
	Tag() names.Tag
	Type() state.ModelType
	UUID() string
	ModelConfig() (*config.Config, error)
	AgentVersion() (version.Number, error)
	OpenedPortRangesForMachine(string) (state.MachinePortRanges, error)
	// The following methods are required for querying the featureset
	// supported by the model.
	Config() (*config.Config, error)
	CloudName() string
	Cloud() (cloud.Cloud, error)
	CloudCredential() (state.Credential, bool, error)
	CloudRegion() string
	ControllerUUID() string
}

// Resources defines a subset of the functionality provided by the
// state.Resources type, as required by the application facade. See
// the state.Resources type for details on the methods.
type Resources interface {
	RemovePendingAppResources(string, map[string]string) error
}

type Generation interface {
	AssignApplication(string) error
}

type stateShim struct {
	*state.State
}

type modelShim struct {
	*state.Model
}

type ExternalController state.ExternalController

func (s stateShim) SaveController(controllerInfo crossmodel.ControllerInfo, modelUUID string) (ExternalController, error) {
	api := state.NewExternalControllers(s.State)
	return api.Save(controllerInfo, modelUUID)
}

type StorageInterface interface {
	storagecommon.StorageAccess
	VolumeAccess() storagecommon.VolumeAccess
	FilesystemAccess() storagecommon.FilesystemAccess
}

var getStorageState = func(st *state.State) (StorageInterface, error) {
	m, err := st.Model()
	if err != nil {
		return nil, err
	}
	sb, err := state.NewStorageBackend(st)
	if err != nil {
		return nil, err
	}
	storageAccess := &storageShim{
		StorageAccess: sb,
		va:            sb,
		fa:            sb,
	}
	// CAAS models don't support volume storage yet.
	if m.Type() == state.ModelTypeCAAS {
		storageAccess.va = nil
	}
	return storageAccess, nil
}

type storageShim struct {
	storagecommon.StorageAccess
	fa storagecommon.FilesystemAccess
	va storagecommon.VolumeAccess
}

func (s *storageShim) VolumeAccess() storagecommon.VolumeAccess {
	return s.va
}

func (s *storageShim) FilesystemAccess() storagecommon.FilesystemAccess {
	return s.fa
}

// NewStateApplication converts a state.Application into an Application.
func NewStateApplication(st *state.State, app *state.Application) Application {
	return stateApplicationShim{app, st}
}

// CharmToStateCharm converts a Charm into a state.Charm. This is
// a hack that is required until the State interface methods we
// deal with stop accepting state.Charms, and start accepting
// charm.Charm and charm.URL.
func CharmToStateCharm(ch Charm) *state.Charm {
	return ch.(stateCharmShim).Charm
}

func (s stateShim) Application(name string) (Application, error) {
	a, err := s.State.Application(name)
	if err != nil {
		return nil, err
	}
	return stateApplicationShim{a, s.State}, nil
}

func (s stateShim) AddApplication(args state.AddApplicationArgs) (Application, error) {
	a, err := s.State.AddApplication(args)
	if err != nil {
		return nil, err
	}
	return stateApplicationShim{a, s.State}, nil
}

// Note that the usedID is only used in some of the implementations of the
// AddPendingResource
func (s stateShim) AddPendingResource(appName string, chRes resource.Resource) (string, error) {
	return s.State.Resources().AddPendingResource(appName, "", chRes)
}

// RemovePendingResources removes any pending resources for the named application
// Mainly used as a cleanup if an error is raised during the deployment
func (s stateShim) RemovePendingResources(applicationID string, pendingIDs map[string]string) error {
	return s.State.Resources().RemovePendingAppResources(applicationID, pendingIDs)
}

func (s stateShim) AddCharmMetadata(info state.CharmInfo) (Charm, error) {
	c, err := s.State.AddCharmMetadata(info)
	if err != nil {
		return nil, err
	}
	return stateCharmShim{Charm: c}, nil
}

func (s stateShim) UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error) {
	c, err := s.State.UpdateUploadedCharm(info)
	if err != nil {
		return nil, err
	}
	return stateCharmShim{Charm: c}, nil
}

func (s stateShim) PrepareCharmUpload(curl string) (services.UploadedCharm, error) {
	c, err := s.State.PrepareCharmUpload(curl)
	if err != nil {
		return nil, err
	}
	return stateCharmShim{Charm: c}, nil
}

type remoteApplicationShim struct {
	*state.RemoteApplication
}

type RemoteApplication interface {
	Name() string
	SourceModel() names.ModelTag
	Endpoints() ([]state.Endpoint, error)
	AddEndpoints(eps []charm.Relation) error
	Bindings() map[string]string
	Spaces() []state.RemoteSpace
	Destroy() error
	DestroyOperation(force bool) *state.DestroyRemoteApplicationOperation
	Status() (status.StatusInfo, error)
	Life() state.Life
}

func (s stateShim) RemoteApplication(name string) (RemoteApplication, error) {
	app, err := s.State.RemoteApplication(name)
	return &remoteApplicationShim{app}, err
}

func (s stateShim) AddRemoteApplication(args state.AddRemoteApplicationParams) (RemoteApplication, error) {
	app, err := s.State.AddRemoteApplication(args)
	return &remoteApplicationShim{app}, err
}

func (s stateShim) AddRelation(eps ...state.Endpoint) (Relation, error) {
	r, err := s.State.AddRelation(eps...)
	if err != nil {
		return nil, err
	}
	return stateRelationShim{r, s.State}, nil
}

func (s stateShim) SaveEgressNetworks(relationKey string, cidrs []string) (state.RelationNetworks, error) {
	api := state.NewRelationEgressNetworks(s.State)
	return api.Save(relationKey, false, cidrs)
}

func (s stateShim) Charm(curl string) (Charm, error) {
	ch, err := s.State.Charm(curl)
	if err != nil {
		return nil, err
	}
	return stateCharmShim{ch}, nil
}

func (s stateShim) Model() (Model, error) {
	m, err := s.State.Model()
	if err != nil {
		return nil, err
	}
	return modelShim{m}, nil
}

func (s stateShim) Relation(id int) (Relation, error) {
	r, err := s.State.Relation(id)
	if err != nil {
		return nil, err
	}
	return stateRelationShim{r, s.State}, nil
}

func (s stateShim) InferActiveRelation(names ...string) (Relation, error) {
	r, err := s.State.InferActiveRelation(names...)
	if err != nil {
		return nil, err
	}
	return stateRelationShim{r, s.State}, nil
}

func (s stateShim) Machine(name string) (Machine, error) {
	m, err := s.State.Machine(name)
	if err != nil {
		return nil, err
	}
	return stateMachineShim{m}, nil
}

func (s stateShim) Unit(name string) (Unit, error) {
	u, err := s.State.Unit(name)
	if err != nil {
		return nil, err
	}
	return stateUnitShim{u, s.State}, nil
}

func (s stateShim) UnitsInError() ([]Unit, error) {
	units, err := s.State.UnitsInError()
	if err != nil {
		return nil, err
	}
	result := make([]Unit, len(units))
	for i, u := range units {
		result[i] = stateUnitShim{u, s.State}
	}
	return result, nil
}

func (s stateShim) Resources() Resources {
	return s.State.Resources()
}

type OfferConnection interface {
	UserName() string
	OfferUUID() string
}

func (s stateShim) OfferConnectionForRelation(key string) (OfferConnection, error) {
	return s.State.OfferConnectionForRelation(key)
}

func (s stateShim) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	offers := state.NewApplicationOffers(s.State)
	return offers.ApplicationOfferForUUID(offerUUID)
}

func (s stateShim) Branch(name string) (Generation, error) {
	gen, err := s.State.Branch(name)
	if err != nil {
		return nil, err
	}
	return Generation(gen), nil
}

type stateApplicationShim struct {
	*state.Application
	st *state.State
}

func (a stateApplicationShim) AddUnit(args state.AddUnitParams) (Unit, error) {
	u, err := a.Application.AddUnit(args)
	if err != nil {
		return nil, err
	}
	return stateUnitShim{u, a.st}, nil
}

func (a stateApplicationShim) Charm() (Charm, bool, error) {
	ch, force, err := a.Application.Charm()
	if err != nil {
		return nil, false, err
	}
	return ch, force, nil
}

func (a stateApplicationShim) AllUnits() ([]Unit, error) {
	units, err := a.Application.AllUnits()
	if err != nil {
		return nil, err
	}
	out := make([]Unit, len(units))
	for i, u := range units {
		out[i] = stateUnitShim{u, a.st}
	}
	return out, nil
}

func (a stateApplicationShim) Relations() ([]Relation, error) {
	rels, err := a.Application.Relations()
	if err != nil {
		return nil, err
	}
	out := make([]Relation, len(rels))
	for i, r := range rels {
		out[i] = stateRelationShim{r, a.st}
	}
	return out, nil
}

func (a stateApplicationShim) EndpointBindings() (Bindings, error) {
	return a.Application.EndpointBindings()
}

type stateCharmShim struct {
	*state.Charm
}

type stateMachineShim struct {
	*state.Machine
}

type stateRelationShim struct {
	*state.Relation
	st *state.State
}

func (r stateRelationShim) Unit(unitName string) (RelationUnit, error) {
	u, err := r.st.Unit(unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ru, err := r.Relation.Unit(u)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return stateRelationUnitShim{ru}, nil
}

func (r stateRelationShim) AllRemoteUnits(appName string) ([]RelationUnit, error) {
	rus, err := r.Relation.AllRemoteUnits(appName)
	if err != nil {
		return nil, err
	}
	out := make([]RelationUnit, len(rus))
	for i, ru := range rus {
		out[i] = stateRelationUnitShim{ru}
	}
	return out, nil
}

type stateRelationUnitShim struct {
	*state.RelationUnit
}

func (ru stateRelationUnitShim) Settings() (map[string]interface{}, error) {
	s, err := ru.RelationUnit.Settings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.Map(), nil
}

type stateUnitShim struct {
	*state.Unit
	st *state.State
}

func (u stateUnitShim) AssignWithPolicy(policy state.AssignmentPolicy) error {
	return u.st.AssignUnit(u.Unit, policy)
}

func (u stateUnitShim) AssignWithPlacement(placement *instance.Placement) error {
	return u.st.AssignUnitWithPlacement(u.Unit, placement)
}

type Subnet interface {
	CIDR() string
	VLANTag() int
	ProviderId() network.Id
	ProviderNetworkId() network.Id
}
