// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	names "gopkg.in/juju/names.v2"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

// InitializeParams contains the parameters for initializing the state database.
type InitializeParams struct {
	// Clock wraps all calls time. Real uses use clock.WallClock,
	// tests may override with a testing clock.
	Clock clock.Clock

	// ControllerModelArgs contains the arguments for creating
	// the controller model.
	ControllerModelArgs ModelArgs

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
	if p.Clock == nil {
		return errors.NotValidf("missing clock")
	}
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
	if err := validateCloud(p.Cloud); err != nil {
		return errors.Annotate(err, "validating cloud")
	}
	if _, err := validateCloudRegion(p.Cloud, p.ControllerModelArgs.CloudRegion); err != nil {
		return errors.Annotate(err, "validating controller model cloud region")
	}
	if _, err := validateCloudCredentials(p.Cloud, p.CloudCredentials); err != nil {
		return errors.Annotate(err, "validating cloud credentials")
	}
	creds := make(map[string]cloud.Credential, len(p.CloudCredentials))
	for tag, cred := range p.CloudCredentials {
		creds[tag.Id()] = cred
	}
	if _, err := validateCloudCredential(
		p.Cloud,
		creds,
		p.ControllerModelArgs.CloudCredential,
	); err != nil {
		return errors.Annotate(err, "validating controller model cloud credential")
	}
	return nil
}

// InitDatabaseFunc defines a function used to
// create the collections and indices in a Juju database.
type InitDatabaseFunc func(*mgo.Session, string, *controller.Config) error

// Initialize sets up an initial empty state and returns it.
// This needs to be performed only once for the initial controller model.
// It returns unauthorizedError if access is unauthorized.
func Initialize(args InitializeParams) (_ *Controller, _ *State, err error) {
	if err := args.Validate(); err != nil {
		return nil, nil, errors.Annotate(err, "validating initialization args")
	}

	controllerTag := names.NewControllerTag(args.ControllerConfig.ControllerUUID())

	modelUUID := args.ControllerModelArgs.Config.UUID()
	if !names.IsValidModel(modelUUID) {
		return nil, nil, errors.New("invalid model UUID")
	}
	modelTag := names.NewModelTag(modelUUID)

	ctlr, err := OpenController(OpenParams{
		Clock:              args.Clock,
		ControllerTag:      controllerTag,
		ControllerModelTag: modelTag,
		MongoInfo:          args.MongoInfo,
		MongoDialOpts:      args.MongoDialOpts,
		NewPolicy:          args.NewPolicy,
		InitDatabaseFunc:   InitDatabase,
	})
	if err != nil {
		return nil, nil, errors.Annotate(err, "opening controller")
	}
	defer func() {
		if err != nil {
			if closeErr := ctlr.Close(); closeErr != nil {
				logger.Errorf("error closing controller while aborting Initialize: %v", closeErr)
			}
		}
	}()

	st, err := ctlr.NewState(modelTag)
	if err != nil {
		return nil, nil, errors.Annotate(err, "opening state")
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
		return nil, nil, errors.New("already initialized")
	} else if !errors.IsNotFound(err) {
		return nil, nil, errors.Trace(err)
	}

	logger.Infof("initializing controller model %s", modelTag.Id())

	modelOps, modelStatusDoc, err := st.modelSetupOps(
		args.ControllerConfig.ControllerUUID(),
		args.ControllerModelArgs,
		&lineage{
			ControllerConfig: args.ControllerInheritedConfig,
			RegionConfig:     args.RegionInheritedConfig,
		})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	salt, err := utils.RandomSalt()
	if err != nil {
		return nil, nil, err
	}

	dateCreated := st.nowToTheSecond()
	ops := createInitialUserOps(
		args.ControllerConfig.ControllerUUID(),
		args.ControllerModelArgs.Owner,
		args.MongoInfo.Password,
		salt,
		dateCreated,
	)
	ops = append(ops,
		txn.Op{
			C:      controllersC,
			Id:     modelGlobalKey,
			Assert: txn.DocMissing,
			Insert: &controllersDoc{
				CloudName: args.Cloud.Name,
				ModelUUID: st.ModelUUID(),
			},
		},
		createCloudOp(args.Cloud),
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
		ops = append(ops, createSettingsOp(globalSettingsC, regionSettingsGlobalKey(args.Cloud.Name, k), v))
	}

	for tag, cred := range args.CloudCredentials {
		ops = append(ops, createCloudCredentialOp(tag, cred))
	}
	ops = append(ops, modelOps...)

	if err := st.db().RunTransaction(ops); err != nil {
		return nil, nil, errors.Trace(err)
	}
	probablyUpdateStatusHistory(st.db(), modelGlobalKey, modelStatusDoc)
	return ctlr, st, nil
}

