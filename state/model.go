// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
)

// modelGlobalKey is the key for the model, its
// settings and constraints.
const modelGlobalKey = "e"

// modelKey will create the kei for a given model using the modelGlobalKey.
func modelKey(modelUUID string) string {
	return fmt.Sprintf("%s#%s", modelGlobalKey, modelUUID)
}

// MigrationMode specifies where the Model is with respect to migration.
type MigrationMode string

const (
	// MigrationModeNone is the default mode for a model and reflects
	// that it isn't involved with a model migration.
	MigrationModeNone MigrationMode = ""

	// MigrationModeExporting reflects a model that is in the process of being
	// exported from one controller to another.
	MigrationModeExporting MigrationMode = "exporting"

	// MigrationModeImporting reflects a model that is being imported into a
	// controller, but is not yet fully active.
	MigrationModeImporting MigrationMode = "importing"
)

// Model represents the state of a model.
type Model struct {
	// st is not necessarily the state of this model. Though it is
	// usually safe to assume that it is. The only times it isn't is when we
	// get models other than the current one - which is mostly in
	// controller api endpoints.
	st  *State
	doc modelDoc
}

// modelDoc represents the internal state of the model in MongoDB.
type modelDoc struct {
	UUID           string `bson:"_id"`
	Name           string
	Life           Life
	Owner          string        `bson:"owner"`
	ControllerUUID string        `bson:"controller-uuid"`
	MigrationMode  MigrationMode `bson:"migration-mode"`

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
}

// modelEntityRefsDoc records references to the top-level entities
// in the model.
type modelEntityRefsDoc struct {
	UUID string `bson:"_id"`

	// Machines contains the names of the top-level machines in the model.
	Machines []string `bson:"machines"`

	// Applicatons contains the names of the applications in the model.
	Applications []string `bson:"applications"`
}

