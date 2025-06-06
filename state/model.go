// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jujutxn "github.com/juju/txn/v3"
	jujuutils "github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo/utils"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/storage"
)

// modelGlobalKey is the key for the model, its
// settings and constraints.
const modelGlobalKey = "e"

// modelKey will create the key for a given model using the modelGlobalKey.
func modelKey(modelUUID string) string {
	return fmt.Sprintf("%s#%s", modelGlobalKey, modelUUID)
}

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

// MigrationMode specifies where the Model is with respect to migration.
type MigrationMode string

const (
	// MigrationModeNone is the default mode for a model and reflects
	// that it isn't involved with a model migration.
	MigrationModeNone = MigrationMode("")

	// MigrationModeExporting reflects a model that is in the process of being
	// exported from one controller to another.
	MigrationModeExporting = MigrationMode("exporting")

	// MigrationModeImporting reflects a model that is being imported into a
	// controller, but is not yet fully active.
	MigrationModeImporting = MigrationMode("importing")
)

// Model represents the state of a model.
type Model struct {
	st  *State
	doc modelDoc
}

// modelDoc represents the internal state of the model in MongoDB.
type modelDoc struct {
	UUID           string        `bson:"_id"`
	Name           string        `bson:"name"`
	Type           ModelType     `bson:"type"`
	Life           Life          `bson:"life"`
	Owner          string        `bson:"owner"`
	ControllerUUID string        `bson:"controller-uuid"`
	MigrationMode  MigrationMode `bson:"migration-mode"`

	// EnvironVersion is the version of the Environ. As providers
	// evolve, cloud resource representations may change; the environ
	// version tracks the current version of that.
	EnvironVersion int `bson:"environ-version"`

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

	// EnvironVersion is the initial version of the Environ for the model.
	EnvironVersion int

	// PasswordHash is used by the caas model operator.
	PasswordHash string
}

