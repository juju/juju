// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"

	coremodel "github.com/juju/juju/core/model"
	internalpassword "github.com/juju/juju/internal/password"
	stateerrors "github.com/juju/juju/state/errors"
)

// modelGlobalKey is the key for the model, its
// settings and constraints.
const modelGlobalKey = "e"

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

// modelEntityRefsDoc records references to the top-level entities in the
// model.  This is also used to determine if a model can be destroyed.
// Consequently, any changes, especially additions of entities, here, would
// need to be reflected, at least, in Model.checkModelEntityRefsEmpty(...) as
// well as Model.destroyOps(...)
type modelEntityRefsDoc struct {
	UUID string `bson:"_id"`

	// Machines contains the names of the top-level machines in the model.
	Machines []string `bson:"machines"`

	// Applications contains the names of the applications in the model.
	Applications []string `bson:"applications"`

	// Volumes contains the IDs of the volumes in the model.
	Volumes []string `bson:"volumes"`

	// Filesystems contains the IDs of the filesystems in the model.
	Filesystems []string `bson:"filesystems"`
}

// Model returns the model entity.
func (st *State) Model() (*Model, error) {
	model := &Model{
		st: st,
	}
	if err := model.refresh(st.modelTag.Id()); err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

// AllModelUUIDs returns the UUIDs for all non-dead models in the controller.
// Results are sorted by (name, owner).
func (st *State) AllModelUUIDs() ([]string, error) {
	return st.filteredModelUUIDs(notDeadDoc)
}

// AllModelUUIDsIncludingDead returns the UUIDs for all models in the controller.
// Results are sorted by (name, owner).
func (st *State) AllModelUUIDsIncludingDead() ([]string, error) {
	return st.filteredModelUUIDs(nil)
}

// filteredModelUUIDs returns all model uuids that match the filter.
func (st *State) filteredModelUUIDs(filter bson.D) ([]string, error) {
	models, closer := st.db().GetCollection(modelsC)
	defer closer()

	var docs []bson.M
	err := models.Find(filter).Sort("name", "owner").Select(bson.M{"_id": 1}).All(&docs)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(docs))
	for i, doc := range docs {
		out[i] = doc["_id"].(string)
	}
	return out, nil
}

// ModelExists returns true if a model with the supplied UUID exists.
func (st *State) ModelExists(uuid string) (bool, error) {
	models, closer := st.db().GetCollection(modelsC)
	defer closer()

	count, err := models.FindId(uuid).Count()
	if err != nil {
		return false, errors.Annotate(err, "querying model")
	}
	return count > 0, nil
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

	// PasswordHash is used by the caas model operator.
	PasswordHash string
}

// Validate validates the ModelArgs.
func (m ModelArgs) Validate() error {
	if m.Type == modelTypeNone {
		return errors.NotValidf("empty Type")
	}
	if m.Owner == (names.UserTag{}) {
		return errors.NotValidf("empty Owner")
	}
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
	st, err := ctlr.pool.SystemState()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	if err := args.Validate(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	controllerInfo, err := st.ControllerInfo()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var prereqOps []txn.Op

	// Ensure that the cloud credential is valid, or if one is not
	// specified, that the cloud supports the "empty" authentication
	// type.
	owner := args.Owner
	// We no longer validate args.CloudCredential here as credential
	// management is moved to domain/credential.

	uuid := args.UUID.String()
	session := st.session.Copy()
	newSt, err := newState(
		st.controllerTag,
		names.NewModelTag(uuid),
		controllerInfo.ModelTag,
		session,
		st.newPolicy,
		st.clock(),
		st.charmServiceGetter,
		st.maxTxnAttempts,
	)
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not create state for new model")
	}
	defer func() {
		if err != nil {
			newSt.Close()
		}
	}()
	newSt.controllerModelTag = st.controllerModelTag

	modelOps, err := newSt.modelSetupOps(st.controllerTag.Id(), args)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create new model")
	}

	ops := append(prereqOps, modelOps...)

	err = newSt.db().RunTransaction(ops)
	if err == txn.ErrAborted {
		name := args.Name
		models, closer := st.db().GetCollection(modelsC)
		defer closer()
		modelCount, countErr := models.Find(bson.D{
			{"owner", owner.Id()},
			{"name", name}},
		).Count()
		if countErr != nil {
			err = errors.Trace(countErr)
		} else if modelCount > 0 {
			err = errors.AlreadyExistsf("model %q for %s", name, owner.Id())
		} else {
			err = errors.Annotate(err, "failed to create new model")
		}
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	newModel, err := newSt.Model()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// TODO(storage) - we need to add the default storage pools using the new dqlite model service
	//storageService, registry, err := newSt.storageServices()
	//if err != nil {
	//	return nil, nil, errors.Trace(err)
	//}
	//pools, err := domainstorage.DefaultStoragePools(registry)
	//if err != nil {
	//	return nil, nil, errors.Trace(err)
	//}
	//for _, pool := range pools {
	//	attr := transform.Map(pool.Attrs, func(k, v string) (string, any) { return k, v })
	//	err = storageService.CreateStoragePool(context.Background(), pool.Name, storage.ProviderType(pool.Provider), attr)
	//	if err != nil {
	//		logger.Criticalf(context.TODO(), "saving storage pool %q: %v", pool.Name, err)
	//		//			return nil, nil, errors.Annotatef(err, "saving storage pool %q", pool.Name)
	//	}
	//}
	return newModel, newSt, nil
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
	if len(password) < internalpassword.MinAgentPasswordLength {
		return fmt.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	passwordHash := internalpassword.AgentPasswordHash(password)
	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.doc.UUID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"passwordhash", passwordHash}}}},
	}}
	err := m.st.db().RunTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set password of model %q: %v", m, onAbort(err, stateerrors.ErrDead))
	}
	m.doc.PasswordHash = passwordHash
	return nil
}