// ControllerModel returns the model that was bootstrapped.
// This is the only model that can have controller machines.
// The owner of this model is also considered "special", in that
// they are the only user that is able to create other users (until we
// have more fine grained permissions), and they cannot be disabled.
func (st *State) ControllerModel() (*Model, error) {
	ssinfo, err := st.ControllerInfo()
	if err != nil {
		return nil, errors.Annotate(err, "could not get controller info")
	}

	models, closer := st.getCollection(modelsC)
	defer closer()

	env := &Model{st: st}
	uuid := ssinfo.ModelTag.Id()
	if err := env.refresh(models.FindId(uuid)); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// Model returns the model entity.
func (st *State) Model() (*Model, error) {
	return st.GetModel(st.modelTag)
}

// GetModel looks for the model identified by the uuid passed in.
func (st *State) GetModel(tag names.ModelTag) (*Model, error) {
	models, closer := st.getCollection(modelsC)
	defer closer()

	model := &Model{st: st}
	if err := model.refresh(models.FindId(tag.Id())); err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

// AllModels returns all the models in the system.
func (st *State) AllModels() ([]*Model, error) {
	models, closer := st.getCollection(modelsC)
	defer closer()

	var modelDocs []modelDoc
	err := models.Find(nil).All(&modelDocs)
	if err != nil {
		return nil, err
	}

	result := make([]*Model, len(modelDocs))
	for i, doc := range modelDocs {
		result[i] = &Model{st: st, doc: doc}
	}

	return result, nil
}

// ModelArgs is a params struct for creating a new model.
type ModelArgs struct {
	// CloudName is the name of the cloud to which the model is deployed.
	CloudName string

	// CloudRegion is the name of the cloud region to which the model is
	// deployed. This will be empty for clouds that do not support regions.
	CloudRegion string

	// CloudCredential is the tag of the cloud credential that will be
	// used for managing cloud resources for this model. This will be
	// empty for clouds that do not require credentials.
	CloudCredential names.CloudCredentialTag

	// Config is the model config.
	Config *config.Config

	// Constraints contains the initial constraints for the model.
	Constraints constraints.Value

	// StorageProviderRegistry is used to determine and store the
	// details of the default storage pools.
	StorageProviderRegistry storage.ProviderRegistry

	// Owner is the user that owns the model.
	Owner names.UserTag

	// MigrationMode is the initial migration mode of the model.
	MigrationMode MigrationMode
}

// Validate validates the ModelArgs.
func (m ModelArgs) Validate() error {
	if m.Config == nil {
		return errors.NotValidf("nil Config")
	}
	if !names.IsValidCloud(m.CloudName) {
		return errors.NotValidf("Cloud Name %q", m.CloudName)
	}
	if m.Owner == (names.UserTag{}) {
		return errors.NotValidf("empty Owner")
	}
	if m.StorageProviderRegistry == nil {
		return errors.NotValidf("nil StorageProviderRegistry")
	}
	switch m.MigrationMode {
	case MigrationModeNone, MigrationModeImporting:
	default:
		return errors.NotValidf("initial migration mode %q", m.MigrationMode)
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
func (st *State) NewModel(args ModelArgs) (_ *Model, _ *State, err error) {
	if err := args.Validate(); err != nil {
		return nil, nil, errors.Trace(err)
	}
	// For now, the model cloud must be the same as the controller cloud.
	controllerInfo, err := st.ControllerInfo()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if controllerInfo.CloudName != args.CloudName {
		return nil, nil, errors.NewNotValid(
			nil, fmt.Sprintf("controller cloud %s does not match model cloud %s", controllerInfo.CloudName, args.CloudName))
	}

	// Ensure that the cloud region is valid, or if one is not specified,
	// that the cloud does not support regions.
	controllerCloud, err := st.Cloud(args.CloudName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	assertCloudRegionOp, err := validateCloudRegion(controllerCloud, args.CloudName, args.CloudRegion)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Ensure that the cloud credential is valid, or if one is not
	// specified, that the cloud supports the "empty" authentication
	// type.
	owner := args.Owner
	cloudCredentials, err := st.CloudCredentials(owner, args.CloudName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	assertCloudCredentialOp, err := validateCloudCredential(
		controllerCloud, args.CloudName, cloudCredentials, args.CloudCredential,
	)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	if owner.IsLocal() {
		if _, err := st.User(owner); err != nil {
			return nil, nil, errors.Annotate(err, "cannot create model")
		}
	}

	uuid := args.Config.UUID()
	session := st.session.Copy()
	newSt, err := newState(names.NewModelTag(uuid), controllerInfo.ModelTag, session, st.mongoInfo, st.newPolicy)
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not create state for new model")
	}
	defer func() {
		if err != nil {
			newSt.Close()
		}
	}()
	newSt.controllerModelTag = st.controllerModelTag

	modelOps, err := newSt.modelSetupOps(st.controllerTag.Id(), args, nil)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create new model")
	}

	prereqOps := []txn.Op{
		assertCloudRegionOp,
		assertCloudCredentialOp,
	}
	ops := append(prereqOps, modelOps...)
	err = newSt.runTransaction(ops)
	if err == txn.ErrAborted {

		// We have a  unique key restriction on the "owner" and "name" fields,
		// which will cause the insert to fail if there is another record with
		// the same "owner" and "name" in the collection. If the txn is
		// aborted, check if it is due to the unique key restriction.
		name := args.Config.Name()
		models, closer := st.getCollection(modelsC)
		defer closer()
		envCount, countErr := models.Find(bson.D{
			{"owner", owner.Canonical()},
			{"name", name}},
		).Count()
		if countErr != nil {
			err = errors.Trace(countErr)
		} else if envCount > 0 {
			err = errors.AlreadyExistsf("model %q for %s", name, owner.Canonical())
		} else {
			err = errors.New("model already exists")
		}
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	err = newSt.start(st.controllerTag)
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not start state for new model")
	}

	newModel, err := newSt.Model()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	_, err = newSt.SetUserAccess(newModel.Owner(), newModel.ModelTag(), description.AdminAccess)
	if err != nil {
		return nil, nil, errors.Annotate(err, "granting admin permission to the owner")
	}

	return newModel, newSt, nil
}

// validateCloudRegion validates the given region name against the
// provided Cloud definition, and returns a txn.Op to include in a
// transaction to assert the same.
func validateCloudRegion(cloud jujucloud.Cloud, cloudName, regionName string) (txn.Op, error) {
	// Ensure that the cloud region is valid, or if one is not specified,
	// that the cloud does not support regions.
	assertCloudRegionOp := txn.Op{
		C:  cloudsC,
		Id: cloudName,
	}
	if regionName != "" {
		region, err := jujucloud.RegionByName(cloud.Regions, regionName)
		if err != nil {
			return txn.Op{}, errors.Trace(err)
		}
		assertCloudRegionOp.Assert = bson.D{
			{"regions." + region.Name, bson.D{{"$exists", true}}},
		}
	} else {
		if len(cloud.Regions) > 0 {
			return txn.Op{}, errors.NotValidf("missing CloudRegion")
		}
		assertCloudRegionOp.Assert = bson.D{
			{"regions", bson.D{{"$exists", false}}},
		}
	}
	return assertCloudRegionOp, nil
}

// validateCloudCredential validates the given cloud credential
// name against the provided cloud definition and credentials,
// and returns a txn.Op to include in a transaction to assert the
// same. A user is supplied, for which access to the credential
// will be asserted.
func validateCloudCredential(
	cloud jujucloud.Cloud,
	cloudName string,
	cloudCredentials map[names.CloudCredentialTag]jujucloud.Credential,
	cloudCredential names.CloudCredentialTag,
) (txn.Op, error) {
	if cloudCredential != (names.CloudCredentialTag{}) {
		if cloudCredential.Cloud().Id() != cloudName {
			return txn.Op{}, errors.NotValidf("credential %q", cloudCredential.Id())
		}
		var found bool
		for tag := range cloudCredentials {
			if tag.Canonical() == cloudCredential.Canonical() {
				found = true
				break
			}
		}
		if !found {
			return txn.Op{}, errors.NotFoundf("credential %q", cloudCredential.Id())
		}
		// NOTE(axw) if we add ACLs for credentials,
		// we'll need to check access here. The map
		// we check above contains only the credentials
		// that the model owner has access to.
		return txn.Op{
			C:      cloudCredentialsC,
			Id:     cloudCredentialDocID(cloudCredential),
			Assert: txn.DocExists,
		}, nil
	}
	var hasEmptyAuth bool
	for _, authType := range cloud.AuthTypes {
		if authType != jujucloud.EmptyAuthType {
			continue
		}
		hasEmptyAuth = true
		break
	}
	if !hasEmptyAuth {
		return txn.Op{}, errors.NotValidf("missing CloudCredential")
	}
	return txn.Op{
		C:      cloudsC,
		Id:     cloudName,
		Assert: bson.D{{"auth-types", string(jujucloud.EmptyAuthType)}},
	}, nil
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

// Cloud returns the name of the cloud to which the model is deployed.
func (m *Model) Cloud() string {
	return m.doc.Cloud
}

// CloudRegion returns the name of the cloud region to which the model is deployed.
func (m *Model) CloudRegion() string {
	return m.doc.CloudRegion
}

// CloudCredential returns the tag of the cloud credential used for managing the
// model's cloud resources, and a boolean indicating whether a credential is set.
func (m *Model) CloudCredential() (names.CloudCredentialTag, bool) {
	if names.IsValidCloudCredential(m.doc.CloudCredential) {
		return names.NewCloudCredentialTag(m.doc.CloudCredential), true
	}
	return names.CloudCredentialTag{}, false
}

// MigrationMode returns whether the model is active or being migrated.
func (m *Model) MigrationMode() MigrationMode {
	return m.doc.MigrationMode
}

// SetMigrationMode updates the migration mode of the model.
func (m *Model) SetMigrationMode(mode MigrationMode) error {
	st, closeState, err := m.getState()
	if err != nil {
		return errors.Trace(err)
	}
	defer closeState()

	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.doc.UUID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"migration-mode", mode}}}},
	}}
	if err := st.runTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return m.Refresh()
}

