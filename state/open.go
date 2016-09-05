// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/worker"
)

// Open connects to the server described by the given
// info, waits for it to be initialized, and returns a new State
// representing the model connected to.
//
// A policy may be provided, which will be used to validate and
// modify behaviour of certain operations in state. A nil policy
// may be provided.
//
// Open returns unauthorizedError if access is unauthorized.
func Open(
	controllerModelTag names.ModelTag,
	controllerTag names.ControllerTag,
	info *mongo.MongoInfo, opts mongo.DialOpts,
	newPolicy NewPolicyFunc,
) (*State, error) {
	st, err := open(controllerModelTag, info, opts, newPolicy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := st.Model(); err != nil {
		if err := st.Close(); err != nil {
			logger.Errorf("closing State for %s: %v", controllerModelTag, err)
		}
		return nil, errors.Annotatef(err, "cannot read model %s", controllerModelTag.Id())
	}

	// State should only be Opened on behalf of a controller environ; all
	// other *States should be created via ForModel.
	if err := st.start(controllerTag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

func open(
	controllerModelTag names.ModelTag,
	info *mongo.MongoInfo, opts mongo.DialOpts,
	newPolicy NewPolicyFunc,
) (*State, error) {
	logger.Infof("opening state, mongo addresses: %q; entity %v", info.Addrs, info.Tag)
	logger.Debugf("dialing mongo")
	session, err := mongo.DialWithInfo(info.Info, opts)
	if err != nil {
		return nil, maybeUnauthorized(err, "cannot connect to mongodb")
	}
	logger.Debugf("connection established")

	err = mongodbLogin(session, info)
	if err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	logger.Debugf("mongodb login successful")

	st, err := newState(controllerModelTag, controllerModelTag, session, info, newPolicy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// mongodbLogin logs in to the mongodb admin database.
func mongodbLogin(session *mgo.Session, mongoInfo *mongo.MongoInfo) error {
	admin := session.DB("admin")
	if mongoInfo.Tag != nil {
		if err := admin.Login(mongoInfo.Tag.String(), mongoInfo.Password); err != nil {
			return maybeUnauthorized(err, fmt.Sprintf("cannot log in to admin database as %q", mongoInfo.Tag))
		}
	} else if mongoInfo.Password != "" {
		if err := admin.Login(mongo.AdminUser, mongoInfo.Password); err != nil {
			return maybeUnauthorized(err, "cannot log in to admin database")
		}
	}
	return nil
}

// InitializeParams contains the parameters for initializing the state database.
type InitializeParams struct {
	// ControllerModelArgs contains the arguments for creating
	// the controller model.
	ControllerModelArgs ModelArgs

	// CloudName is the name of the cloud that the controller
	// runs in.
	CloudName string

	// Cloud contains the properties of the cloud that the
	// controller runs in.
	Cloud cloud.Cloud

	// CloudCredentials contains the credentials for the owner of
	// the controller model to store in the controller.
	CloudCredentials map[names.CloudCredentialTag]cloud.Credential

	// ControllerConfig contains config attributes for
	// the controller.
	ControllerConfig controller.Config

	// ControllerInheritedConfig contains default config attributes for
	// models on the specified cloud.
	ControllerInheritedConfig map[string]interface{}

	// RegionInheritedConfig contains region specific configuration for
	// models running on specific cloud regions.
	RegionInheritedConfig cloud.RegionConfig

	// NewPolicy is a function that returns the set of state policies
	// to apply.
	NewPolicy NewPolicyFunc

	// MongoInfo contains the information required to address and
	// authenticate with Mongo.
	MongoInfo *mongo.MongoInfo

	// MongoDialOpts contains the dial options for connecting to
	// Mongo.
	MongoDialOpts mongo.DialOpts
}

// Validate checks that the state initialization parameters are valid.
func (p InitializeParams) Validate() error {
	if err := p.ControllerModelArgs.Validate(); err != nil {
		return errors.Trace(err)
	}
	if p.ControllerModelArgs.MigrationMode != MigrationModeNone {
		return errors.NotValidf("migration mode %q", p.ControllerModelArgs.MigrationMode)
	}
	uuid := p.ControllerModelArgs.Config.UUID()
	controllerUUID := p.ControllerConfig.ControllerUUID()
	if uuid == controllerUUID {
		return errors.NotValidf("same controller model uuid (%v) and controller-uuid (%v)", uuid, controllerUUID)
	}
	if p.MongoInfo == nil {
		return errors.NotValidf("nil MongoInfo")
	}
	if p.CloudName == "" {
		return errors.NotValidf("empty CloudName")
	}
	if p.Cloud.Type == "" {
		return errors.NotValidf("empty Cloud")
	}
	if err := validateCloud(p.Cloud); err != nil {
		return errors.Annotate(err, "validating cloud")
	}
	if _, err := validateCloudRegion(p.Cloud, p.CloudName, p.ControllerModelArgs.CloudRegion); err != nil {
		return errors.Annotate(err, "validating controller model cloud region")
	}
	if _, err := validateCloudCredentials(p.Cloud, p.CloudName, p.CloudCredentials); err != nil {
		return errors.Annotate(err, "validating cloud credentials")
	}
	if _, err := validateCloudCredential(
		p.Cloud,
		p.CloudName,
		p.CloudCredentials,
		p.ControllerModelArgs.CloudCredential,
	); err != nil {
		return errors.Annotate(err, "validating controller model cloud credential")
	}
	return nil
}

// Initialize sets up an initial empty state and returns it.
// This needs to be performed only once for the initial controller model.
// It returns unauthorizedError if access is unauthorized.
func Initialize(args InitializeParams) (_ *State, err error) {
	if err := args.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating initialization args")
	}

	// When creating the controller model, the new model
	// UUID is also used as the controller UUID.
	modelTag := names.NewModelTag(args.ControllerModelArgs.Config.UUID())
	st, err := open(modelTag, args.MongoInfo, args.MongoDialOpts, args.NewPolicy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			if closeErr := st.Close(); closeErr != nil {
				logger.Errorf("error closing state while aborting Initialize: %v", closeErr)
			}
		}
	}()
	st.controllerModelTag = modelTag

	// A valid model is used as a signal that the
	// state has already been initalized. If this is the case
	// do nothing.
	if _, err := st.Model(); err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	logger.Infof("initializing controller model %s", modelTag.Id())

	modelOps, err := st.modelSetupOps(
		args.ControllerConfig.ControllerUUID(),
		args.ControllerModelArgs,
		&lineage{
			ControllerConfig: args.ControllerInheritedConfig,
			RegionConfig:     args.RegionInheritedConfig,
		})
	if err != nil {
		return nil, errors.Trace(err)
	}
	salt, err := utils.RandomSalt()
	if err != nil {
		return nil, err
	}

	ops := createInitialUserOps(args.ControllerConfig.ControllerUUID(), args.ControllerModelArgs.Owner, args.MongoInfo.Password, salt)
	ops = append(ops,

		txn.Op{
			C:      controllersC,
			Id:     modelGlobalKey,
			Assert: txn.DocMissing,
			Insert: &controllersDoc{
				CloudName: args.CloudName,
				ModelUUID: st.ModelUUID(),
			},
		},
		createCloudOp(args.Cloud, args.CloudName),
		txn.Op{
			C:      controllersC,
			Id:     apiHostPortsKey,
			Assert: txn.DocMissing,
			Insert: &apiHostPortsDoc{},
		},
		txn.Op{
			C:      controllersC,
			Id:     stateServingInfoKey,
			Assert: txn.DocMissing,
			Insert: &StateServingInfo{},
		},
		txn.Op{
			C:      controllersC,
			Id:     hostedModelCountKey,
			Assert: txn.DocMissing,
			Insert: &hostedModelCountDoc{},
		},
		createSettingsOp(controllersC, controllerSettingsGlobalKey, args.ControllerConfig),
		createSettingsOp(globalSettingsC, controllerInheritedSettingsGlobalKey, args.ControllerInheritedConfig),
	)
	for k, v := range args.Cloud.RegionConfig {
		// Create an entry keyed on cloudname#<key>, value for each region in
		// region-config. The values here are themselves
		// map[string]interface{}.
		ops = append(ops, createSettingsOp(globalSettingsC, regionSettingsGlobalKey(args.CloudName, k), v))
	}

	for tag, cred := range args.CloudCredentials {
		ops = append(ops, createCloudCredentialOp(tag, cred))
	}
	ops = append(ops, modelOps...)

	if err := st.runTransaction(ops); err != nil {
		return nil, errors.Trace(err)
	}
	controllerTag := names.NewControllerTag(args.ControllerConfig.ControllerUUID())
	if err := st.start(controllerTag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// lineage is a composite of inheritable properties for the extent of
// passing them into modelSetupOps.
type lineage struct {
	ControllerConfig map[string]interface{}
	RegionConfig     cloud.RegionConfig
}

// modelSetupOps returns the transactions necessary to set up a model.
func (st *State) modelSetupOps(controllerUUID string, args ModelArgs, inherited *lineage) ([]txn.Op, error) {
	if inherited != nil {
		if err := checkControllerInheritedConfig(inherited.ControllerConfig); err != nil {
			return nil, errors.Trace(err)
		}
	}
	if err := checkModelConfig(args.Config); err != nil {
		return nil, errors.Trace(err)
	}

	controllerModelUUID := st.controllerModelTag.Id()
	modelUUID := args.Config.UUID()
	modelStatusDoc := statusDoc{
		ModelUUID: modelUUID,
		// TODO(fwereade): 2016-03-17 lp:1558657
		Updated: time.Now().UnixNano(),
		// TODO(axw) 2016-04-13 lp:1569632
		// We need to decide how we will
		// represent migration in model status.
		Status: status.StatusAvailable,
	}

	modelUserOps := createModelUserOps(
		modelUUID, args.Owner, args.Owner, args.Owner.Name(), nowToTheSecond(), description.AdminAccess,
	)
	ops := []txn.Op{
		createStatusOp(st, modelGlobalKey, modelStatusDoc),
		createConstraintsOp(st, modelGlobalKey, args.Constraints),
	}
	// Inc ref count for hosted models.
	if controllerModelUUID != modelUUID {
		ops = append(ops, incHostedModelCountOp())
	}

	// Create the default storage pools for the model.
	defaultStoragePoolsOps, err := st.createDefaultStoragePoolsOps(args.StorageProviderRegistry)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, defaultStoragePoolsOps...)

	// Create the final map of config attributes for the model.
	// If we have ControllerInheritedConfig passed in, that means state
	// is being initialised and there won't be any config sources
	// in state.
	var configSources []modelConfigSource
	if inherited != nil {
		configSources = []modelConfigSource{
			{
				name: config.JujuDefaultSource,
				sourceFunc: modelConfigSourceFunc(func() (attrValues, error) {
					return config.ConfigDefaults(), nil
				})},
			{
				name: config.JujuControllerSource,
				sourceFunc: modelConfigSourceFunc(func() (attrValues, error) {
					return inherited.ControllerConfig, nil
				})},
			{
				name: config.JujuRegionSource,
				sourceFunc: modelConfigSourceFunc(func() (attrValues, error) {
					// We return the values specific to this region for this model.
					return attrValues(inherited.RegionConfig[args.CloudRegion]), nil
				})},
		}
	} else {
		rspec := &environs.RegionSpec{Cloud: args.CloudName, Region: args.CloudRegion}
		configSources = modelConfigSources(st, rspec)
	}
	modelCfg, err := composeModelConfigAttributes(args.Config.AllAttrs(), configSources...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Some values require marshalling before storage.
	modelCfg = config.CoerceForStorage(modelCfg)
	ops = append(ops,
		createSettingsOp(settingsC, modelGlobalKey, modelCfg),
		createModelEntityRefsOp(modelUUID),
		createModelOp(
			args.Owner,
			args.Config.Name(),
			modelUUID, controllerUUID,
			args.CloudName, args.CloudRegion, args.CloudCredential,
			args.MigrationMode,
		),
		createUniqueOwnerModelNameOp(args.Owner, args.Config.Name()),
	)
	ops = append(ops, modelUserOps...)
	return ops, nil
}

func (st *State) createDefaultStoragePoolsOps(registry storage.ProviderRegistry) ([]txn.Op, error) {
	m := poolmanager.MemSettings{make(map[string]map[string]interface{})}
	pm := poolmanager.New(m, registry)
	for _, providerType := range registry.StorageProviderTypes() {
		p, err := registry.StorageProvider(providerType)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := poolmanager.AddDefaultStoragePools(p, pm); err != nil {
			return nil, errors.Annotatef(
				err, "adding default storage pools for %q", providerType,
			)
		}
	}

	var ops []txn.Op
	for key, settings := range m.Settings {
		ops = append(ops, createSettingsOp(settingsC, key, settings))
	}
	return ops, nil
}

func maybeUnauthorized(err error, msg string) error {
	if err == nil {
		return nil
	}
	if isUnauthorized(err) {
		return errors.Unauthorizedf("%s: unauthorized mongo access: %v", msg, err)
	}
	return errors.Annotatef(err, msg)
}

func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	// Some unauthorized access errors have no error code,
	// just a simple error string; and some do have error codes
	// but are not of consistent types (LastError/QueryError).
	for _, prefix := range []string{"auth fail", "not authorized", "server returned error on SASL authentication step: Authentication failed."} {
		if strings.HasPrefix(err.Error(), prefix) {
			return true
		}
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized"
	}
	return false
}

// newState creates an incomplete *State, with no running workers or
// controllerTag. You must start() the returned *State before it will
// function correctly.
//
// newState takes responsibility for the supplied *mgo.Session, and will
// close it if it cannot be returned under the aegis of a *State.
func newState(
	modelTag, controllerModelTag names.ModelTag,
	session *mgo.Session, mongoInfo *mongo.MongoInfo,
	newPolicy NewPolicyFunc,
) (_ *State, err error) {

	defer func() {
		if err != nil {
			session.Close()
		}
	}()

	// Set up database.
	rawDB := session.DB(jujuDB)
	database, err := allCollections().Load(rawDB, modelTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := InitDbLogs(session); err != nil {
		return nil, errors.Trace(err)
	}

	// Create State.
	st := &State{
		modelTag:           modelTag,
		controllerModelTag: controllerModelTag,
		mongoInfo:          mongoInfo,
		session:            session,
		database:           database,
		newPolicy:          newPolicy,
	}
	if newPolicy != nil {
		st.policy = newPolicy(st)
	}
	return st, nil
}

// MongoConnectionInfo returns information for connecting to mongo
func (st *State) MongoConnectionInfo() *mongo.MongoInfo {
	return st.mongoInfo
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() string {
	return st.mongoInfo.CACert
}

// Close the connection to the database.
func (st *State) Close() (err error) {
	defer errors.DeferredAnnotatef(&err, "closing state failed")

	var errs []error
	handle := func(name string, err error) {
		if err != nil {
			errs = append(errs, errors.Annotatef(err, "error stopping %s", name))
		}
	}
	if st.workers != nil {
		handle("standard workers", worker.Stop(st.workers))
	}

	st.mu.Lock()
	if st.allManager != nil {
		handle("allwatcher manager", st.allManager.Stop())
	}
	if st.allModelManager != nil {
		handle("allModelWatcher manager", st.allModelManager.Stop())
	}
	if st.allModelWatcherBacking != nil {
		handle("allModelWatcher backing", st.allModelWatcherBacking.Release())
	}
	st.session.Close()
	st.mu.Unlock()

	if len(errs) > 0 {
		for _, err := range errs[1:] {
			logger.Errorf("while closing state: %v", err)
		}
		return errs[0]
	}
	logger.Debugf("closed state without error")
	return nil
}
