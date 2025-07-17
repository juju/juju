// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.state")

// State represents the state of an model
// managed by juju.
type State struct {
	stateClock         clock.Clock
	modelTag           names.ModelTag
	controllerModelTag names.ModelTag
	controllerTag      names.ControllerTag
	session            *mgo.Session
	database           Database
	policy             Policy
	newPolicy          NewPolicyFunc
	maxTxnAttempts     int
	// Note(nvinuesa): Having a dqlite domain service here is an awful hack
	// and should disapear as soon as we migrate units and applications.
	charmServiceGetter func(modelUUID coremodel.UUID) (CharmService, error)
}

// IsController returns true if this state instance has the bootstrap
// model UUID.
func (st *State) IsController() bool {
	return st.modelTag == st.controllerModelTag
}

// ControllerUUID returns the UUID for the controller
// of this state instance.
func (st *State) ControllerUUID() string {
	return st.controllerTag.Id()
}

// ControllerTag returns the tag form of the ControllerUUID.
func (st *State) ControllerTag() names.ControllerTag {
	return st.controllerTag
}

// ControllerTimestamp returns the current timestamp of the backend
// controller.
func (st *State) ControllerTimestamp() (*time.Time, error) {
	now := time.Now()
	return &now, nil
}

// ControllerModelUUID returns the UUID of the model that was
// bootstrapped.  This is the only model that can have controller
// machines.  The owner of this model is also considered "special", in
// that they are the only user that is able to create other users
// (until we have more fine grained permissions), and they cannot be
// disabled.
func (st *State) ControllerModelUUID() string {
	return st.controllerModelTag.Id()
}

// RemoveDyingModel sets current model to dead then removes all documents from
// multi-model collections.
func (st *State) RemoveDyingModel() error {
	return nil
}

// ModelUUID returns the model UUID for the model
// controlled by this state instance.
func (st *State) ModelUUID() string {
	return st.modelTag.Id()
}

// EnsureModelRemoved returns an error if any multi-model
// documents for this model are found. It is intended only to be used in
// tests and exported so it can be used in the tests of other packages.
func (st *State) EnsureModelRemoved() error {
	return nil
}

// Ping probes the state's database connection to ensure
// that it is still alive.
func (st *State) Ping() error {
	return nil
}

// MongoVersion return the string repre
func (st *State) MongoVersion() (string, error) {
	return "-4.4", nil
}

// MongoSession returns the underlying mongodb session
// used by the state. It is exposed so that external code
// can maintain the mongo replica set and should not
// otherwise be used.
func (st *State) MongoSession() *mgo.Session {
	return nil
}

// Upgrader is an interface that can be used to check if an upgrade is in
// progress.
type Upgrader interface {
	IsUpgrading() (bool, error)
}

// SetModelAgentVersion changes the agent version for the model to the
// given version, only if the model is in a stable state (all agents are
// running the current version). If this is a hosted model, newVersion
// cannot be higher than the controller version.
func (st *State) SetModelAgentVersion(newVersion semversion.Number, stream *string, ignoreAgentVersions bool, upgrader Upgrader) (err error) {
	return nil
}

// SaveCloudServiceArgs defines the arguments for SaveCloudService method.
type SaveCloudServiceArgs struct {
	// Id will be the application Name if it's a part of application,
	// and will be controller UUID for k8s a controller(controller does not have an application),
	// then is wrapped with applicationGlobalKey.
	Id         string
	ProviderId string
	Addresses  network.SpaceAddresses

	Generation            int64
	DesiredScaleProtected bool
}

// CharmRef is an indirection to a charm, this allows us to pass in a charm,
// without having a full concrete charm.
type CharmRef interface {
	Meta() *charm.Meta
	Manifest() *charm.Manifest
}

// CharmRefFull is actually almost a full charm with addition information. This
// is purely here as a hack to push a charm from the dqlite layer to the state
// layer.
// Deprecated: This is an abomination and should be removed.
type CharmRefFull interface {
	CharmRef

	Actions() *charm.Actions
	Config() *charm.Config
	Revision() int
	URL() string
	Version() string
}

// AddApplicationArgs defines the arguments for AddApplication method.
type AddApplicationArgs struct {
	Name              string
	Charm             CharmRef
	CharmURL          string
	CharmOrigin       *CharmOrigin
	Storage           map[string]StorageConstraints
	AttachStorage     []names.StorageTag
	EndpointBindings  map[string]string
	ApplicationConfig *config.Config
	CharmConfig       charm.Settings
	NumUnits          int
	Placement         []*instance.Placement
	Constraints       constraints.Value
	Resources         map[string]string
}

// AddApplication creates a new application, running the supplied charm, with the
// supplied name (which must be unique). If the charm defines peer relations,
// they will be created automatically.
func (st *State) AddApplication(
	args AddApplicationArgs,
	store objectstore.ObjectStore,
) (_ *Application, err error) {
	return &Application{st: st}, nil
}

// Application returns an application state by name.
func (st *State) Application(name string) (_ *Application, err error) {
	return &Application{st: st, doc: applicationDoc{Name: name}}, nil
}

// Report conforms to the Dependency Engine Report() interface, giving an opportunity to introspect
// what is going on at runtime.
func (st *State) Report() map[string]interface{} {
	return nil
}

// Unit returns a unit by name.
func (st *State) Unit(name string) (*Unit, error) {
	app, _ := names.UnitApplication(name)
	return &Unit{st: st, doc: unitDoc{Name: name, Application: app}}, nil
}

// TagFromDocID tries attempts to extract an entity-identifying tag from a
// Mongo document ID.
// For example "c9741ea1-0c2a-444d-82f5-787583a48557:a#mediawiki" would yield
// an application tag for "mediawiki"
func TagFromDocID(docID string) names.Tag {
	return nil
}