// Life returns whether the model is Alive, Dying or Dead.
func (m *Model) Life() Life {
	return m.doc.Life
}

// Owner returns tag representing the owner of the model.
// The owner is the user that created the model.
func (m *Model) Owner() names.UserTag {
	return names.NewUserTag(m.doc.Owner)
}

// Status returns the status of the model.
func (m *Model) Status() (status.StatusInfo, error) {
	st, closeState, err := m.getState()
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}
	defer closeState()
	status, err := getStatus(st, m.globalKey(), "model")
	if err != nil {
		return status, err
	}
	return status, nil
}

// SetStatus sets the status of the model.
func (m *Model) SetStatus(sInfo status.StatusInfo) error {
	st, closeState, err := m.getState()
	if err != nil {
		return errors.Trace(err)
	}
	defer closeState()
	if !status.ValidModelStatus(sInfo.Status) {
		return errors.Errorf("cannot set invalid status %q", sInfo.Status)
	}
	return setStatus(st, setStatusParams{
		badge:     "model",
		globalKey: m.globalKey(),
		status:    sInfo.Status,
		message:   sInfo.Message,
		rawData:   sInfo.Data,
		updated:   sInfo.Since,
	})
}

// Config returns the config for the model.
func (m *Model) Config() (*config.Config, error) {
	st, closeState, err := m.getState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closeState()
	return st.ModelConfig()
}