// String returns the model name.
func (m *Model) String() string {
	return m.doc.Name
}

// PasswordValid returns whether the given password is valid
// for the given application.
func (m *Model) PasswordValid(password string) bool {
	agentHash := internalpassword.AgentPasswordHash(password)
	if agentHash == m.doc.PasswordHash {
		return true
	}
	return false
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

// CloudCredentialTag returns the tag of the cloud credential used for managing the
// model's cloud resources, and a boolean indicating whether a credential is set.
func (m *Model) CloudCredentialTag() (names.CloudCredentialTag, bool) {
	if names.IsValidCloudCredential(m.doc.CloudCredential) {
		return names.NewCloudCredentialTag(m.doc.CloudCredential), true
	}
	return names.CloudCredentialTag{}, false
}

// Life returns whether the model is Alive, Dying or Dead.
func (m *Model) Life() Life {
	return m.doc.Life
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

// localID returns the local id value by stripping off the model uuid prefix
// if it is there.
func (m *Model) localID(id string) string {
	modelUUID, localID, ok := splitDocID(id)
	if !ok || modelUUID != m.doc.UUID {
		return id
	}
	return localID
}

// globalKey returns the global database key for the model.
func (m *Model) globalKey() string {
	return modelGlobalKey
}

func (m *Model) Refresh() error {
	return m.refresh(m.UUID())
}

func (m *Model) refresh(uuid string) error {
	models, closer := m.st.db().GetCollection(modelsC)
	defer closer()
	err := models.FindId(uuid).One(&m.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("model %q", uuid)
	}
	return err
}

// IsControllerModel returns a boolean indicating whether
// this model is responsible for running a controller.
func (m *Model) IsControllerModel() bool {
	return m.st.controllerModelTag.Id() == m.doc.UUID
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

func (m *Model) uniqueIndexID() string {
	return userModelNameIndex(m.doc.Owner, m.doc.Name)
}

// Destroy sets the models's lifecycle to Dying, preventing
// addition of applications or machines to state. If called on
// an empty hosted model, the lifecycle will be advanced
// straight to Dead.
func (m *Model) Destroy(args DestroyModelParams) (err error) {
	return
}

// getEntityRefs reads the current model entity refs document for the model.
func (m *Model) getEntityRefs() (*modelEntityRefsDoc, error) {
	modelEntityRefs, closer := m.st.db().GetCollection(modelEntityRefsC)
	defer closer()

	var doc modelEntityRefsDoc
	if err := modelEntityRefs.FindId(m.UUID()).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("entity references doc for model %s", m.UUID())
		}
		return nil, errors.Annotatef(err, "getting entity references for model %s", m.UUID())
	}
	return &doc, nil
}

// (TODO) externalreality: Temporary method to access state from model while
// factoring Model concerns out from state.
func (model *Model) State() *State {
	return model.st
}

// checkModelEntityRefsEmpty checks that the model is empty of any entities
// that may require external resource cleanup. If the model is not empty,
// then an error will be returned; otherwise txn.Ops are returned to assert
// the continued emptiness.
func checkModelEntityRefsEmpty(doc *modelEntityRefsDoc) ([]txn.Op, error) {
	// These errors could be potentially swallowed as we re-try to destroy model.
	// Let's, at least, log them for observation.
	err := stateerrors.NewModelNotEmptyError(
		len(doc.Machines),
		len(doc.Applications),
		len(doc.Volumes),
		len(doc.Filesystems),
	)
	if err != nil {
		return nil, err
	}

	isEmpty := func(attribute string) bson.DocElem {
		// We consider it empty if the array has no entries, or if the attribute doesn't exist
		return bson.DocElem{
			Name: "$or", Value: []bson.M{{
				attribute: bson.M{"$exists": false},
			}, {
				attribute: bson.M{"$size": 0},
			}},
		}
	}

	return []txn.Op{{
		C:  modelEntityRefsC,
		Id: doc.UUID,
		Assert: bson.D{
			isEmpty("machines"),
			isEmpty("applications"),
			isEmpty("volumes"),
			isEmpty("filesystems"),
		},
	}}, nil
}

func addModelApplicationRefOp(mb modelBackend, applicationname string) txn.Op {
	return addModelEntityRefOp(mb, "applications", applicationname)
}

func addModelVolumeRefOp(mb modelBackend, volumeId string) txn.Op {
	return addModelEntityRefOp(mb, "volumes", volumeId)
}

func addModelFilesystemRefOp(mb modelBackend, filesystemId string) txn.Op {
	return addModelEntityRefOp(mb, "filesystems", filesystemId)
}

func addModelEntityRefOp(mb modelBackend, entityField, entityId string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     mb.ModelUUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$addToSet", bson.D{{entityField, entityId}}}},
	}
}

