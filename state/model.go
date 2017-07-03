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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/permission"
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
	// globalState is a State that is only safe for accessing non-model
	// specific data, e.g. (non-model) users, models.
	globalState *State
	doc         modelDoc
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

	// SLA is the current support level of the model.
	SLA slaDoc `bson:"sla"`

	// MeterStatus is the current meter status of the model.
	MeterStatus modelMeterStatusdoc `bson:"meter-status"`
}

// slaLevel enumerates the support levels available to a model.
type slaLevel string

const (
	slaNone        = slaLevel("")
	SLAUnsupported = slaLevel("unsupported")
	SLAEssential   = slaLevel("essential")
	SLAStandard    = slaLevel("standard")
	SLAAdvanced    = slaLevel("advanced")
)

// String implements fmt.Stringer returning the string representation of an
// SLALevel.
func (l slaLevel) String() string {
	if l == slaNone {
		l = SLAUnsupported
	}
	return string(l)
}

// newSLALevel returns a new SLA level from a string representation.
func newSLALevel(level string) (slaLevel, error) {
	l := slaLevel(level)
	if l == slaNone {
		l = SLAUnsupported
	}
	switch l {
	case SLAUnsupported, SLAEssential, SLAStandard, SLAAdvanced:
		return l, nil
	}
	return l, errors.NotValidf("SLA level %q", level)
}

// slaDoc represents the state of the SLA on the model.
type slaDoc struct {
	// Level is the current support level set on the model.
	Level slaLevel `bson:"level"`

	// Owner is the SLA owner of the model.
	Owner string `bson:"owner,omitempty"`

	// Credentials authenticates the support level setting.
	Credentials []byte `bson:"credentials"`
}

type modelMeterStatusdoc struct {
	Code string `bson:"code"`
	Info string `bson:"info"`
}

// modelEntityRefsDoc records references to the top-level entities
// in the model.
// (anastasiamac 2017-04-10) This is also used to determine if a model can be destroyed.
// Consequently, any changes, especially additions of entities, here,
// would need to be reflected, at least, in Model.checkEmpty(...) as well as
// Model.destroyOps(...)
type modelEntityRefsDoc struct {
	UUID string `bson:"_id"`

	// Machines contains the names of the top-level machines in the model.
	Machines []string `bson:"machines"`

	// Applicatons contains the names of the applications in the model.
	Applications []string `bson:"applications"`

	// Volumes contains the IDs of the volumes in the model.
	Volumes []string `bson:"volumes"`

	// Filesystems contains the IDs of the filesystems in the model.
	Filesystems []string `bson:"filesystems"`
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
	return st.GetModel(ssinfo.ModelTag)
}

// Model returns the model entity.
func (st *State) Model() (*Model, error) {
	return st.GetModel(st.modelTag)
}