// ConfigValues returns the config values for the model.
func (m *Model) ConfigValues() (config.ConfigValues, error) {
	st, closeState, err := m.getState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closeState()
	return st.ModelConfigValues()
}

// UpdateLatestToolsVersion looks up for the latest available version of
// juju tools and updates environementDoc with it.
func (m *Model) UpdateLatestToolsVersion(ver version.Number) error {
	v := ver.String()
	// TODO(perrito666): I need to assert here that there isn't a newer
	// version in place.
	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"available-tools", v}}}},
	}}
	err := m.st.runTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}
	return m.Refresh()
}

// LatestToolsVersion returns the newest version found in the last
// check in the streams.
// Bear in mind that the check was performed filtering only
// new patches for the current major.minor. (major.minor.patch)
func (m *Model) LatestToolsVersion() version.Number {
	ver := m.doc.LatestAvailableTools
	if ver == "" {
		return version.Zero
	}
	v, err := version.Parse(ver)
	if err != nil {
		// This is being stored from a valid version but
		// in case this data would beacame corrupt It is not
		// worth to fail because of it.
		return version.Zero
	}
	return v
}

// globalKey returns the global database key for the model.
func (m *Model) globalKey() string {
	return modelGlobalKey
}

func (m *Model) Refresh() error {
	models, closer := m.st.getCollection(modelsC)
	defer closer()
	return m.refresh(models.FindId(m.UUID()))
}

func (m *Model) refresh(query mongo.Query) error {
	err := query.One(&m.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("model")
	}
	return err
}