// InitDatabase creates all the collections and indices in a Juju database.
func InitDatabase(session *mgo.Session, modelUUID string, settings *controller.Config) error {
	schema := allCollections()
	if err := schema.Create(session.DB(jujuDB), settings); err != nil {
		return errors.Trace(err)
	}
	if err := InitDbLogs(session, modelUUID); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// lineage is a composite of inheritable properties for the extent of
// passing them into modelSetupOps.
type lineage struct {
	ControllerConfig map[string]interface{}
	RegionConfig     cloud.RegionConfig
}

// modelSetupOps returns the transactions necessary to set up a model.
func (st *State) modelSetupOps(controllerUUID string, args ModelArgs, inherited *lineage) ([]txn.Op, statusDoc, error) {
	var modelStatusDoc statusDoc
	if inherited != nil {
		if err := checkControllerInheritedConfig(inherited.ControllerConfig); err != nil {
			return nil, modelStatusDoc, errors.Trace(err)
		}
	}
	if err := checkModelConfig(args.Config); err != nil {
		return nil, modelStatusDoc, errors.Trace(err)
	}

	controllerModelUUID := st.controllerModelTag.Id()
	modelUUID := args.Config.UUID()
	modelStatusDoc = statusDoc{
		ModelUUID: modelUUID,
		Updated:   st.clock().Now().UnixNano(),
		Status:    status.Available,
	}

	modelUserOps := createModelUserOps(
		modelUUID, args.Owner, args.Owner, args.Owner.Name(), st.nowToTheSecond(), permission.AdminAccess,
	)
	ops := []txn.Op{
		createStatusOp(st, modelGlobalKey, modelStatusDoc),
		createConstraintsOp(modelGlobalKey, args.Constraints),
	}
	// Inc ref count for hosted models.
	if controllerModelUUID != modelUUID {
		ops = append(ops, incHostedModelCountOp())
	}

	// Create the default storage pools for the model.
	if args.StorageProviderRegistry != nil {
		defaultStoragePoolsOps, err := st.createDefaultStoragePoolsOps(args.StorageProviderRegistry)
		if err != nil {
			return nil, modelStatusDoc, errors.Trace(err)
		}
		ops = append(ops, defaultStoragePoolsOps...)
	}

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
		return nil, modelStatusDoc, errors.Trace(err)
	}
	// Some values require marshalling before storage.
	modelCfg = config.CoerceForStorage(modelCfg)
	ops = append(ops,
		createSettingsOp(settingsC, modelGlobalKey, modelCfg),
		createModelEntityRefsOp(modelUUID),
		createModelOp(
			args.Type,
			args.Owner,
			args.Config.Name(),
			modelUUID, controllerUUID,
			args.CloudName, args.CloudRegion, args.CloudCredential,
			args.MigrationMode,
			args.EnvironVersion,
		),
		createUniqueOwnerModelNameOp(args.Owner, args.Config.Name()),
	)
	ops = append(ops, modelUserOps...)
	return ops, modelStatusDoc, nil
}

func (st *State) createDefaultStoragePoolsOps(registry storage.ProviderRegistry) ([]txn.Op, error) {
	m := poolmanager.MemSettings{make(map[string]map[string]interface{})}
	pm := poolmanager.New(m, registry)
	providerTypes, err := registry.StorageProviderTypes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, providerType := range providerTypes {
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
