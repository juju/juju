// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/permission"
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

	// ForceDestroyed is whether --force was specified when destroying
	// this model. It only has any meaning when the model is dying or
	// dead.
	ForceDestroyed bool `bson:"force-destroyed,omitempty"`
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
	st := ctlr.pool.SystemState()

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

	ops := append(prereqOps, modelOps...)

	// Increment the model count for the cloud to which this model belongs.
	incCloudRefOp, err := incCloudModelRefOp(st, args.CloudName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	ops = append(ops, incCloudRefOp)

	// Create a record for the default space.
	ops = append(ops, createDefaultSpaceOp())

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

	err = newSt.start(st.controllerTag, ctlr.pool.hub)
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

// Owner returns tag representing the owner of the model.
// The owner is the user that created the model.
func (m *Model) Owner() names.UserTag {
	return names.NewUserTag(m.doc.Owner)
}

func modelStatusInvalidCredential() status.StatusInfo {
	return status.StatusInfo{Status: status.Suspended, Message: "suspended since cloud credential is not valid"}
}

// Status returns the status of the model.
func (m *Model) Status() (status.StatusInfo, error) {
	// If model credential is invalid, model is suspended.
	if credentialTag, hasCredential := m.CloudCredential(); hasCredential {
		credential, err := m.st.CloudCredential(credentialTag)
		if err != nil {
			return status.StatusInfo{}, errors.Annotatef(err, "could not get model credential %v", credentialTag.Id())
		}
		if !credential.IsValid() {
			return modelStatusInvalidCredential(), nil
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

// ApplicationEndpointBindings - endpointBinding->space details for each application
type ApplicationEndpointBindings struct {
	AppName  string
	Bindings map[string]string
}

// AllEndpointBindings returns all endpoint->space bindings for every application
func (m *Model) AllEndpointBindings() ([]ApplicationEndpointBindings, error) {
	endpointBindings, closer := m.st.db().GetCollection(endpointBindingsC)
	defer closer()

	var docs []endpointBindingsDoc
	err := endpointBindings.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get endpoint bindings")
	}

	var appEndpointBindings []ApplicationEndpointBindings
	for _, doc := range docs {
		var applicationName string
		applicationKey := m.localID(doc.DocID)
		// for each application deployed we have an instance of ApplicationEndpointBindings struct
		if strings.HasPrefix(applicationKey, "a#") {
			applicationName = applicationKey[2:]
		} else {
			return nil, errors.NotValidf("application key %v", applicationKey)
		}
		endpointBindings := ApplicationEndpointBindings{
			AppName:  applicationName,
			Bindings: doc.Bindings,
		}

		appEndpointBindings = append(appEndpointBindings, endpointBindings)
	}
	return appEndpointBindings, nil
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
				if _, ok := err.(DeletedUserError); !ok {
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
	// IsHasHostedModelsError will be returned.
	//
	// TODO(axw) this should be moved to the Controller type.
	DestroyHostedModels bool

	// DestroyStorage controls whether or not storage in the
	// model (and hosted models, if DestroyHostedModels is true)
	// should be destroyed.
	//
	// This is ternary: nil, false, or true. If nil and
	// there is persistent storage in the model (or hosted
	// models), an error satisfying IsHasPersistentStorageError
	// will be returned.
	DestroyStorage *bool

	// Force specifies whether model destruction will be forced, i.e.
	// keep going despite operational errors.
	Force *bool

	// MaxWait specifies the amount of time that each step in model destroy process
	// will wait before forcing the next step to kick-off. This parameter
	// only makes sense in combination with 'force' set to 'true'.
	MaxWait time.Duration
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

type hasHostedModelsError int

func (e hasHostedModelsError) Error() string {
	s := ""
	if e != 1 {
		s = "s"
	}
	return fmt.Sprintf("hosting %d other model"+s, e)
}

// IsHasHostedModelsError reports whether or not the given error
// was caused by an attempt to destroy the controller model while
// it contained non-empty hosted models, without specifying that
// they should also be destroyed.
func IsHasHostedModelsError(err error) bool {
	_, ok := errors.Cause(err).(hasHostedModelsError)
	return ok
}

type hasPersistentStorageError struct{}

func (hasPersistentStorageError) Error() string {
	return "model contains persistent storage"
}

// IsHasPersistentStorageError reports whether or not the given
// error was caused by an attempt to destroy a model while it
// contained persistent storage, without specifying how the
// storage should be removed (destroyed or released).
func IsHasPersistentStorageError(err error) bool {
	_, ok := errors.Cause(err).(hasPersistentStorageError)
	return ok
}

type modelNotEmptyError struct {
	machines     int
	applications int
	volumes      int
	filesystems  int
}

// Error is part of the error interface.
func (e modelNotEmptyError) Error() string {
	msg := "model not empty, found "
	plural := func(n int, thing string) string {
		s := fmt.Sprintf("%d %s", n, thing)
		if n != 1 {
			s += "s"
		}
		return s
	}
	var contains []string
	if n := e.machines; n > 0 {
		contains = append(contains, plural(n, "machine"))
	}
	if n := e.applications; n > 0 {
		contains = append(contains, plural(n, "application"))
	}
	if n := e.volumes; n > 0 {
		contains = append(contains, plural(n, "volume"))
	}
	if n := e.filesystems; n > 0 {
		contains = append(contains, plural(n, "filesystem"))
	}
	return msg + strings.Join(contains, ", ")
}

// IsModelNotEmptyError reports whether or not the given error was caused
// due to an operation requiring a model to be empty, where the model is
// non-empty.
func IsModelNotEmptyError(err error) bool {
	_, ok := errors.Cause(err).(modelNotEmptyError)
	return ok
}

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
	force := args.Force != nil && *args.Force
	if m.Life() != Alive {
		if !force {
			return nil, errModelNotAlive
		}
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
	modelUUID := m.UUID()
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
			storageOps, err := checkModelEntityRefsAllReleasableStorage(sb, modelEntityRefs)
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
				if !IsModelNotEmptyError(err) {
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
		// hosted model in the course of destroying the controller. In
		// that case we'll get errors if we try to enqueue hosted-model
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
	err := modelNotEmptyError{
		machines:     len(doc.Machines),
		applications: len(doc.Applications),
		volumes:      len(doc.Volumes),
		filesystems:  len(doc.Filesystems),
	}
	if err != (modelNotEmptyError{}) {
		return nil, err
	}

	isEmpty := func(attribute string) bson.DocElem {
		// We consider it empty if the array has no entries, or if the attribute doesn't exist
		return bson.DocElem{
			"$or", []bson.M{{
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
			return nil, hasPersistentStorageError{}
		}
	}
	for _, filesystemId := range doc.Filesystems {
		filesystemTag := names.NewFilesystemTag(filesystemId)
		detachable, err := isDetachableFilesystemTag(db, filesystemTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if detachable {
			return nil, hasPersistentStorageError{}
		}
	}
	return noNewStorageModelEntityRefs(doc), nil
}

// checkModelEntityRefsAllReleasableStorage checks that there all
// persistent storage in the model is releasable. If it is, then
// txn.Ops are returned to assert the same; if it is not, then an
// error is returned.
func checkModelEntityRefsAllReleasableStorage(sb *storageBackend, doc *modelEntityRefsDoc) ([]txn.Op, error) {
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
			return nil, errors.Annotatef(err,
				"cannot release %s", names.ReadableString(volumeTag),
			)
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
			return nil, errors.Annotatef(err,
				"cannot release %s", names.ReadableString(filesystemTag),
			)
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
		"filesystems", bson.D{{
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
	modelType ModelType,
	owner names.UserTag,
	name, uuid, controllerUUID, cloudName, cloudRegion string,
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