// Users returns a slice of all users for this model.
func (m *Model) Users() ([]description.UserAccess, error) {
	if m.st.ModelUUID() != m.UUID() {
		return nil, errors.New("cannot lookup model users outside the current model")
	}
	coll, closer := m.st.getCollection(modelUsersC)
	defer closer()

	var userDocs []userAccessDoc
	err := coll.Find(nil).All(&userDocs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var modelUsers []description.UserAccess
	for _, doc := range userDocs {
		// check if the User belonging to this model user has
		// been deleted, in this case we should not return it.
		userTag := names.NewUserTag(doc.UserName)
		if userTag.IsLocal() {
			_, err := m.st.User(userTag)
			if errors.IsUserNotFound(err) {
				continue
			}
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		mu, err := NewModelUserAccess(m.st, doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelUsers = append(modelUsers, mu)
	}

	return modelUsers, nil
}

func (m *Model) isControllerModel() bool {
	return m.st.controllerModelTag.Id() == m.doc.UUID
}

// Destroy sets the models's lifecycle to Dying, preventing
// addition of services or machines to state. If called on
// an empty hosted model, the lifecycle will be advanced
// straight to Dead.
//
// If called on a controller model, and that controller is
// hosting any non-Dead models, this method will return an
// error satisfying IsHasHostedsError.
func (m *Model) Destroy() error {
	ensureNoHostedModels := false
	if m.isControllerModel() {
		ensureNoHostedModels = true
	}
	return m.destroy(ensureNoHostedModels)
}

// DestroyIncludingHosted sets the model's lifecycle to Dying, preventing
// addition of services or machines to state. If this model is a controller
// hosting other models, they will also be destroyed.
func (m *Model) DestroyIncludingHosted() error {
	ensureNoHostedModels := false
	return m.destroy(ensureNoHostedModels)
}

func (m *Model) destroy(ensureNoHostedModels bool) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to destroy model")

	st, closeState, err := m.getState()
	if err != nil {
		return errors.Trace(err)
	}
	defer closeState()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// On the first attempt, we assume memory state is recent
		// enough to try using...
		if attempt != 0 {
			// ...but on subsequent attempts, we read fresh environ
			// state from the DB. Note that we do *not* refresh `e`
			// itself, as detailed in doc/hacking-state.txt
			if m, err = st.Model(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops, err := m.destroyOps(ensureNoHostedModels, false)
		if err == errModelNotAlive {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}

		return ops, nil
	}

	return st.run(buildTxn)
}

// errModelNotAlive is a signal emitted from destroyOps to indicate
// that model destruction is already underway.
var errModelNotAlive = errors.New("model is no longer alive")

type hasHostedModelsError int

func (e hasHostedModelsError) Error() string {
	return fmt.Sprintf("hosting %d other models", e)
}

func IsHasHostedModelsError(err error) bool {
	_, ok := errors.Cause(err).(hasHostedModelsError)
	return ok
}

// destroyOps returns the txn operations necessary to begin model
// destruction, or an error indicating why it can't.
//
// If ensureNoHostedModels is true, then destroyOps will
// fail if there are any non-Dead hosted models
func (m *Model) destroyOps(ensureNoHostedModels, ensureEmpty bool) ([]txn.Op, error) {
	if m.Life() != Alive {
		return nil, errModelNotAlive
	}

	// Ensure we're using the model's state.
	st, closeState, err := m.getState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closeState()

	// Check if the model is empty. If it is, we can advance the model's
	// lifecycle state directly to Dead.
	checkEmptyErr := m.checkEmpty()
	isEmpty := checkEmptyErr == nil
	if ensureEmpty && !isEmpty {
		return nil, errors.Trace(checkEmptyErr)
	}

	modelUUID := m.UUID()
	nextLife := Dying
	var prereqOps []txn.Op
	if isEmpty {
		prereqOps = []txn.Op{{
			C:  modelEntityRefsC,
			Id: modelUUID,
			Assert: bson.D{
				{"machines", bson.D{{"$size", 0}}},
				{"applications", bson.D{{"$size", 0}}},
			},
		}}
		if !m.isControllerModel() {
			// The model is empty, and is not the controller
			// model, so we can move it straight to Dead.
			nextLife = Dead
		}
	}

	if ensureNoHostedModels {
		// Check for any Dying or alive but non-empty models. If there
		// are any, we return an error indicating that there are hosted
		// models.
		models, err := st.AllModels()
		if err != nil {
			return nil, errors.Trace(err)
		}
		var aliveEmpty, aliveNonEmpty, dying, dead int
		for _, model := range models {
			if model.UUID() == m.UUID() {
				// Ignore the controller model.
				continue
			}
			if model.Life() == Dead {
				// Dead hosted models don't affect
				// whether the controller can be
				// destroyed or not, but they are
				// still counted in the hosted models.
				dead++
				continue
			}
			// See if the model is empty, and if it is,
			// get the ops required to destroy it.
			ops, err := model.destroyOps(false, true)
			switch err {
			case errModelNotAlive:
				dying++
			case nil:
				prereqOps = append(prereqOps, ops...)
				aliveEmpty++
			default:
				aliveNonEmpty++
			}
		}
		if dying > 0 || aliveNonEmpty > 0 {
			// There are Dying, or Alive but non-empty models.
			// We cannot destroy the controller without first
			// destroying the models and waiting for them to
			// become Dead.
			return nil, errors.Trace(
				hasHostedModelsError(dying + aliveNonEmpty + aliveEmpty),
			)
		}
		// Ensure that the number of active models has not changed
		// between the query and when the transaction is applied.
		//
		// Note that we assert that each empty model that we intend
		// move to Dead is still Alive, so we're protected from an
		// ABA style problem where an empty model is concurrently
		// removed, and replaced with a non-empty model.
		prereqOps = append(prereqOps, assertHostedModelsOp(aliveEmpty+dead))
	}

	timeOfDying := nowToTheSecond()
	modelUpdateValues := bson.D{
		{"life", nextLife},
		{"time-of-dying", timeOfDying},
	}
	if nextLife == Dead {
		modelUpdateValues = append(modelUpdateValues, bson.DocElem{
			"time-of-death", timeOfDying,
		})
	}

	ops := []txn.Op{{
		C:      modelsC,
		Id:     modelUUID,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", modelUpdateValues}},
	}}

	// Because txn operations execute in order, and may encounter
	// arbitrarily long delays, we need to make sure every op
	// causes a state change that's still consistent; so we make
	// sure the cleanup ops are the last thing that will execute.
	if m.isControllerModel() {
		cleanupOp := st.newCleanupOp(cleanupModelsForDyingController, modelUUID)
		ops = append(ops, cleanupOp)
	}
	if !isEmpty {
		// We only need to destroy machines and models if the model is
		// non-empty. It wouldn't normally be harmful to enqueue the
		// cleanups otherwise, except for when we're destroying an empty
		// hosted model in the course of destroying the controller. In
		// that case we'll get errors if we try to enqueue hosted-model
		// cleanups, because the cleanups collection is non-global.
		cleanupMachinesOp := st.newCleanupOp(cleanupMachinesForDyingModel, modelUUID)
		cleanupServicesOp := st.newCleanupOp(cleanupServicesForDyingModel, modelUUID)
		ops = append(ops, cleanupMachinesOp, cleanupServicesOp)
	}
	return append(prereqOps, ops...), nil
}

// checkEmpty checks that the machine is empty of any entities that may
// require external resource cleanup. If the model is not empty, then
// an error will be returned.
func (m *Model) checkEmpty() error {
	st, closeState, err := m.getState()
	if err != nil {
		return errors.Trace(err)
	}
	defer closeState()

	modelEntityRefs, closer := st.getCollection(modelEntityRefsC)
	defer closer()

	var doc modelEntityRefsDoc
	if err := modelEntityRefs.FindId(m.UUID()).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return errors.NotFoundf("entity references doc for model %s", m.UUID())
		}
		return errors.Annotatef(err, "getting entity references for model %s", m.UUID())
	}
	if n := len(doc.Machines); n > 0 {
		return errors.Errorf("model not empty, found %d machine(s)", n)
	}
	if n := len(doc.Applications); n > 0 {
		return errors.Errorf("model not empty, found %d applications(s)", n)
	}
	return nil
}

func addModelMachineRefOp(st *State, machineId string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     st.ModelUUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$addToSet", bson.D{{"machines", machineId}}}},
	}
}