// GetModel looks for the model identified by the uuid passed in.
func (st *State) GetModel(tag names.ModelTag) (*Model, error) {
	models, closer := st.db().GetCollection(modelsC)
	defer closer()

	model := &Model{
		globalState: st,
	}
	if err := model.refresh(models.FindId(tag.Id())); err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

// AllModels returns all the models in the system.
func (st *State) AllModels() ([]*Model, error) {
	models, closer := st.db().GetCollection(modelsC)
	defer closer()

	var modelDocs []modelDoc
	err := models.Find(nil).Sort("name", "owner").All(&modelDocs)
	if err != nil {
		return nil, err
	}

	result := make([]*Model, len(modelDocs))
	for i, doc := range modelDocs {
		result[i] = &Model{
			globalState: st,
			doc:         doc,
		}
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
	assertCloudRegionOp, err := validateCloudRegion(controllerCloud, args.CloudRegion)
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
		controllerCloud, cloudCredentials, args.CloudCredential,
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
	newSt, err := newState(
		names.NewModelTag(uuid),
		controllerInfo.ModelTag,
		session,
		st.mongoInfo,
		st.newPolicy,
		st.clock(),
		st.runTransactionObserver,
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

	modelOps, modelStatusDoc, err := newSt.modelSetupOps(st.controllerTag.Id(), args, nil)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create new model")
	}

	prereqOps := []txn.Op{
		assertCloudRegionOp,
		assertCloudCredentialOp,
	}
	ops := append(prereqOps, modelOps...)
	err = newSt.db().RunTransaction(ops)
	if err == txn.ErrAborted {

		// We have a  unique key restriction on the "owner" and "name" fields,
		// which will cause the insert to fail if there is another record with
		// the same "owner" and "name" in the collection. If the txn is
		// aborted, check if it is due to the unique key restriction.
		name := args.Config.Name()
		models, closer := st.db().GetCollection(modelsC)
		defer closer()
		envCount, countErr := models.Find(bson.D{
			{"owner", owner.Id()},
			{"name", name}},
		).Count()
		if countErr != nil {
			err = errors.Trace(countErr)
		} else if envCount > 0 {
			err = errors.AlreadyExistsf("model %q for %s", name, owner.Id())
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
	if args.MigrationMode != MigrationModeImporting {
		probablyUpdateStatusHistory(newSt.db(), modelGlobalKey, modelStatusDoc)
	}

	_, err = newSt.SetUserAccess(newModel.Owner(), newModel.ModelTag(), permission.AdminAccess)
	if err != nil {
		return nil, nil, errors.Annotate(err, "granting admin permission to the owner")
	}

	if err := InitDbLogs(session, uuid); err != nil {
		return nil, nil, errors.Annotate(err, "initialising model logs collection")
	}
	return newModel, newSt, nil
}

// validateCloudRegion validates the given region name against the
// provided Cloud definition, and returns a txn.Op to include in a
// transaction to assert the same.
func validateCloudRegion(cloud jujucloud.Cloud, regionName string) (txn.Op, error) {
	// Ensure that the cloud region is valid, or if one is not specified,
	// that the cloud does not support regions.
	assertCloudRegionOp := txn.Op{
		C:  cloudsC,
		Id: cloud.Name,
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
	cloudCredentials map[string]jujucloud.Credential,
	cloudCredential names.CloudCredentialTag,
) (txn.Op, error) {
	if cloudCredential != (names.CloudCredentialTag{}) {
		if cloudCredential.Cloud().Id() != cloud.Name {
			return txn.Op{}, errors.NotValidf("credential %q", cloudCredential.Id())
		}
		var found bool
		for tag := range cloudCredentials {
			if tag == cloudCredential.Id() {
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
		Id:     cloud.Name,
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
	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.doc.UUID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"migration-mode", mode}}}},
	}}
	if err := m.globalState.db().RunTransaction(ops); err != nil {
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
	db, closer := m.modelDatabase()
	defer closer()
	status, err := getStatus(db, m.globalKey(), "model")
	if err != nil {
		return status, err
	}
	return status, nil
}

// SetStatus sets the status of the model.
func (m *Model) SetStatus(sInfo status.StatusInfo) error {
	if !status.ValidModelStatus(sInfo.Status) {
		return errors.Errorf("cannot set invalid status %q", sInfo.Status)
	}
	db, closer := m.modelDatabase()
	defer closer()
	return setStatus(db, setStatusParams{
		badge:     "model",
		globalKey: m.globalKey(),
		status:    sInfo.Status,
		message:   sInfo.Message,
		rawData:   sInfo.Data,
		updated:   timeOrNow(sInfo.Since, m.globalState.clock()),
	})
}

// StatusHistory returns a slice of at most filter.Size StatusInfo items
// or items as old as filter.Date or items newer than now - filter.Delta time
// representing past statuses for this application.
func (m *Model) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	db, closer := m.modelDatabase()
	defer closer()
	args := &statusHistoryArgs{
		db:        db,
		globalKey: m.globalKey(),
		filter:    filter,
	}
	return statusHistory(args)
}

// Config returns the config for the model.
func (m *Model) Config() (*config.Config, error) {
	db, closer := m.modelDatabase()
	defer closer()
	return getModelConfig(db)
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
	err := m.globalState.db().RunTransaction(ops)
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

// SLALevel returns the SLA level as a string.
func (m *Model) SLALevel() string {
	return m.doc.SLA.Level.String()
}

// SLAOwner returns the SLA owner as a string. Note that this may differ from
// the model owner.
func (m *Model) SLAOwner() string {
	return m.doc.SLA.Owner
}

// SLACredential returns the SLA credential.
func (m *Model) SLACredential() []byte {
	return m.doc.SLA.Credentials
}

// SetSLA sets the SLA on the model.
func (m *Model) SetSLA(level, owner string, credentials []byte) error {
	l, err := newSLALevel(level)
	if err != nil {
		return errors.Trace(err)
	}
	ops := []txn.Op{{
		C:  modelsC,
		Id: m.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"sla", slaDoc{
			Level:       l,
			Owner:       owner,
			Credentials: credentials,
		}}}}},
	}}
	err = m.globalState.db().RunTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}
	return m.Refresh()
}

