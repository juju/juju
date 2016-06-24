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
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/status"
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
func Open(tag names.ModelTag, info *mongo.MongoInfo, opts mongo.DialOpts, policy Policy) (*State, error) {
	st, err := open(tag, info, opts, policy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := st.Model(); err != nil {
		if err := st.Close(); err != nil {
			logger.Errorf("closing State for %s: %v", tag, err)
		}
		return nil, errors.Annotatef(err, "cannot read model %s", tag.Id())
	}

	// State should only be Opened on behalf of a controller environ; all
	// other *States should be created via ForModel.
	if err := st.start(tag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

func open(tag names.ModelTag, info *mongo.MongoInfo, opts mongo.DialOpts, policy Policy) (*State, error) {
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

	// In rare circumstances, we may be upgrading from pre-1.23, and not have the
	// model UUID available. In that case we need to infer what it might be;
	// we depend on the assumption that this is the only circumstance in which
	// the the UUID might not be known.
	if tag.Id() == "" {
		logger.Warningf("creating state without model tag; inferring bootstrap model")
		ssInfo, err := readRawControllerInfo(session)
		if err != nil {
			session.Close()
			return nil, errors.Trace(err)
		}
		tag = ssInfo.ModelTag
	}

	st, err := newState(tag, session, info, policy)
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
	CloudCredentials map[string]cloud.Credential

	// ControllerConfig contains config attributes for
	// the controller.
	ControllerConfig controller.Config

	// LocalCloudConfig contains default config attributes for
	// models on the specified cloud.
	LocalCloudConfig map[string]interface{}

	// Policy is the set of state policies to apply.
	Policy Policy

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
	if p.ControllerModelArgs.MigrationMode != MigrationModeActive {
		return errors.NotValidf("migration mode %q", p.ControllerModelArgs.MigrationMode)
	}
	uuid := p.ControllerModelArgs.Config.UUID()
	controllerUUID := p.ControllerConfig.ControllerUUID()
	if uuid != controllerUUID {
		return errors.NotValidf("mismatching uuid (%v) and controller-uuid (%v)", uuid, controllerUUID)
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
	if _, err := validateCloudRegion(p.Cloud, p.ControllerModelArgs.CloudRegion); err != nil {
		return errors.Annotate(err, "validating controller model cloud region")
	}
	if _, err := validateCloudCredentials(p.Cloud, p.CloudCredentials); err != nil {
		return errors.Annotate(err, "validating cloud credentials")
	}
	if _, err := validateCloudCredential(
		p.Cloud, p.CloudCredentials,
		p.ControllerModelArgs.CloudCredential,
		p.ControllerModelArgs.Owner,
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
	st, err := open(modelTag, args.MongoInfo, args.MongoDialOpts, args.Policy)
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
	st.controllerTag = modelTag

	// A valid model is used as a signal that the
	// state has already been initalized. If this is the case
	// do nothing.
	if _, err := st.Model(); err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	logger.Infof("initializing controller model %s", modelTag.Id())

	modelOps, err := st.modelSetupOps(args.ControllerModelArgs, args.LocalCloudConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	salt, err := utils.RandomSalt()
	if err != nil {
		return nil, err
	}

	ops := []txn.Op{
		createInitialUserOp(st, args.ControllerModelArgs.Owner, args.MongoInfo.Password, salt),
		{
			C:      controllersC,
			Id:     modelGlobalKey,
			Assert: txn.DocMissing,
			Insert: &controllersDoc{
				CloudName: args.CloudName,
				ModelUUID: st.ModelUUID(),
			},
		},
		createCloudOp(args.Cloud, args.CloudName),
		{
			C:      controllersC,
			Id:     apiHostPortsKey,
			Assert: txn.DocMissing,
			Insert: &apiHostPortsDoc{},
		},
		{
			C:      controllersC,
			Id:     stateServingInfoKey,
			Assert: txn.DocMissing,
			Insert: &StateServingInfo{},
		},
		{
			C:      controllersC,
			Id:     hostedModelCountKey,
			Assert: txn.DocMissing,
			Insert: &hostedModelCountDoc{},
		},
		createSettingsOp(controllersC, controllerSettingsGlobalKey, args.ControllerConfig),
		createSettingsOp(globalSettingsC, cloudGlobalKey(args.CloudName), args.LocalCloudConfig),
	}
	if len(args.CloudCredentials) > 0 {
		credentialsOps := updateCloudCredentialsOps(
			args.ControllerModelArgs.Owner, args.CloudCredentials,
		)
		ops = append(ops, credentialsOps...)
	}
	ops = append(ops, modelOps...)

	if err := st.runTransaction(ops); err != nil {
		return nil, errors.Trace(err)
	}
	if err := st.start(modelTag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// modelSetupOps returns the transactions necessary to set up a model.
func (st *State) modelSetupOps(args ModelArgs, localCloudConfig map[string]interface{}) ([]txn.Op, error) {
	if err := checkLocalCloudConfigDefaults(localCloudConfig); err != nil {
		return nil, errors.Trace(err)
	}
	if err := checkModelConfig(args.Config); err != nil {
		return nil, errors.Trace(err)
	}

	controllerUUID := st.controllerTag.Id()
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

	// When creating the controller model, the new model
	// UUID is also used as the controller UUID.
	isHostedModel := controllerUUID != modelUUID

	modelUserOps := createModelUserOps(
		modelUUID, args.Owner, args.Owner, args.Owner.Name(), nowToTheSecond(), AdminAccess,
	)
	ops := []txn.Op{
		createStatusOp(st, modelGlobalKey, modelStatusDoc),
		createConstraintsOp(st, modelGlobalKey, args.Constraints),
	}
	if isHostedModel {
		ops = append(ops, incHostedModelCountOp())
	}

	// Create the final map of config attributes for the model.
	// If we have localCloudConfig passed in, that means state
	// is being initialised and there won't be any config sources
	// in state.
	var configSources []modelConfigSource
	if len(localCloudConfig) > 0 {
		configSources = []modelConfigSource{{
			name: config.JujuControllerSource,
			sourceFunc: modelConfigSourceFunc(func() (map[string]interface{}, error) {
				return localCloudConfig, nil
			})}}
	} else {
		configSources = modelConfigSources(st)
	}
	modelCfg, cfgSource, err := composeModelConfigAttributes(args.Config.AllAttrs(), configSources...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops,
		createSettingsOp(settingsC, modelGlobalKey, modelCfg),
		createSettingsSourceOp(cfgSource),
		createModelEntityRefsOp(modelUUID),
		createModelOp(
			args.Owner,
			args.Config.Name(),
			modelUUID, controllerUUID,
			args.CloudRegion, args.CloudCredential,
			args.MigrationMode,
		),
		createUniqueOwnerModelNameOp(args.Owner, args.Config.Name()),
	)
	ops = append(ops, modelUserOps...)
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
func newState(modelTag names.ModelTag, session *mgo.Session, mongoInfo *mongo.MongoInfo, policy Policy) (_ *State, err error) {

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
	return &State{
		modelTag:  modelTag,
		mongoInfo: mongoInfo,
		session:   session,
		database:  database,
		policy:    policy,
	}, nil
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