func removeModelMachineRefOp(st *State, machineId string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     st.ModelUUID(),
		Update: bson.D{{"$pull", bson.D{{"machines", machineId}}}},
	}
}

func addModelServiceRefOp(st *State, applicationname string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     st.ModelUUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$addToSet", bson.D{{"applications", applicationname}}}},
	}
}

func removeModelServiceRefOp(st *State, applicationname string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     st.ModelUUID(),
		Update: bson.D{{"$pull", bson.D{{"applications", applicationname}}}},
	}
}

// createModelOp returns the operation needed to create
// an model document with the given name and UUID.
func createModelOp(
	owner names.UserTag,
	name, uuid, controllerUUID, cloudName, cloudRegion string,
	cloudCredential names.CloudCredentialTag,
	migrationMode MigrationMode,
) txn.Op {
	doc := &modelDoc{
		UUID:            uuid,
		Name:            name,
		Life:            Alive,
		Owner:           owner.Canonical(),
		ControllerUUID:  controllerUUID,
		MigrationMode:   migrationMode,
		Cloud:           cloudName,
		CloudRegion:     cloudRegion,
		CloudCredential: cloudCredential.Canonical(),
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

func assertHostedModelsOp(n int) txn.Op {
	return txn.Op{
		C:      controllersC,
		Id:     hostedModelCountKey,
		Assert: bson.D{{"refcount", n}},
	}
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

func hostedModelCount(st *State) (int, error) {
	var doc hostedModelCountDoc
	controllers, closer := st.getCollection(controllersC)
	defer closer()

	if err := controllers.Find(bson.D{{"_id", hostedModelCountKey}}).One(&doc); err != nil {
		return 0, errors.Trace(err)
	}
	return doc.RefCount, nil
}

// createUniqueOwnerModelNameOp returns the operation needed to create
// an usermodelnameC document with the given owner and model name.
func createUniqueOwnerModelNameOp(owner names.UserTag, envName string) txn.Op {
	return txn.Op{
		C:      usermodelnameC,
		Id:     userModelNameIndex(owner.Canonical(), envName),
		Assert: txn.DocMissing,
		Insert: bson.M{},
	}
}

// assertAliveOp returns a txn.Op that asserts the model is alive.
func (m *Model) assertActiveOp() txn.Op {
	return assertModelActiveOp(m.UUID())
}

// getState returns the State for the model, and a function to close
// it when done. If m.st is the model-specific state, then it will
// be returned and the close function will be a no-op.
//
// TODO(axw) 2016-04-14 #1570269
// find a way to guarantee that every Model is associated with the
// appropriate State. The current work-around is too easy to get wrong.
func (m *Model) getState() (*State, func(), error) {
	if m.st.modelTag == m.ModelTag() {
		return m.st, func() {}, nil
	}
	st, err := m.st.ForModel(m.ModelTag())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	uuid := st.ModelUUID()
	return st, func() {
		if err := st.Close(); err != nil {
			logger.Errorf("closing temporary state for model %s", uuid)
		}
	}, nil
}

// assertModelActiveOp returns a txn.Op that asserts the given
// model UUID refers to an Alive model.
func assertModelActiveOp(modelUUID string) txn.Op {
	return txn.Op{
		C:      modelsC,
		Id:     modelUUID,
		Assert: append(isAliveDoc, bson.DocElem{"migration-mode", MigrationModeNone}),
	}
}

func checkModelActive(st *State) error {
	model, err := st.Model()
	if (err == nil && model.Life() != Alive) || errors.IsNotFound(err) {
		return errors.Errorf("model %q is no longer alive", model.Name())
	} else if err != nil {
		return errors.Annotate(err, "unable to read model")
	} else if mode := model.MigrationMode(); mode != MigrationModeNone {
		return errors.Errorf("model %q is being migrated", model.Name())
	}
	return nil
}