// SetMeterStatus sets the current meter status for this model.
func (m *Model) SetMeterStatus(status, info string) error {
	if _, err := isValidMeterStatusCode(status); err != nil {
		return errors.Trace(err)
	}
	ops := []txn.Op{{
		C:  modelsC,
		Id: m.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"meter-status", modelMeterStatusdoc{
			Code: status,
			Info: info,
		}}}}},
	}}
	err := m.globalState.db().RunTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}
	return m.Refresh()
}

// MeterStatus returns the current meter status for this model.
func (m *Model) MeterStatus() MeterStatus {
	ms := m.doc.MeterStatus
	return MeterStatus{
		Code: MeterStatusFromString(ms.Code),
		Info: ms.Info,
	}
}

// globalKey returns the global database key for the model.
func (m *Model) globalKey() string {
	return modelGlobalKey
}

func (m *Model) Refresh() error {
	models, closer := m.globalState.db().GetCollection(modelsC)
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
func (m *Model) Users() ([]permission.UserAccess, error) {
	db, dbCloser := m.modelDatabase()
	defer dbCloser()
	coll, closer := db.GetCollection(modelUsersC)
	defer closer()

	var userDocs []userAccessDoc
	err := coll.Find(nil).All(&userDocs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var modelUsers []permission.UserAccess
	for _, doc := range userDocs {
		// check if the User belonging to this model user has
		// been deleted, in this case we should not return it.
		userTag := names.NewUserTag(doc.UserName)
		if userTag.IsLocal() {
			_, err := m.globalState.User(userTag)
			if err != nil {
				if _, ok := err.(DeletedUserError); !ok {
					// We ignore deleted users for now. So if it is not a
					// DeletedUserError we return the error.
					return nil, errors.Trace(err)
				}
				continue
			}
		}
		mu, err := NewModelUserAccess(m.globalState, doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelUsers = append(modelUsers, mu)
	}

	return modelUsers, nil
}

func (m *Model) isControllerModel() bool {
	return m.globalState.controllerModelTag.Id() == m.doc.UUID
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

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// On the first attempt, we assume memory state is recent
		// enough to try using...
		if attempt != 0 {
			// ...but on subsequent attempts, we read fresh environ
			// state from the DB. Note that we do *not* refresh the
			// original `m` itself, as detailed in doc/hacking-state.txt
			if attempt == 1 {
				mCopy := *m
				m = &mCopy
			}
			if err := m.Refresh(); err != nil {
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

	return m.globalState.db().RunFor(m.UUID(), buildTxn)
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
				{"volumes", bson.D{{"$size", 0}}},
				{"filesystems", bson.D{{"$size", 0}}},
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
		models, err := m.globalState.AllModels()
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

	timeOfDying := m.globalState.nowToTheSecond()
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
		cleanupOp := newCleanupOp(cleanupModelsForDyingController, modelUUID)
		ops = append(ops, cleanupOp)
	}
	if !isEmpty {
		// We only need to destroy resources if the model is non-empty.
		// It wouldn't normally be harmful to enqueue the cleanups
		// otherwise, except for when we're destroying an empty
		// hosted model in the course of destroying the controller. In
		// that case we'll get errors if we try to enqueue hosted-model
		// cleanups, because the cleanups collection is non-global.
		cleanupMachinesOp := newCleanupOp(cleanupMachinesForDyingModel, modelUUID)
		cleanupApplicationsOp := newCleanupOp(cleanupApplicationsForDyingModel, modelUUID)
		cleanupStorageOp := newCleanupOp(cleanupStorageForDyingModel, modelUUID)
		ops = append(ops,
			cleanupMachinesOp,
			cleanupApplicationsOp,
			cleanupStorageOp,
		)
	}
	return append(prereqOps, ops...), nil
}

// checkEmpty checks that the machine is empty of any entities that may
// require external resource cleanup. If the model is not empty, then
// an error will be returned.
func (m *Model) checkEmpty() error {
	modelEntityRefs, closer := m.globalState.db().GetCollection(modelEntityRefsC)
	defer closer()

	var doc modelEntityRefsDoc
	if err := modelEntityRefs.FindId(m.UUID()).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return errors.NotFoundf("entity references doc for model %s", m.UUID())
		}
		return errors.Annotatef(err, "getting entity references for model %s", m.UUID())
	}
	// These errors could be potentially swallowed as we re-try to destroy model.
	// Let's, at least, log them for observation.
	if n := len(doc.Machines); n > 0 {
		logger.Infof("model is still not empty, has machines: %v", doc.Machines)
		return errors.Errorf("model not empty, found %d machine(s)", n)
	}
	if n := len(doc.Applications); n > 0 {
		logger.Infof("model is still not empty, has applications: %v", doc.Applications)
		return errors.Errorf("model not empty, found %d application(s)", n)
	}
	if n := len(doc.Volumes); n > 0 {
		logger.Infof("model is still not empty, has volumes: %v", doc.Volumes)
		return errors.Errorf("model not empty, found %d volume(s)", n)
	}
	if n := len(doc.Filesystems); n > 0 {
		logger.Infof("model is still not empty, has file systems: %v", doc.Filesystems)
		return errors.Errorf("model not empty, found %d filesystem(s)", n)
	}
	return nil
}

func addModelMachineRefOp(mb modelBackend, machineId string) txn.Op {
	return addModelEntityRefOp(mb, "machines", machineId)
}

func removeModelMachineRefOp(mb modelBackend, machineId string) txn.Op {
	return removeModelEntityRefOp(mb, "machines", machineId)
}

func addModelApplicationRefOp(mb modelBackend, applicationname string) txn.Op {
	return addModelEntityRefOp(mb, "applications", applicationname)
}

func removeModelApplicationRefOp(mb modelBackend, applicationname string) txn.Op {
	return removeModelEntityRefOp(mb, "applications", applicationname)
}

func addModelVolumeRefOp(mb modelBackend, volumeId string) txn.Op {
	return addModelEntityRefOp(mb, "volumes", volumeId)
}

func removeModelVolumeRefOp(mb modelBackend, volumeId string) txn.Op {
	return removeModelEntityRefOp(mb, "volumes", volumeId)
}

func addModelFilesystemRefOp(mb modelBackend, filesystemId string) txn.Op {
	return addModelEntityRefOp(mb, "filesystems", filesystemId)
}

func removeModelFilesystemRefOp(mb modelBackend, filesystemId string) txn.Op {
	return removeModelEntityRefOp(mb, "filesystems", filesystemId)
}

func addModelEntityRefOp(mb modelBackend, entityField, entityId string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     mb.modelUUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$addToSet", bson.D{{entityField, entityId}}}},
	}
}

func removeModelEntityRefOp(mb modelBackend, entityField, entityId string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     mb.modelUUID(),
		Update: bson.D{{"$pull", bson.D{{entityField, entityId}}}},
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
		Owner:           owner.Id(),
		ControllerUUID:  controllerUUID,
		MigrationMode:   migrationMode,
		Cloud:           cloudName,
		CloudRegion:     cloudRegion,
		CloudCredential: cloudCredential.Id(),
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
	controllers, closer := st.db().GetCollection(controllersC)
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
		Id:     userModelNameIndex(owner.Id(), envName),
		Assert: txn.DocMissing,
		Insert: bson.M{},
	}
}

// assertAliveOp returns a txn.Op that asserts the model is alive.
func (m *Model) assertActiveOp() txn.Op {
	return assertModelActiveOp(m.UUID())
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

// modelDatabase returns a Database scoped to the model's UUID,
// and a function that will close the database when called.
func (m *Model) modelDatabase() (Database, func()) {
	return m.globalState.db().CopyForModel(m.UUID())
}