// Validate validates the ModelArgs.
func (m ModelArgs) Validate() error {
	if m.Type == modelTypeNone {
		return errors.NotValidf("empty Type")
	}
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

	// Ensure that the cloud region is valid, or if one is not specified,
	// that the cloud does not support regions.
	modelCloud, err := st.Cloud(args.CloudName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	var prereqOps []txn.Op

	// If no region specified and the cloud only has one, default to that.
	if args.CloudRegion == "" && len(modelCloud.Regions) == 1 {
		args.CloudRegion = modelCloud.Regions[0].Name
	}
	assertCloudRegionOp, err := validateCloudRegion(modelCloud, args.CloudRegion)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	prereqOps = append(prereqOps, assertCloudRegionOp)

	// Ensure that the cloud credential is valid, or if one is not
	// specified, that the cloud supports the "empty" authentication
	// type.
	owner := args.Owner
	cloudCredentials, err := st.CloudCredentials(owner, args.CloudName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	assertCloudCredentialOp, err := validateCloudCredential(
		modelCloud, cloudCredentials, args.CloudCredential,
	)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	prereqOps = append(prereqOps, assertCloudCredentialOp)

	uuid := args.Config.UUID()
	session := st.session.Copy()
	newSt, err := newState(
		st.controllerTag,
		names.NewModelTag(uuid),
		controllerInfo.ModelTag,
		session,
		st.newPolicy,
		st.clock(),
		st.runTransactionObserver,
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

	modelOps, modelStatusDoc, err := newSt.modelSetupOps(st.controllerTag.Id(), args, nil)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create new model")
	}

	ops := append(prereqOps, modelOps...)

	// Increment the model count for the cloud to which this model belongs.
	incCloudRefOp, err := incCloudModelRefOp(st, args.CloudName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	ops = append(ops, incCloudRefOp)

	err = newSt.db().RunTransaction(ops)
	if err == txn.ErrAborted {
		// Check that the cloud exists.
		// TODO(wallyworld) - this can't yet be tested since we check that the
		// model cloud is the same as the controller cloud, and hooks can't be
		// used because a new state is created.
		if _, err := newSt.Cloud(args.CloudName); err != nil {
			return nil, nil, errors.Trace(err)
		}

		// We have a  unique key restriction on the "owner" and "name" fields,
		// which will cause the insert to fail if there is another record with
		// the same "owner" and "name" in the collection. If the txn is
		// aborted, check if it is due to the unique key restriction.
		name := args.Config.Name()
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
	if args.MigrationMode != MigrationModeImporting {
		_, _ = probablyUpdateStatusHistory(newSt.db(), modelGlobalKey, modelStatusDoc)
	}

	_, err = newSt.SetUserAccess(newModel.Owner(), newModel.ModelTag(), permission.AdminAccess)
	if err != nil {
		return nil, nil, errors.Annotate(err, "granting admin permission to the owner")
	}

	config, err := st.ControllerConfig()
	if err != nil {
		return nil, nil, errors.Annotate(err, "unable to get controller config")
	}

	if err := InitDbLogsForModel(session, uuid, config.ModelLogsSizeMB()); err != nil {
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
			{"regions." + utils.EscapeKey(region.Name), bson.D{{"$exists", true}}},
		}
	} else {
		if len(cloud.Regions) > 0 {
			return txn.Op{}, errors.NotValidf("missing cloud region")
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
	cloudCredentials map[string]Credential,
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

// SetPassword sets the password for the model's agent.
func (m *Model) SetPassword(password string) error {
	if len(password) < jujuutils.MinAgentPasswordLength {
		return fmt.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	passwordHash := jujuutils.AgentPasswordHash(password)
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

// PasswordHash returns the password hash set on the model document
func (m *Model) PasswordHash() string {
	return m.doc.PasswordHash
}

// String returns the model name.
func (m *Model) String() string {
	return m.doc.Name
}

// PasswordValid returns whether the given password is valid
// for the given application.
func (m *Model) PasswordValid(password string) bool {
	agentHash := jujuutils.AgentPasswordHash(password)
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

// Cloud returns the cloud to which the model is deployed.
func (m *Model) Cloud() (jujucloud.Cloud, error) {
	return m.st.Cloud(m.CloudName())
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

// CloudCredential returns the cloud credential used for managing the
// model's cloud resources, and a boolean indicating whether a credential is set.
func (m *Model) CloudCredential() (Credential, bool, error) {
	tag, ok := m.CloudCredentialTag()
	if !ok {
		return Credential{}, false, nil
	}
	cred, err := m.st.CloudCredential(tag)
	return cred, true, err
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
	if err := m.st.db().RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return m.Refresh()
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

func modelStatusInvalidCredential(reason string) status.StatusInfo {
	return status.StatusInfo{
		Status:  status.Suspended,
		Message: "suspended since cloud credential is not valid",
		Data: map[string]interface{}{
			"reason": reason,
		},
	}
}

// Status returns the status of the model.
func (m *Model) Status() (status.StatusInfo, error) {
	// If model credential is invalid, model is suspended.
	if credentialTag, hasCredential := m.CloudCredentialTag(); hasCredential {
		credential, err := m.st.CloudCredential(credentialTag)
		if err != nil {
			return status.StatusInfo{}, errors.Annotatef(err, "could not get model credential %v", credentialTag.Id())
		}
		if !credential.IsValid() {
			return modelStatusInvalidCredential(credential.InvalidReason), nil
		}
	}

	modelStatus, err := getStatus(m.st.db(), m.globalKey(), "model")
	if err != nil {
		return modelStatus, err
	}
	return modelStatus, nil
}

// localID returns the local id value by stripping off the model uuid prefix
// if it is there.
func (m *Model) localID(ID string) string {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != m.doc.UUID {
		return ID
	}
	return localID
}

// SetStatus sets the status of the model.
func (m *Model) SetStatus(sInfo status.StatusInfo) error {
	if !status.ValidModelStatus(sInfo.Status) {
		return errors.Errorf("cannot set invalid status %q", sInfo.Status)
	}
	return setStatus(m.st.db(), setStatusParams{
		badge:     "model",
		globalKey: m.globalKey(),
		status:    sInfo.Status,
		message:   sInfo.Message,
		rawData:   sInfo.Data,
		updated:   timeOrNow(sInfo.Since, m.st.clock()),
	})
}

// StatusHistory returns a slice of at most filter.Size StatusInfo items
// or items as old as filter.Date or items newer than now - filter.Delta time
// representing past statuses for this application.
func (m *Model) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        m.st.db(),
		globalKey: m.globalKey(),
		filter:    filter,
		clock:     m.st.clock(),
	}
	return statusHistory(args)
}

// Config returns the config for the model.
func (m *Model) Config() (*config.Config, error) {
	return getModelConfig(m.st.db(), m.UUID())
}

// UpdateLatestToolsVersion looks up for the latest available version of
// juju tools and updates modelDoc with it.
func (m *Model) UpdateLatestToolsVersion(ver version.Number) error {
	v := ver.String()
	// TODO(perrito666): I need to assert here that there isn't a newer
	// version in place.
	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"available-tools", v}}}},
	}}
	err := m.st.db().RunTransaction(ops)
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
		// This is being stored from a valid version,
		// but we don't fail should it become corrupt.
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
	err = m.st.db().RunTransaction(ops)
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
	err := m.st.db().RunTransaction(ops)
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

// EnvironVersion is the version of the model's environ -- the related
// cloud provider resources. The environ version is used by the controller
// to identify environ/provider upgrade steps to run for a model's environ
// after the controller is upgraded, or the model is migrated to another
// controller.
func (m *Model) EnvironVersion() int {
	return m.doc.EnvironVersion
}

// SetEnvironVersion sets the model's current environ version. The value
// must be monotonically increasing.
func (m *Model) SetEnvironVersion(v int) error {
	mOrig := m
	mCopy := *m
	m = &mCopy // copy so we can refresh without affecting the original m
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if v < m.doc.EnvironVersion {
			return nil, errors.Errorf(
				"cannot set environ version to %v, which is less than the current version %v",
				v, m.doc.EnvironVersion,
			)
		}
		if v == m.doc.EnvironVersion {
			return nil, jujutxn.ErrNoOperations
		}
		return []txn.Op{{
			C:      modelsC,
			Id:     m.doc.UUID,
			Assert: bson.D{{"environ-version", m.doc.EnvironVersion}},
			Update: bson.D{{"$set", bson.D{{"environ-version", v}}}},
		}}, nil
	}
	if err := m.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	mOrig.doc.EnvironVersion = v
	return nil
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

// AllUnits returns all units for a model, for all applications.
func (m *Model) AllUnits() ([]*Unit, error) {
	coll, closer := m.st.db().GetCollection(unitsC)
	defer closer()

	docs := []unitDoc{}
	err := coll.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all units for model")
	}
	var units []*Unit
	for i := range docs {
		units = append(units, newUnit(m.st, m.Type(), &docs[i]))
	}
	return units, nil
}

// ModelMetrics contains metrics to be collected and sent to
// Charmhub via the charm revision updater for a model.
type ModelMetrics struct {
	ApplicationCount string
	MachineCount     string
	UnitCount        string
	CloudName        string
	CloudRegion      string
	Provider         string
	UUID             string
	ControllerUUID   string
}

// Metrics returns model level metrics to be collected and sent to
// Charmhub via the charm revision updater.
func (m *Model) Metrics() (ModelMetrics, error) {
	cloud, err := m.Cloud()
	if err != nil {
		return ModelMetrics{}, errors.Trace(err)
	}

	appCnt, err := m.applicationCount()
	if err != nil {
		return ModelMetrics{}, errors.Trace(err)
	}
	machCnt, err := m.machineCount()
	if err != nil {
		return ModelMetrics{}, errors.Trace(err)
	}
	unitCnt, err := m.unitCount()
	if err != nil {
		return ModelMetrics{}, errors.Trace(err)
	}

	return ModelMetrics{
		ApplicationCount: strconv.Itoa(appCnt),
		MachineCount:     strconv.Itoa(machCnt),
		UnitCount:        strconv.Itoa(unitCnt),
		CloudName:        cloud.Name,
		CloudRegion:      m.CloudRegion(),
		Provider:         cloud.Type,
		UUID:             m.UUID(),
		ControllerUUID:   m.doc.ControllerUUID,
	}, nil
}

func (m *Model) applicationCount() (int, error) {
	coll, closer := m.st.db().GetCollection(applicationsC)
	defer closer()
	return coll.Find(isAliveDoc).Count()
}

func (m *Model) machineCount() (int, error) {
	coll, closer := m.st.db().GetCollection(machinesC)
	defer closer()
	return coll.Find(isAliveDoc).Count()
}

func (m *Model) unitCount() (int, error) {
	coll, closer := m.st.db().GetCollection(unitsC)
	defer closer()
	return coll.Find(isAliveDoc).Count()
}

// AllEndpointBindings returns all endpoint->space bindings
// keyed by application name.
func (m *Model) AllEndpointBindings() (map[string]*Bindings, error) {
	endpointBindings, closer := m.st.db().GetCollection(endpointBindingsC)
	defer closer()

	var docs []endpointBindingsDoc
	err := endpointBindings.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get endpoint bindings")
	}

	appEndpointBindings := make(map[string]*Bindings, 0)
	for _, doc := range docs {
		var applicationName string
		applicationKey := m.localID(doc.DocID)
		if strings.HasPrefix(applicationKey, "a#") {
			applicationName = applicationKey[2:]
		} else {
			return nil, errors.NotValidf("application key %v", applicationKey)
		}

		bindings, err := NewBindings(m.st, doc.Bindings)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot make bindings")
		}
		appEndpointBindings[applicationName] = bindings
	}

	return appEndpointBindings, nil
}

// AllEndpointBindingsSpaceNames returns a set of spaces names for all the
// endpoint bindings.
func (st *State) AllEndpointBindingsSpaceNames() (set.Strings, error) {
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	allEndpointBindings, err := model.AllEndpointBindings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	lookup, err := st.AllSpaceInfos()
	if err != nil {
		return nil, errors.Trace(err)
	}

	allEndpointBindingsSpaces := set.NewStrings()
	for _, bindings := range allEndpointBindings {
		bindingSpaceNames, err := bindings.MapWithSpaceNames(lookup)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, spaceName := range bindingSpaceNames {
			allEndpointBindingsSpaces.Add(spaceName)
		}
	}

	return allEndpointBindingsSpaces, nil
}

// Users returns a slice of all users for this model.
func (m *Model) Users() ([]permission.UserAccess, error) {
	coll, closer := m.st.db().GetCollection(modelUsersC)
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
			_, err := m.st.User(userTag)
			if err != nil {
				if !IsDeletedUserError(err) {
					// We ignore deleted users for now. So if it is not a
					// DeletedUserError we return the error.
					return nil, errors.Trace(err)
				}
				continue
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

		ops, err := m.destroyOps(args, false, false)
		if err == errModelNotAlive {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}
	return m.st.db().Run(buildTxn)
}

// errModelNotAlive is a signal emitted from destroyOps to indicate
// that model destruction is already underway.
var errModelNotAlive = errors.New("model is no longer alive")

// destroyOps returns the txn operations necessary to begin model
// destruction, or an error indicating why it can't.
//
// If ensureEmpty is true, then destroyOps will return an error
// if the model is non-empty.
//
// If destroyingController is true, then destroyOps will progress
// empty models to Dead, but otherwise will return only non-mutating
// ops to assert the current state of the model, and will leave it
// to the "models" cleanup to destroy the model.
func (m *Model) destroyOps(
	args DestroyModelParams,
	ensureEmpty bool,
	destroyingController bool,
) ([]txn.Op, error) {
	modelUUID := m.UUID()
	force := args.Force != nil && *args.Force
	if m.Life() != Alive && !force {
		currentTimeout := m.DestroyTimeout()
		if !destroyingController && ((currentTimeout == nil && args.Timeout != nil) ||
			(currentTimeout != nil && args.Timeout != nil && *currentTimeout != *args.Timeout)) {
			var ops []txn.Op
			modelOp := txn.Op{
				C:      modelsC,
				Id:     modelUUID,
				Assert: bson.D{{"life", m.Life()}},
			}
			modelOp.Update = bson.D{{
				"$set",
				bson.D{
					{"destroy-timeout", args.Timeout},
				},
			}}
			ops = append(ops, modelOp)
			return ops, nil
		}
		return nil, errModelNotAlive
	}

	// Check if the model is empty. If it is, we can advance the model's
	// lifecycle state directly to Dead.
	modelEntityRefs, err := m.getEntityRefs()
	if err != nil {
		if !force {
			return nil, errors.Annotatef(err, "getting model %v entity refs", m.UUID())
		}
		logger.Warningf("getting model %v entity refs: %v", m.UUID(), err)
	}
	isEmpty := true
	nextLife := Dying

	prereqOps, err := checkModelEntityRefsEmpty(modelEntityRefs)
	if err != nil {
		if ensureEmpty {
			return nil, errors.Trace(err)
		}
		isEmpty = false
		prereqOps = nil
		if args.DestroyStorage == nil {
			// The model is non-empty, and the user has not specified
			// whether storage should be destroyed or released. Make
			// sure there are no filesystems or volumes in the model.
			storageOps, err := checkModelEntityRefsNoPersistentStorage(m.st.db(), modelEntityRefs)
			if err != nil {
				return nil, errors.Trace(err)
			}
			prereqOps = storageOps
		} else if !*args.DestroyStorage {
			// The model is non-empty, and the user has specified that
			// storage should be released. Make sure the storage is
			// all releasable.
			sb, err := NewStorageBackend(m.st)
			if err != nil {
				return nil, errors.Trace(err)
			}
			force := args.Force != nil && *args.Force
			storageOps, err := checkModelEntityRefsAllReleasableStorage(sb, modelEntityRefs, force)
			if err != nil {
				return nil, errors.Trace(err)
			}
			prereqOps = storageOps
		}
	}

	if m.IsControllerModel() && (!args.DestroyHostedModels || args.DestroyStorage == nil || !*args.DestroyStorage) {
		// This is the controller model, and we've not been instructed
		// to destroy hosted models, or we've not been instructed to
		// destroy storage.
		//
		// Check for any Dying or alive but non-empty models. If there
		// are any and we have not been instructed to destroy them, we
		// return an error indicating that there are hosted models.
		//
		// The same logic applies even if the destruction is forced since really
		// we are talking about a controller model here and any forceful destruction of the controller
		// needs to be dealt through destroy-controller code path.

		modelUUIDs, err := m.st.AllModelUUIDsIncludingDead()
		if err != nil {
			return nil, errors.Trace(err)
		}
		var aliveEmpty, aliveNonEmpty, dying, dead int
		for _, aModelUUID := range modelUUIDs {
			if aModelUUID == m.UUID() {
				// Ignore the controller model.
				continue
			}
			newSt, err := m.st.newStateNoWorkers(aModelUUID)
			if err != nil {
				// This model could have been removed.
				if errors.IsNotFound(err) {
					continue
				}
				// TODO (force 2019-4-24) we should not break out here but
				// continue with other models.
				return nil, errors.Trace(err)
			}
			defer newSt.Close()

			model, err := newSt.Model()
			if err != nil {
				// TODO (force 2019-4-24) we should not break out here but
				// continue with other models.
				return nil, errors.Trace(err)
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
			// get the ops required to ensure it can be
			// destroyed, but without effecting the
			// destruction. The destruction is carried
			// out by the cleanup.
			ops, err := model.destroyOps(args, !args.DestroyHostedModels, true)
			switch err {
			case errModelNotAlive:
				// this empty hosted model is still dying.
				dying++
			case nil:
				prereqOps = append(prereqOps, ops...)
				aliveEmpty++
			default:
				if !errors.Is(err, stateerrors.ModelNotEmptyError) {
					// TODO (force 2019-4-24) we should not break out here but
					// continue with other models.
					return nil, errors.Trace(err)
				}
				aliveNonEmpty++
			}
		}
		if !args.DestroyHostedModels && (dying > 0 || aliveNonEmpty > 0) {
			// There are Dying, or Alive but non-empty models.
			// We cannot destroy the controller without first
			// destroying the models and waiting for them to
			// become Dead.
			return nil, errors.Trace(
				stateerrors.NewHasHostedModelsError(dying + aliveNonEmpty + aliveEmpty),
			)
		}
		// Ensure that the number of active models has not changed
		// between the query and when the transaction is applied.
		//
		// Note that we assert that each empty model that we intend
		// move to Dead is still Alive, so we're protected from an
		// ABA style problem where an empty model is concurrently
		// removed, and replaced with a non-empty model.
		prereqOps = append(prereqOps,
			assertHostedModelsOp(aliveEmpty+aliveNonEmpty+dying+dead),
		)
	}

	assert := isAliveDoc
	if force {
		assert = bson.D{{"life", m.Life()}}
	}
	var ops []txn.Op
	modelOp := txn.Op{
		C:      modelsC,
		Id:     modelUUID,
		Assert: assert,
	}
	if !destroyingController {
		modelOp.Update = bson.D{
			{
				"$set",
				bson.D{
					{"life", nextLife},
					{"time-of-dying", m.st.nowToTheSecond()},
					{"force-destroyed", force},
					{"destroy-timeout", args.Timeout},
				},
			},
		}
	}
	ops = append(ops, modelOp)
	if destroyingController {
		// We're destroying the controller model, and being asked
		// to check the validity of destroying hosted models,
		// progressing empty models to Dead. We don't want to
		// include the cleanups below, as they assume we're
		// destroying the model.
		return append(prereqOps, ops...), nil
	}

	// Because txn operations execute in order, and may encounter
	// arbitrarily long delays, we need to make sure every op
	// causes a state change that's still consistent; so we make
	// sure the cleanup ops are the last thing that will execute.
	if m.IsControllerModel() {
		ops = append(ops, newCleanupOp(
			cleanupModelsForDyingController, modelUUID,
			// pass through the DestroyModelArgs to the cleanup,
			// so the models can be destroyed according to the
			// same rules.
			args,
		))
	}
	if !isEmpty {
		// We only need to destroy resources if the model is non-empty.
		// It wouldn't normally be harmful to enqueue the cleanups
		// otherwise, except for when we're destroying an empty
		// model in the course of destroying the controller. In
		// that case we'll get errors if we try to enqueue model
		// cleanups, because the cleanups collection is non-global.
		ops = append(ops, newCleanupOp(cleanupApplicationsForDyingModel, modelUUID, args))
		if m.Type() == ModelTypeIAAS {
			ops = append(ops, newCleanupOp(cleanupMachinesForDyingModel, modelUUID, args))
		}
		if args.DestroyStorage != nil {
			// The user has specified that the storage should be destroyed
			// or released, which we can do in a cleanup. If the user did
			// not specify either, then we have already added prereq ops
			// to assert that there is no storage in the model.
			ops = append(ops, newCleanupOp(cleanupStorageForDyingModel, modelUUID, args))
		}
	}
	// Ensure we clean up the branches upon clean up of a model.
	ops = append(ops, newCleanupOp(cleanupBranchesForDyingModel, modelUUID, args))
	return append(prereqOps, ops...), nil
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

// checkModelEntityRefsNoPersistentStorage checks that there is no
// persistent storage in the model. If there is, then an error of
// type hasPersistentStorageError is returned. If there is not,
// txn.Ops are returned to assert the same.
func checkModelEntityRefsNoPersistentStorage(
	db Database, doc *modelEntityRefsDoc,
) ([]txn.Op, error) {
	for _, volumeId := range doc.Volumes {
		volumeTag := names.NewVolumeTag(volumeId)
		detachable, err := isDetachableVolumeTag(db, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if detachable {
			return nil, stateerrors.PersistentStorageError
		}
	}
	for _, filesystemId := range doc.Filesystems {
		filesystemTag := names.NewFilesystemTag(filesystemId)
		detachable, err := isDetachableFilesystemTag(db, filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if detachable {
			return nil, stateerrors.PersistentStorageError
		}
	}
	return noNewStorageModelEntityRefs(doc), nil
}

// checkModelEntityRefsAllReleasableStorage checks that there all
// persistent storage in the model is releasable. If it is, then
// txn.Ops are returned to assert the same; if it is not, then an
// error is returned.
func checkModelEntityRefsAllReleasableStorage(sb *storageBackend, doc *modelEntityRefsDoc, force bool) ([]txn.Op, error) {
	for _, volumeId := range doc.Volumes {
		volumeTag := names.NewVolumeTag(volumeId)
		volume, err := getVolumeByTag(sb.mb, volumeTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !volume.Detachable() {
			continue
		}
		if err := checkStoragePoolReleasable(sb, volume.pool()); err != nil {
			logger.Warningf("error checking releasable volumes for model %s: %v", doc.UUID, err)
			if !force {
				// If the storage cannot be released, return the error without
				// additional annotation.
				if errors.Is(err, stateerrors.StorageNotReleasableError) {
					return nil, errors.Trace(err)
				}
				return nil, errors.Annotatef(err,
					"checking %s is releasable", names.ReadableString(volumeTag),
				)
			}
		}
	}
	for _, filesystemId := range doc.Filesystems {
		filesystemTag := names.NewFilesystemTag(filesystemId)
		filesystem, err := getFilesystemByTag(sb.mb, filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !filesystem.Detachable() {
			continue
		}
		if err := checkStoragePoolReleasable(sb, filesystem.pool()); err != nil {
			logger.Warningf("error checking releasable filesystems for model %s: %v", doc.UUID, err)
			if !force {
				// If the storage cannot be released, return the error without
				// additional annotation.
				if errors.Is(err, stateerrors.StorageNotReleasableError) {
					return nil, errors.Trace(err)
				}
				return nil, errors.Annotatef(err,
					"checking %s is releasable", names.ReadableString(filesystemTag),
				)
			}
		}
	}
	return noNewStorageModelEntityRefs(doc), nil
}

func noNewStorageModelEntityRefs(doc *modelEntityRefsDoc) []txn.Op {
	noNewVolumes := bson.DocElem{
		"volumes", bson.D{{
			"$not", bson.D{{
				"$elemMatch", bson.D{{
					"$nin", doc.Volumes,
				}},
			}},
		}},
		// There are no volumes that are not in
		// the set of volumes we previously knew
		// about => the current set of volumes
		// is a subset of the previously known set.
	}
	noNewFilesystems := bson.DocElem{
		Name: "filesystems", Value: bson.D{{
			"$not", bson.D{{
				"$elemMatch", bson.D{{
					"$nin", doc.Filesystems,
				}},
			}},
		}},
	}
	return []txn.Op{{
		C:  modelEntityRefsC,
		Id: doc.UUID,
		Assert: bson.D{
			noNewVolumes,
			noNewFilesystems,
		},
	}}
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
		Id:     mb.ModelUUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$addToSet", bson.D{{entityField, entityId}}}},
	}
}

func removeModelEntityRefOp(mb modelBackend, entityField, entityId string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     mb.ModelUUID(),
		Update: bson.D{{"$pull", bson.D{{entityField, entityId}}}},
	}
}

// createModelOp returns the operation needed to create
// an model document with the given name and UUID.
func createModelOp(
	modelType ModelType,
	owner names.UserTag,
	name, uuid, controllerUUID, cloudName, cloudRegion, passwordHash string,
	cloudCredential names.CloudCredentialTag,
	migrationMode MigrationMode,
	environVersion int,
) txn.Op {
	doc := &modelDoc{
		Type:            modelType,
		UUID:            uuid,
		Name:            name,
		Life:            Alive,
		Owner:           owner.Id(),
		ControllerUUID:  controllerUUID,
		MigrationMode:   migrationMode,
		EnvironVersion:  environVersion,
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
// a usermodelnameC document with the given owner and model name.
func createUniqueOwnerModelNameOp(owner names.UserTag, modelName string) txn.Op {
	return txn.Op{
		C:      usermodelnameC,
		Id:     userModelNameIndex(owner.Id(), modelName),
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
	return assertModelUsableOp(modelUUID, isAliveDoc)
}

// assertModelNotDeadOp returns a txn.Op that asserts the given
// model UUID refers to a model that is not dead and not currently
// in the process of migration.
func assertModelNotDeadOp(modelUUID string) txn.Op {
	return assertModelUsableOp(modelUUID, notDeadDoc)
}

func assertModelUsableOp(modelUUID string, lifeAssertion bson.D) txn.Op {
	return txn.Op{
		C:      modelsC,
		Id:     modelUUID,
		Assert: append(lifeAssertion, bson.DocElem{Name: "migration-mode", Value: MigrationModeNone}),
	}
}

func checkModelActive(st *State) error {
	return errors.Trace(checkModelUsable(st, func(life Life) bool { return life == Alive }))
}

func checkModelNotDead(st *State) error {
	return errors.Trace(checkModelUsable(st, func(life Life) bool { return life != Dead }))
}

func checkModelUsable(st *State, validLife func(Life) bool) error {
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	if !validLife(model.Life()) {
		return errors.Errorf("model %q is %s", model.Name(), model.Life().String())
	}

	if model.MigrationMode() != MigrationModeNone {
		return errors.Errorf("model %q is being migrated", model.Name())
	}

	return nil
}
