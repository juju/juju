// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	coremodel "github.com/juju/juju/core/model"
)

// ModelType signals the type of a model - IAAS or CAAS
type ModelType string

const (
	modelTypeNone = ModelType("")
	ModelTypeIAAS = ModelType("iaas")
	ModelTypeCAAS = ModelType("caas")
)

// ParseModelType turns a valid model type string into a ModelType
// constant.
func ParseModelType(raw string) (ModelType, error) {
	for _, typ := range []ModelType{ModelTypeIAAS, ModelTypeCAAS} {
		if raw == string(typ) {
			return typ, nil
		}
	}
	return "", errors.NotValidf("model type %v", raw)
}

// Model represents the state of a model.
type Model struct {
	st  *State
	doc modelDoc
}

// modelDoc represents the internal state of the model in MongoDB.
type modelDoc struct {
	UUID           string    `bson:"_id"`
	Name           string    `bson:"name"`
	Type           ModelType `bson:"type"`
	Life           Life      `bson:"life"`
	Owner          string    `bson:"owner"`
	ControllerUUID string    `bson:"controller-uuid"`

	// Cloud is the name of the cloud to which the model is deployed.
	Cloud string `bson:"cloud"`

	// CloudRegion is the name of the cloud region to which the model is
	// deployed. This will be empty for clouds that do not support regions.
	CloudRegion string `bson:"cloud-region,omitempty"`

	// CloudCredential is the ID of the cloud credential that is used
	// for managing cloud resources for this model. This will be empty
	// for clouds that do not require credentials.
	CloudCredential string `bson:"cloud-credential,omitempty"`

	// LatestAvailableTools is a string representing the newest version
	// found while checking streams for new versions.
	LatestAvailableTools string `bson:"available-tools,omitempty"`

	// PasswordHash is used by the caas model operator.
	PasswordHash string `bson:"passwordhash"`

	// ForceDestroyed is whether --force was specified when destroying
	// this model. It only has any meaning when the model is dying or
	// dead.
	ForceDestroyed bool `bson:"force-destroyed,omitempty"`

	// DestroyTimeout is the timeout passed in when the
	// model was destroyed.
	DestroyTimeout *time.Duration `bson:"destroy-timeout,omitempty"`
}

// Model returns the model entity.
func (st *State) Model() (*Model, error) {
	model := &Model{
		st: st,
	}
	return model, nil
}

// AllModelUUIDs returns the UUIDs for all non-dead models in the controller.
// Results are sorted by (name, owner).
func (st *State) AllModelUUIDs() ([]string, error) {
	return nil, nil
}

// ModelArgs is a params struct for creating a new model.
type ModelArgs struct {
	UUID coremodel.UUID

	Name string

	// Type specifies the general type of the model (IAAS or CAAS).
	Type ModelType

	// CloudName is the name of the cloud to which the model is deployed.
	CloudName string

	// CloudRegion is the name of the cloud region to which the model is
	// deployed. This will be empty for clouds that do not support regions.
	CloudRegion string

	// CloudCredential is the tag of the cloud credential that will be
	// used for managing cloud resources for this model. This will be
	// empty for clouds that do not require credentials.
	CloudCredential names.CloudCredentialTag

	// Owner is the user that owns the model.
	Owner names.UserTag
}

// Validate validates the ModelArgs.
func (m ModelArgs) Validate() error {
	return nil
}

// NewModel creates a new model with its own UUID and
// prepares it for use. Model and State instances for the new
// model are returned.
//
// The controller model's UUID is attached to the new
// model's document. Having the server UUIDs stored with each
// model document means that we have a way to represent external
// models, perhaps for future use around cross model
// relations.
func (ctlr *Controller) NewModel(args ModelArgs) (_ *Model, _ *State, err error) {
	return &Model{st: &State{}}, &State{}, nil
}