// createModelOp returns the operation needed to create
// an model document with the given name and UUID.
func createModelOp(
	modelType ModelType,
	owner names.UserTag,
	name, uuid, controllerUUID, cloudName, cloudRegion, passwordHash string,
	cloudCredential names.CloudCredentialTag,
) txn.Op {
	doc := &modelDoc{
		Type:            modelType,
		UUID:            uuid,
		Name:            name,
		Life:            Alive,
		Owner:           owner.Id(),
		ControllerUUID:  controllerUUID,
		Cloud:           cloudName,
		CloudRegion:     cloudRegion,
		CloudCredential: cloudCredential.Id(),
		PasswordHash:    passwordHash,
	}
	return txn.Op{
		C:      modelsC,
		Id:     uuid,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func createModelEntityRefsOp(uuid string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     uuid,
		Assert: txn.DocMissing,
		Insert: &modelEntityRefsDoc{UUID: uuid},
	}
}

const hostedModelCountKey = "hostedModelCount"

type hostedModelCountDoc struct {
	// RefCount is the number of models in the Juju system.
	// We do not count the system model.
	RefCount int `bson:"refcount"`
}

func incHostedModelCountOp() txn.Op {
	return HostedModelCountOp(1)
}

func decHostedModelCountOp() txn.Op {
	return HostedModelCountOp(-1)
}

func HostedModelCountOp(amount int) txn.Op {
	return txn.Op{
		C:      controllersC,
		Id:     hostedModelCountKey,
		Assert: txn.DocExists,
		Update: bson.M{
			"$inc": bson.M{"refcount": amount},
		},
	}
}

// createUniqueOwnerModelNameOp returns the operation needed to create
// a usermodelnameC document with the given owner and model name.
func createUniqueOwnerModelNameOp(owner names.UserTag, modelName string) txn.Op {
	return txn.Op{
		C:      usermodelnameC,
		Id:     userModelNameIndex(owner.Id(), modelName),
		Assert: txn.DocMissing,
		Insert: bson.M{},
	}
}

// assertModelActiveOp returns a txn.Op that asserts the given
// model UUID refers to an Alive model.
func assertModelActiveOp(modelUUID string) txn.Op {
	return assertModelUsableOp(modelUUID, isAliveDoc)
}

func assertModelUsableOp(modelUUID string, lifeAssertion bson.D) txn.Op {
	return txn.Op{
		C:      modelsC,
		Id:     modelUUID,
		Assert: lifeAssertion, // bson.DocElem{Name: "migration-mode", Value: MigrationModeNone}),
	}
}

func checkModelActive(st *State) error {
	return errors.Trace(checkModelUsable(st, func(life Life) bool { return life == Alive }))
}

func checkModelUsable(st *State, validLife func(Life) bool) error {
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	if !validLife(model.Life()) {
		return errors.Errorf("model %q is %s", model.Name(), model.Life().String())
	}

	return nil
}