// Tag returns a name identifying the model.
// The returned name will be different from other Tag values returned
// by any other entities from the same state.
func (m *Model) Tag() names.Tag {
	return m.ModelTag()
}

// ModelTag is the concrete model tag for this model.
func (m *Model) ModelTag() names.ModelTag {
	return names.NewModelTag(m.doc.UUID)
}

// SetPassword sets the password for the model's agent.
func (m *Model) SetPassword(password string) error {
	return nil
}

// String returns the model name.
func (m *Model) String() string {
	return m.doc.Name
}

// PasswordValid returns whether the given password is valid
// for the given application.
func (m *Model) PasswordValid(password string) bool {
	return true
}

// ControllerTag is the tag for the controller that the model is
// running within.
func (m *Model) ControllerTag() names.ControllerTag {
	return names.NewControllerTag(m.doc.ControllerUUID)
}

// UUID returns the universally unique identifier of the model.
func (m *Model) UUID() string {
	return m.doc.UUID
}

// ControllerUUID returns the universally unique identifier of the controller
// in which the model is running.
func (m *Model) ControllerUUID() string {
	return m.doc.ControllerUUID
}

// Name returns the human friendly name of the model.
func (m *Model) Name() string {
	return m.doc.Name
}

// Type returns the type of the model.
func (m *Model) Type() ModelType {
	return m.doc.Type
}

// CloudName returns the name of the cloud to which the model is deployed.
func (m *Model) CloudName() string {
	return m.doc.Cloud
}

// CloudRegion returns the name of the cloud region to which the model is deployed.
func (m *Model) CloudRegion() string {
	return m.doc.CloudRegion
}

// Life returns whether the model is Alive, Dying or Dead.
func (m *Model) Life() Life {
	return Alive
}

// ForceDestroyed returns whether the destruction of a dying/dead
// model was forced. It's always false for a model that's alive.
func (m *Model) ForceDestroyed() bool {
	return m.doc.ForceDestroyed
}

// DestroyTimeout returns the timeout passed in when the
// model was destroyed.
func (m *Model) DestroyTimeout() *time.Duration {
	return m.doc.DestroyTimeout
}

// Owner returns tag representing the owner of the model.
// The owner is the user that created the model.
func (m *Model) Owner() names.UserTag {
	return names.NewUserTag(m.doc.Owner)
}

func (m *Model) Refresh() error {
	return nil
}

// IsControllerModel returns a boolean indicating whether
// this model is responsible for running a controller.
func (m *Model) IsControllerModel() bool {
	return m.st.modelTag == m.st.controllerModelTag
}

// DestroyModelParams contains parameters for destroy a model.
type DestroyModelParams struct {
	// DestroyHostedModels controls whether or not hosted models
	// are destroyed also. This only applies to the controller
	// model.
	//
	// If this is false when destroying the controller model,
	// there must be no hosted models, or an error satisfying
	// HasHostedModelsError will be returned.
	//
	// TODO(axw) this should be moved to the Controller type.
	DestroyHostedModels bool

	// DestroyStorage controls whether or not storage in the
	// model (and hosted models, if DestroyHostedModels is true)
	// should be destroyed.
	//
	// This is ternary: nil, false, or true. If nil and
	// there is persistent storage in the model (or hosted
	// models), an error satisfying PersistentStorageError
	// will be returned.
	DestroyStorage *bool

	// Force specifies whether model destruction will be forced, i.e.
	// keep going despite operational errors.
	Force *bool

	// MaxWait specifies the amount of time that each step in model destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait time.Duration

	Timeout *time.Duration
}

// Destroy sets the models's lifecycle to Dying, preventing
// addition of applications or machines to state. If called on
// an empty hosted model, the lifecycle will be advanced
// straight to Dead.
func (m *Model) Destroy(args DestroyModelParams) (err error) {
	return
}

// (TODO) externalreality: Temporary method to access state from model while
// factoring Model concerns out from state.
func (model *Model) State() *State {
	return model.st
}
