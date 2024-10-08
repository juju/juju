// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	environsContext "github.com/juju/juju/environs/envcontext"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

var (
	logger = internallogger.GetLogger("juju.apiserver.modelmanager")
)

type newCaasBrokerFunc func(_ context.Context, args environs.OpenParams) (caas.Broker, error)

// StateBackend represents the mongo backend.
type StateBackend interface {
	common.ModelManagerBackend
	InvalidateModelCredential(string) error
}

// ModelManagerAPI implements the model manager interface and is
// the concrete implementation of the api end point.
type ModelManagerAPI struct {
	*common.ModelStatusAPI

	// Access control.
	authorizer facade.Authorizer
	isAdmin    bool
	apiUser    names.UserTag

	// Legacy state access.
	state     StateBackend
	ctlrState common.ModelManagerBackend
	model     common.Model
	check     common.BlockCheckerInterface

	// Services required by the model manager.
	domainServicesGetter DomainServicesGetter
	modelService         ModelService
	modelDefaultsService ModelDefaultsService
	cloudService         CloudService
	credentialService    CredentialService
	applicationService   ApplicationService
	networkService       NetworkService
	machineService       MachineService
	configSchemaSource   config.ConfigSchemaSourceGetter
	accessService        AccessService
	modelExporter        func(coremodel.UUID, facade.LegacyStateExporter) ModelExporter
	store                objectstore.ObjectStore
	secretBackendService SecretBackendService

	// ToolsFinder is used to find tools for a given version.
	toolsFinder common.ToolsFinder

	// Broker/Provider management.
	getBroker      newCaasBrokerFunc
	controllerUUID uuid.UUID
}

// NewModelManagerAPI creates a new api server endpoint for managing
// models.
func NewModelManagerAPI(
	ctx context.Context,
	st StateBackend,
	modelExporter func(coremodel.UUID, facade.LegacyStateExporter) ModelExporter,
	ctlrSt common.ModelManagerBackend,
	controllerUUID uuid.UUID,
	services Services,
	configSchemaSource config.ConfigSchemaSourceGetter,
	toolsFinder common.ToolsFinder,
	getBroker newCaasBrokerFunc,
	blockChecker common.BlockCheckerInterface,
	authorizer facade.Authorizer,
	m common.Model,
) (*ModelManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)
	// Pretty much all of the user manager methods have special casing for admin
	// users, so look once when we start and remember if the user is an admin.
	err := authorizer.HasPermission(ctx, permission.SuperuserAccess, st.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, errors.Trace(err)
	}
	isAdmin := err == nil

	return &ModelManagerAPI{
		ModelStatusAPI:       common.NewModelStatusAPI(st, services.MachineService, authorizer, apiUser),
		state:                st,
		domainServicesGetter: services.DomainServicesGetter,
		modelExporter:        modelExporter,
		ctlrState:            ctlrSt,
		cloudService:         services.CloudService,
		credentialService:    services.CredentialService,
		networkService:       services.NetworkService,
		machineService:       services.MachineService,
		applicationService:   services.ApplicationService,
		configSchemaSource:   configSchemaSource,
		store:                services.ObjectStore,
		getBroker:            getBroker,
		check:                blockChecker,
		authorizer:           authorizer,
		toolsFinder:          toolsFinder,
		apiUser:              apiUser,
		isAdmin:              isAdmin,
		model:                m,
		modelService:         services.ModelService,
		modelDefaultsService: services.ModelDefaultsService,
		accessService:        services.AccessService,
		secretBackendService: services.SecretBackendService,
		controllerUUID:       controllerUUID,
	}, nil
}

// authCheck checks if the user is acting on their own behalf, or if they
// are an administrator acting on behalf of another user.
func (m *ModelManagerAPI) authCheck(user names.UserTag) error {
	if m.isAdmin {
		logger.Tracef("%q is a controller admin", m.apiUser.Id())
		return nil
	}

	// We can't just compare the UserTags themselves as the provider part
	// may be unset, and gets replaced with 'local'. We must compare against
	// the Canonical value of the user tag.
	if m.apiUser == user {
		return nil
	}
	return apiservererrors.ErrPerm
}

func (m *ModelManagerAPI) hasWriteAccess(ctx context.Context, modelTag names.ModelTag) (bool, error) {
	err := m.authorizer.HasPermission(ctx, permission.WriteAccess, modelTag)
	return err == nil, err
}

// ConfigSource describes a type that is able to provide config.
// Abstracted primarily for testing.
type ConfigSource interface {
	Config() (*config.Config, error)
}

func (m *ModelManagerAPI) checkAddModelPermission(ctx context.Context, cloud string, userTag names.UserTag) (bool, error) {
	target := permission.ID{
		ObjectType: permission.Cloud,
		Key:        cloud,
	}
	perm, err := m.accessService.ReadUserAccessLevelForTarget(ctx, user.NameFromTag(userTag), target)
	if err != nil && !errors.Is(err, accesserrors.AccessNotFound) {
		return false, errors.Trace(err)
	}
	if !perm.EqualOrGreaterCloudAccessThan(permission.AddModelAccess) {
		return false, nil
	}
	return true, nil
}

// createModelNew is the work in progress logic for moving this facade over to
// the new services layer. It should be considered the new logic that will be
// merged in place of state eventually. We have split it out as a temp work
// around to get the DDL changes needed into Juju before finishing the rest.
func (m *ModelManagerAPI) createModelNew(
	ctx context.Context,
	args params.ModelCreateArgs,
) (coremodel.UUID, error) {
	// TODO (stickupkid): We need to create a saga (pattern) coordinator here,
	// to ensure that anything written to both databases are at least rollback
	// if there was an error. If a failure to rollback occurs, then the endpoint
	// should at least be somewhat idempotent.

	creationArgs := model.ModelCreationArgs{
		CloudRegion: args.CloudRegion,
		Name:        args.Name,
	}

	// We need to get the controller's default cloud and credential. To help
	// Juju users when creating their first models we allow them to omit this
	// information from the model creation args. If they have done exactly this
	// we will try and apply the defaults where authorisation allows us to.
	defaultCloudName, _, err := m.modelService.DefaultModelCloudNameAndCredential(ctx)
	if errors.Is(err, modelerrors.NotFound) {
		return coremodel.UUID(""), errors.New("failed to find default model cloud and credential for controller")
	}

	var cloudTag names.CloudTag
	if args.CloudTag != "" {
		var err error
		cloudTag, err = names.ParseCloudTag(args.CloudTag)
		if err != nil {
			return coremodel.UUID(""), errors.Trace(err)
		}
	} else {
		cloudTag = names.NewCloudTag(defaultCloudName)
	}
	creationArgs.Cloud = cloudTag.Id()

	err = m.authorizer.HasPermission(ctx, permission.SuperuserAccess, m.state.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return coremodel.UUID(""), errors.Trace(err)
	}
	if err != nil {
		canAddModel, err := m.checkAddModelPermission(ctx, cloudTag.Id(), m.apiUser)
		if err != nil {
			return coremodel.UUID(""), errors.Trace(err)
		}
		if !canAddModel {
			return coremodel.UUID(""), apiservererrors.ErrPerm
		}
	}

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return coremodel.UUID(""), errors.Trace(err)
	}

	// a special case of ErrPerm will happen if the user has add-model permission but is trying to
	// create a model for another person, which is not yet supported.
	if !m.isAdmin && ownerTag != m.apiUser {
		return coremodel.UUID(""), errors.Annotatef(apiservererrors.ErrPerm, "%q permission does not permit creation of models for different owners", permission.AddModelAccess)
	}

	user, err := m.accessService.GetUserByName(ctx, user.NameFromTag(ownerTag))
	if err != nil {
		// TODO handle error properly
		return coremodel.UUID(""), errors.Trace(err)
	}
	creationArgs.Owner = user.UUID

	var cloudCredentialTag names.CloudCredentialTag
	if args.CloudCredentialTag != "" {
		var err error
		cloudCredentialTag, err = names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return coremodel.UUID(""), errors.Trace(err)
		}

		creationArgs.Credential = credential.KeyFromTag(cloudCredentialTag)
	}

	// Create the model in the controller database.
	modelID, activator, err := m.modelService.CreateModel(ctx, creationArgs)
	if err != nil {
		return coremodel.UUID(""), errors.Annotatef(err, "failed to create model %q", modelID)
	}

	// We need to get the model domain services from the newly created model
	// above. We should be able to directly access the model domain services
	// because the model manager use the MultiModelContext to access other
	// models.

	// We use the returned model UUID as we can guarantee that's the one that
	// was written to the database.
	modelDomainServices := m.domainServicesGetter.DomainServicesForModel(modelID)
	modelInfoService := modelDomainServices.ModelInfo()

	modelConfigService := modelDomainServices.Config()

	if err := modelConfigService.SetModelConfig(ctx, args.Config); err != nil {
		return modelID, errors.Annotatef(err, "failed to set model config for model %q", modelID)
	}

	// TODO (stickupkid): Once tlm has fixed the CreateModel method to read
	// from the model database to create the model, move the activator call
	// to the end of the method.
	if err := activator(ctx); err != nil {
		return modelID, errors.Annotatef(err, "failed to finalise model %q", modelID)
	}

	// Create the model information in the model database. This information
	// is read-only and is used for providers and brokers without the need
	// to query the controller database.
	if err := modelInfoService.CreateModel(ctx, m.controllerUUID); err != nil {
		return modelID, errors.Annotatef(err, "failed to create model info for model %q", modelID)
	}

	// Reload the substrate spaces for the newly created model.
	return modelID, reloadSpaces(ctx, modelDomainServices.Network())
}

// reloadSpaces wraps the call to ReloadSpaces and its returned errors.
func reloadSpaces(ctx context.Context, modelNetworkService NetworkService) error {
	if err := modelNetworkService.ReloadSpaces(ctx); err != nil {
		if errors.Is(err, errors.NotSupported) {
			logger.Debugf("Not performing spaces load on a non-networking environment")
		} else {
			return errors.Annotate(err, "Failed to perform spaces discovery")
		}
	}
	return nil
}

// CreateModel creates a new model using the account and
// model config specified in the args.
func (m *ModelManagerAPI) CreateModel(ctx context.Context, args params.ModelCreateArgs) (params.ModelInfo, error) {
	result := params.ModelInfo{}

	// Get the controller model first. We need it both for the state
	// server owner and the ability to get the config.
	controllerModel, err := m.ctlrState.Model()
	if err != nil {
		return result, errors.Trace(err)
	}

	var cloudTag names.CloudTag
	cloudRegionName := args.CloudRegion
	if args.CloudTag != "" {
		var err error
		cloudTag, err = names.ParseCloudTag(args.CloudTag)
		if err != nil {
			return result, errors.Trace(err)
		}
	} else {
		cloudTag = names.NewCloudTag(controllerModel.CloudName())
	}
	if cloudRegionName == "" && cloudTag.Id() == controllerModel.CloudName() {
		cloudRegionName = controllerModel.CloudRegion()
	}

	err = m.authorizer.HasPermission(ctx, permission.SuperuserAccess, m.state.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}
	if err != nil {
		canAddModel, err := m.checkAddModelPermission(ctx, cloudTag.Id(), m.apiUser)
		if err != nil {
			return result, errors.Trace(err)
		}
		if !canAddModel {
			return result, apiservererrors.ErrPerm
		}
	}

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	// a special case of ErrPerm will happen if the user has add-model permission but is trying to
	// create a model for another person, which is not yet supported.
	if !m.isAdmin && ownerTag != m.apiUser {
		return result, errors.Annotatef(apiservererrors.ErrPerm, "%q permission does not permit creation of models for different owners", permission.AddModelAccess)
	}

	cloud, err := m.cloudService.Cloud(ctx, cloudTag.Id())
	if err != nil {
		if errors.Is(err, errors.NotFound) && args.CloudTag != "" {
			// A cloud was specified, and it was not found.
			// Annotate the error with the supported clouds.
			clouds, err := m.cloudService.ListAll(ctx)
			if err != nil {
				return result, errors.Trace(err)
			}
			cloudNames := make([]string, 0, len(clouds))
			for _, cld := range clouds {
				cloudNames = append(cloudNames, cld.Name)
			}
			sort.Strings(cloudNames)
			return result, errors.NewNotFound(err, fmt.Sprintf(
				"cloud %q not found, expected one of %q",
				cloudTag.Id(), cloudNames,
			))
		}
		return result, errors.Annotate(err, "getting cloud definition")
	}

	var cloudCredentialTag names.CloudCredentialTag
	if args.CloudCredentialTag != "" {
		var err error
		cloudCredentialTag, err = names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return result, errors.Trace(err)
		}
	} else {
		if ownerTag == controllerModel.Owner() {
			cloudCredentialTag, _ = controllerModel.CloudCredentialTag()
		} else {
			// TODO(axw) check if the user has one and only one
			// cloud credential, and if so, use it? For now, we
			// require the user to specify a credential unless
			// the cloud does not require one.
			var hasEmpty bool
			for _, authType := range cloud.AuthTypes {
				if authType != jujucloud.EmptyAuthType {
					continue
				}
				hasEmpty = true
				break
			}
			if !hasEmpty {
				return result, errors.NewNotValid(nil, "no credential specified")
			}
		}
	}

	var cred *jujucloud.Credential
	if cloudCredentialTag != (names.CloudCredentialTag{}) {
		credentialValue, err := m.credentialService.CloudCredential(ctx, credential.KeyFromTag(cloudCredentialTag))
		if err != nil {
			return result, errors.Annotate(err, "getting credential")
		}
		cloudCredential := jujucloud.NewNamedCredential(
			credentialValue.Label,
			credentialValue.AuthType(),
			credentialValue.Attributes(),
			credentialValue.Revoked,
		)
		cred = &cloudCredential
	}

	cloudSpec, err := environscloudspec.MakeCloudSpec(*cloud, cloudRegionName, cred)
	if err != nil {
		return result, errors.Trace(err)
	}

	// createModelNew represents the logic needed for moving to DQlite. It is in
	// a half finished state at the moment for the purpose of removing the model
	// manager service. This check will go in the very near future.
	// We check here if the modelService is nil. If it is then we are in testing
	// mode and don't make the calls so test can keep passing.
	// THIS IS VERY TEMPORARY.
	var modelUUID coremodel.UUID
	if m.modelService != nil {
		args.CloudRegion = cloudRegionName
		modelUUID, err = m.createModelNew(ctx, args)
		if err != nil {
			return result, err
		}

		if args.Config == nil {
			args.Config = map[string]any{}
		}
		args.Config[config.UUIDKey] = modelUUID.String()
	}

	configService := m.domainServicesGetter.DomainServicesForModel(modelUUID).Config()
	newConfig, err := configService.ModelConfig(ctx)
	if err != nil {
		return result, errors.Annotate(err, "failed to get config")
	}

	var createdModel common.Model
	if jujucloud.CloudIsCAAS(*cloud) {
		createdModel, err = m.newCAASModel(
			ctx,
			cloudSpec,
			args,
			cloudTag,
			cloudRegionName,
			cloudCredentialTag,
			ownerTag,
			newConfig,
		)
	} else {
		createdModel, err = m.newIAASModel(
			ctx,
			cloudSpec,
			cloudTag,
			cloudRegionName,
			cloudCredentialTag,
			ownerTag,
			newConfig,
		)
	}
	if err != nil {
		return result, errors.Trace(err)
	}

	modelInfo, err := m.getModelInfo(ctx, createdModel.ModelTag(), false)
	if err != nil {
		return result, err
	}

	return modelInfo, nil
}

func (m *ModelManagerAPI) newCAASModel(
	ctx context.Context,
	cloudSpec environscloudspec.CloudSpec,
	createArgs params.ModelCreateArgs,
	cloudTag names.CloudTag,
	cloudRegionName string,
	cloudCredentialTag names.CloudCredentialTag,
	ownerTag names.UserTag,
	newConfig *config.Config,
) (_ common.Model, err error) {
	defer func() {
		// Retain the error stack but with a better message.
		if errors.Is(err, errors.AlreadyExists) {
			err = errors.Wrap(err, errors.NewAlreadyExists(nil,
				`
the model cannot be created because a namespace with the proposed
model name already exists in the k8s cluster.
Please choose a different model name.
`[1:],
			))
		}
	}()

	broker, err := m.getBroker(ctx, environs.OpenParams{
		ControllerUUID: m.controllerUUID.String(),
		Cloud:          cloudSpec,
		Config:         newConfig,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to open kubernetes client")
	}

	callCtx := environsContext.WithoutCredentialInvalidator(ctx)
	if err = broker.Create(
		callCtx,
		environs.CreateParams{ControllerUUID: m.controllerUUID.String()},
	); err != nil {
		return nil, errors.Annotatef(err, "creating namespace %q", createArgs.Name)
	}

	storageProviderRegistry := stateenvirons.NewStorageProviderRegistry(broker)

	model, st, err := m.state.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeCAAS,
		CloudName:               cloudTag.Id(),
		CloudRegion:             cloudRegionName,
		CloudCredential:         cloudCredentialTag,
		Config:                  newConfig,
		Owner:                   ownerTag,
		StorageProviderRegistry: storageProviderRegistry,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to create new model")
	}
	defer st.Close()

	return model, nil
}

func (m *ModelManagerAPI) newIAASModel(
	ctx context.Context,
	cloudSpec environscloudspec.CloudSpec,
	cloudTag names.CloudTag,
	cloudRegionName string,
	cloudCredentialTag names.CloudCredentialTag,
	ownerTag names.UserTag,
	newConfig *config.Config,
) (common.Model, error) {
	// Create the Environ.
	env, err := environs.New(ctx, environs.OpenParams{
		ControllerUUID: m.controllerUUID.String(),
		Cloud:          cloudSpec,
		Config:         newConfig,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to open environ")
	}

	callCtx := environsContext.WithoutCredentialInvalidator(ctx)
	err = env.Create(
		callCtx,
		environs.CreateParams{
			ControllerUUID: m.controllerUUID.String(),
		},
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create environ")
	}
	storageProviderRegistry := stateenvirons.NewStorageProviderRegistry(env)

	// NOTE: check the agent-version of the config, and if it is > the current
	// version, it is not supported, also check existing tools, and if we don't
	// have tools for that version, also die.
	model, st, err := m.state.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               cloudTag.Id(),
		CloudRegion:             cloudRegionName,
		CloudCredential:         cloudCredentialTag,
		Config:                  newConfig,
		Owner:                   ownerTag,
		StorageProviderRegistry: storageProviderRegistry,
		EnvironVersion:          env.Provider().Version(),
	})
	if err != nil {
		// Clean up the environ.
		if e := env.Destroy(callCtx); e != nil {
			logger.Warningf("failed to destroy environ, error %v", e)
		}
		return nil, errors.Annotate(err, "failed to create new model")
	}
	defer st.Close()

	return model, nil
}

func (m *ModelManagerAPI) dumpModel(ctx context.Context, args params.Entity, simplified bool) ([]byte, error) {
	modelTag, err := names.ParseModelTag(args.Tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if !m.isAdmin {
		if err := m.authorizer.HasPermission(ctx, permission.AdminAccess, modelTag); err != nil {
			return nil, err
		}
	}

	modelState, release, err := m.state.GetBackend(modelTag.Id())
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(apiservererrors.ErrBadId)
		}
		return nil, errors.Trace(err)
	}
	defer release()

	exportConfig := state.ExportConfig{IgnoreIncompleteModel: true}
	if simplified {
		exportConfig.SkipActions = true
		exportConfig.SkipAnnotations = true
		exportConfig.SkipCloudImageMetadata = true
		exportConfig.SkipCredentials = true
		exportConfig.SkipIPAddresses = true
		exportConfig.SkipSettings = true
		exportConfig.SkipSSHHostKeys = true
		exportConfig.SkipStatusHistory = true
		exportConfig.SkipLinkLayerDevices = true
	}

	model, err := m.modelExporter(coremodel.UUID(modelTag.Id()), modelState).ExportModelPartial(ctx, exportConfig, m.store)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bytes, err := description.Serialize(model)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bytes, nil
}

func (m *ModelManagerAPI) dumpModelDB(ctx context.Context, args params.Entity) (map[string]interface{}, error) {
	modelTag, err := names.ParseModelTag(args.Tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if !m.isAdmin {
		if err := m.authorizer.HasPermission(ctx, permission.AdminAccess, modelTag); err != nil {
			return nil, err
		}
	}

	type dumper interface {
		DumpAll() (map[string]interface{}, error)
		ModelTag() names.ModelTag
	}

	var st dumper = m.state
	if st.ModelTag() != modelTag {
		newSt, release, err := m.state.GetBackend(modelTag.Id())
		if errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(apiservererrors.ErrBadId)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		defer release()
		st = newSt
	}

	return st.DumpAll()
}

// DumpModels will export the models into the database agnostic
// representation. The user needs to either be a controller admin, or have
// admin privileges on the model itself.
func (m *ModelManagerAPI) DumpModels(ctx context.Context, args params.DumpModelRequest) params.StringResults {
	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		bytes, err := m.dumpModel(ctx, entity, args.Simplified)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// We know here that the bytes are valid YAML.
		results.Results[i].Result = string(bytes)
	}
	return results
}

// DumpModelsDB will gather all documents from all model collections
// for the specified model. The map result contains a map of collection
// names to lists of documents represented as maps.
func (m *ModelManagerAPI) DumpModelsDB(ctx context.Context, args params.Entities) params.MapResults {
	results := params.MapResults{
		Results: make([]params.MapResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		dumped, err := m.dumpModelDB(ctx, entity)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = dumped
	}
	return results
}

// ListModelSummaries returns models that the specified user
// has access to in the current server.  Controller admins (superuser)
// can list models for any user.  Other users
// can only ask about their own models.
func (m *ModelManagerAPI) ListModelSummaries(ctx context.Context, req params.ModelSummariesRequest) (params.ModelSummaryResults, error) {
	userTag, err := names.ParseUserTag(req.UserTag)
	if err != nil {
		return params.ModelSummaryResults{}, errors.Trace(err)
	}

	err = m.authCheck(userTag)
	if err != nil {
		return params.ModelSummaryResults{}, errors.Trace(err)
	}

	if req.All {
		if !m.isAdmin {
			return params.ModelSummaryResults{}, fmt.Errorf(
				"%w: cannot list all models as non-admin user", apiservererrors.ErrPerm,
			)
		}
		return m.listAllModelSummaries(ctx)
	} else {
		return m.listModelSummariesForUser(ctx, userTag)
	}
}

// listAllModelSummaries returns the model summary results containing summaries
// for all the models known to the controller.
func (m *ModelManagerAPI) listAllModelSummaries(ctx context.Context) (params.ModelSummaryResults, error) {
	result := params.ModelSummaryResults{}
	modelInfos, err := m.modelService.ListAllModelSummaries(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Results = make([]params.ModelSummaryResult, len(modelInfos))
	for i, mi := range modelInfos {
		summary, err := m.makeModelSummary(ctx, mi)
		if err != nil {
			result.Results[i] = params.ModelSummaryResult{Error: apiservererrors.ServerError(err)}
		} else {
			result.Results[i] = params.ModelSummaryResult{Result: summary}
		}
	}
	return result, nil
}

// listModelSummariesForUser returns the model summary results containing
// summaries for all the models known to the user.
func (m *ModelManagerAPI) listModelSummariesForUser(ctx context.Context, tag names.UserTag) (params.ModelSummaryResults, error) {
	result := params.ModelSummaryResults{}
	modelInfos, err := m.modelService.ListModelSummariesForUser(ctx, user.NameFromTag(tag))
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Results = make([]params.ModelSummaryResult, len(modelInfos))
	for i, mi := range modelInfos {
		summary, err := m.makeUserModelSummary(ctx, mi)
		if err != nil {
			result.Results[i] = params.ModelSummaryResult{Error: apiservererrors.ServerError(err)}
		} else {
			result.Results[i] = params.ModelSummaryResult{Result: summary}
		}
	}
	return result, nil
}

func (m *ModelManagerAPI) makeUserModelSummary(ctx context.Context, mi coremodel.UserModelSummary) (*params.ModelSummary, error) {
	userAccess, err := common.EncodeAccess(mi.UserAccess)
	if err != nil && !errors.Is(err, errors.NotValid) {
		return nil, errors.Trace(err)
	}
	ms, err := m.makeModelSummary(ctx, mi.ModelSummary)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ms.UserAccess = userAccess
	ms.UserLastConnection = mi.UserLastConnection
	return ms, nil
}

func (m *ModelManagerAPI) makeModelSummary(ctx context.Context, mi coremodel.ModelSummary) (*params.ModelSummary, error) {
	credTag, err := mi.CloudCredentialKey.Tag()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// These should never be invalid since it has come from our database. We
	// have to check anyway since the names package will panic if they are
	// somehow not.
	if !names.IsValidCloud(mi.CloudName) {
		return nil, apiservererrors.ServerError(
			fmt.Errorf("invalid cloud name %q", mi.CloudName),
		)
	}
	cloudTag := names.NewCloudTag(mi.CloudName)
	userTag := names.NewUserTag(mi.OwnerName.Name())

	// TODO(aflynn): 07-08-24 Move this check into the function on model domain
	// once the state is in domain.
	err = m.fillInStatusBasedOnCloudCredentialValidity(ctx, &mi)
	if err != nil {
		return nil, apiservererrors.ServerError(
			errors.Annotatef(err, "listing model summaries: filling in status for missing cloud credential"),
		)
	}

	summary := &params.ModelSummary{
		Name:           mi.Name,
		UUID:           mi.UUID.String(),
		Type:           mi.ModelType.String(),
		OwnerTag:       userTag.String(),
		ControllerUUID: mi.ControllerUUID,
		IsController:   mi.IsController,
		Life:           mi.Life,

		CloudTag:    cloudTag.String(),
		CloudRegion: mi.CloudRegion,

		CloudCredentialTag: credTag.String(),

		ProviderType: mi.CloudType,
		AgentVersion: &mi.AgentVersion,

		Status: common.EntityStatusFromState(mi.Status),
		Counts: []params.ModelEntityCount{},
	}
	if mi.MachineCount > 0 {
		summary.Counts = append(summary.Counts, params.ModelEntityCount{Entity: params.Machines, Count: mi.MachineCount})
	}

	if mi.CoreCount > 0 {
		summary.Counts = append(summary.Counts, params.ModelEntityCount{Entity: params.Cores, Count: mi.CoreCount})
	}

	if mi.UnitCount > 0 {
		summary.Counts = append(summary.Counts, params.ModelEntityCount{Entity: params.Units, Count: mi.UnitCount})
	}

	if mi.Migration != nil {
		summary.Migration = &params.ModelMigrationStatus{
			Status: mi.Migration.Status,
			Start:  mi.Migration.Start,
			End:    mi.Migration.End,
		}
	}

	return summary, nil
}

// fillInStatusBasedOnCloudCredentialValidity fills in the Status on every model
// (if credential is invalid).
func (m *ModelManagerAPI) fillInStatusBasedOnCloudCredentialValidity(ctx context.Context, summary *coremodel.ModelSummary) error {
	if summary.CloudCredentialKey.IsZero() {
		return nil
	}
	tag, err := summary.CloudCredentialKey.Tag()
	if err != nil {
		return errors.Trace(err)
	}
	cred, err := m.credentialService.CloudCredential(ctx, credential.KeyFromTag(tag))
	if err != nil {
		return errors.Trace(err)
	}
	if cred.Invalid {
		summary.Status = state.ModelStatusInvalidCredential(cred.InvalidReason)
	}
	return nil
}

// ListModels returns the models that the specified user
// has access to in the current server.  Controller admins (superuser)
// can list models for any user.  Other users
// can only ask about their own models.
func (m *ModelManagerAPI) ListModels(ctx context.Context, userEntity params.Entity) (params.UserModelList, error) {
	result := params.UserModelList{}

	userTag, err := names.ParseUserTag(userEntity.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}

	err = m.authCheck(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	ctrlUser, err := m.accessService.GetUserByName(ctx, user.NameFromTag(userTag))
	if err != nil {
		return result, errors.Trace(err)
	}

	var models []coremodel.Model
	// If the currently logged in user is an admin we list all models in the
	// controller.
	if m.isAdmin {
		models, err = m.modelService.ListAllModels(ctx)
	} else {
		models, err = m.modelService.ListModelsForUser(ctx, ctrlUser.UUID)
	}

	if err != nil {
		return result, errors.Trace(err)
	}

	for _, mi := range models {
		var lastConnection *time.Time
		lc, err := m.accessService.LastModelLogin(ctx, user.NameFromTag(userTag), mi.UUID)
		if errors.Is(err, accesserrors.UserNeverAccessedModel) {
			lastConnection = nil
		} else if errors.Is(err, modelerrors.NotFound) {
			// Continue if the model has been removed since we got the UUID.
			continue
		} else if err != nil {
			return result, errors.Annotatef(err, "getting last login time for user %q on model %q", userTag.Name(), mi.Name)
		} else {
			lastConnection = &lc
		}

		result.UserModels = append(result.UserModels, params.UserModel{
			Model: params.Model{
				Name:     mi.Name,
				UUID:     mi.UUID.String(),
				Type:     string(mi.ModelType),
				OwnerTag: names.NewUserTag(mi.OwnerName.Name()).String(),
			},
			LastConnection: lastConnection,
		})
	}
	return result, nil
}

// DestroyModels will try to destroy the specified models.
// If there is a block on destruction, this method will return an error.
// From ModelManager v7 onwards, DestroyModels gains 'force' and 'max-wait' parameters.
func (m *ModelManagerAPI) DestroyModels(ctx context.Context, args params.DestroyModelsParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Models)),
	}

	destroyModel := func(modelUUID string, destroyStorage, force *bool, maxWait *time.Duration, timeout *time.Duration) error {
		st, releaseSt, err := m.state.GetBackend(modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		defer releaseSt()

		stModel, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}
		if !m.isAdmin {
			if err := m.authorizer.HasPermission(ctx, permission.AdminAccess, stModel.ModelTag()); err != nil {
				return err
			}
		}

		err = errors.Trace(common.DestroyModel(ctx, st, destroyStorage, force, maxWait, timeout))
		if err != nil {
			return errors.Trace(err)
		}

		// TODO (stickupkid): There are consequences to this failing after the
		// model has been deleted. Although in it's current guise this shouldn't
		// cause too much fallout. If we're unable to delete the model from the
		// database, then we won't be able to create a new model with the same
		// model uuid as there is a UNIQUE constraint on the model uuid column.
		// TODO (tlm): The modelService nil check will go when the tests are
		// moved from mongo.
		if m.modelService != nil {
			// We need to get the model domain services from the model
			// We should be able to directly access the model domain services
			// because the model manager uses the MultiModelContext to access
			// other models.
			modelUUID := coremodel.UUID(stModel.UUID())

			// TODO (stickupkid): We can't the delete the model info when
			// destroying the model at the moment. Attempting to delete the
			// model causes everything to lock up. Once we implement tear-down
			// we'll need to ensure we correctly delete the model info.
			// We need to progress the life of the model, atm it goes from
			// alive to dead, skipping dying.
			//
			// modelDomainServices := m.domainServicesGetter.DomainServicesForModel(modelUUID)
			// modelInfoService := modelDomainServices.ModelInfo()
			// if err := modelInfoService.DeleteModel(ctx, modelUUID); err != nil && !errors.Is(err, modelerrors.NotFound) {
			// 	return errors.Annotatef(err, "failed to delete model info for model %q", modelUUID)
			// }

			err = m.modelService.DeleteModel(ctx, modelUUID)
			if err != nil && errors.Is(err, modelerrors.NotFound) {
				return nil
			}
			return errors.Annotatef(err, "failed to delete model %q", modelUUID)
		}
		return nil
	}

	for i, arg := range args.Models {
		tag, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err := destroyModel(tag.Id(), arg.DestroyStorage, arg.Force, arg.MaxWait, arg.Timeout); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}

// ModelInfo returns information about the specified models.
func (m *ModelManagerAPI) ModelInfo(ctx context.Context, args params.Entities) (params.ModelInfoResults, error) {
	results := params.ModelInfoResults{
		Results: make([]params.ModelInfoResult, len(args.Entities)),
	}

	getModelInfo := func(arg params.Entity) (params.ModelInfo, error) {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			return params.ModelInfo{}, errors.Trace(err)
		}
		ok, err := m.checkReadModelPermission(ctx, coremodel.UUID(tag.Id()), user.NameFromTag(m.apiUser))
		if err != nil {
			return params.ModelInfo{}, errors.Trace(err)
		} else if !ok {
			return params.ModelInfo{}, errors.Trace(apiservererrors.ErrPerm)
		}
		modelInfo, err := m.getModelInfo(ctx, tag, true)
		if err != nil {
			return params.ModelInfo{}, errors.Trace(err)
		}
		if modelInfo.CloudCredentialTag != "" {
			credentialTag, err := names.ParseCloudCredentialTag(modelInfo.CloudCredentialTag)
			if err != nil {
				return params.ModelInfo{}, errors.Trace(err)
			}
			cred, err := m.credentialService.CloudCredential(ctx, credential.KeyFromTag(credentialTag))
			if err != nil {
				return params.ModelInfo{}, errors.Trace(err)
			}
			valid := !cred.Invalid
			modelInfo.CloudCredentialValidity = &valid
		}
		return modelInfo, nil
	}

	for i, arg := range args.Entities {
		modelInfo, err := getModelInfo(arg)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = &modelInfo
	}
	return results, nil
}

func (m *ModelManagerAPI) getModelInfo(ctx context.Context, tag names.ModelTag, withSecrets bool) (params.ModelInfo, error) {
	st, release, err := m.state.GetBackend(tag.Id())
	if errors.Is(err, errors.NotFound) {
		return params.ModelInfo{}, errors.Trace(apiservererrors.ErrPerm)
	} else if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	defer release()

	model, err := st.Model()
	if errors.Is(err, errors.NotFound) {
		return params.ModelInfo{}, errors.Trace(apiservererrors.ErrPerm)
	} else if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}

	modelUUID := model.UUID()

	info := params.ModelInfo{
		Name:           model.Name(),
		Type:           string(model.Type()),
		UUID:           modelUUID,
		ControllerUUID: m.controllerUUID.String(),
		IsController:   st.IsController(),
		OwnerTag:       model.Owner().String(),
		Life:           life.Value(model.Life().String()),
		CloudTag:       names.NewCloudTag(model.CloudName()).String(),
		CloudRegion:    model.CloudRegion(),
	}

	if cloudCredentialTag, ok := model.CloudCredentialTag(); ok {
		info.CloudCredentialTag = cloudCredentialTag.String()
	}

	// If model is not alive - dying or dead - or if it is being imported,
	// there is no guarantee that the rest of the call will succeed.
	// For these models we can ignore NotFound errors coming from persistence layer.
	// However, for Alive models, these errors are genuine and cannot be ignored.
	ignoreNotFoundError := model.Life() != state.Alive || model.MigrationMode() == state.MigrationModeImporting

	// If we received an error and cannot ignore it, we should consider it fatal and surface it.
	// We should do the same if we can ignore NotFound errors but the given error is of some other type.
	shouldErr := func(thisErr error) bool {
		if thisErr == nil {
			return false
		}
		return !ignoreNotFoundError || !(errors.Is(thisErr, errors.NotFound) || errors.Is(thisErr, modelerrors.NotFound))
	}
	cfg, err := model.Config()
	if shouldErr(err) {
		return params.ModelInfo{}, errors.Trace(err)
	}
	if err == nil {
		info.ProviderType = cfg.Type()

		if agentVersion, exists := cfg.AgentVersion(); exists {
			info.AgentVersion = &agentVersion
		}
	}

	status, err := model.Status()
	if shouldErr(err) {
		return params.ModelInfo{}, errors.Trace(err)
	}
	if err == nil {
		entityStatus := common.EntityStatusFromState(status)
		info.Status = entityStatus
	}

	modelAdmin := m.isModelAdmin(ctx, tag)
	info.Users, err = common.ModelUserInfo(ctx, m.modelService, tag, user.NameFromTag(m.apiUser), modelAdmin)
	if shouldErr(err) {
		return params.ModelInfo{}, errors.Annotate(err, "getting model user info")
	}

	canSeeMachinesAndSecrets := modelAdmin
	if !canSeeMachinesAndSecrets {
		canSeeMachinesAndSecrets, err = m.hasWriteAccess(ctx, tag)
		if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return params.ModelInfo{}, errors.Trace(err)
		}
	}
	if canSeeMachinesAndSecrets {
		if info.Machines, err = common.ModelMachineInfo(ctx, st, m.machineService); shouldErr(err) {
			return params.ModelInfo{}, err
		}
	}
	if withSecrets && canSeeMachinesAndSecrets {
		backends, err := m.secretBackendService.BackendSummaryInfoForModel(ctx, coremodel.UUID(modelUUID))
		if shouldErr(err) {
			return params.ModelInfo{}, errors.Trace(err)
		}
		for _, backend := range backends {
			name := backend.Name
			if name == kubernetes.BackendName {
				name = kubernetes.BuiltInName(model.Name())
			}
			info.SecretBackends = append(info.SecretBackends, params.SecretBackendResult{
				// Don't expose the id.
				NumSecrets: backend.NumSecrets,
				Status:     backend.Status,
				Message:    backend.Message,
				Result: params.SecretBackend{
					Name:                name,
					BackendType:         backend.BackendType,
					TokenRotateInterval: backend.TokenRotateInterval,
					Config:              backend.Config,
				},
			})
		}
	}

	migration, err := st.LatestMigration()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return params.ModelInfo{}, errors.Trace(err)
	}
	if err == nil {
		startTime := migration.StartTime()
		endTime := new(time.Time)
		*endTime = migration.EndTime()
		var zero time.Time
		if *endTime == zero {
			endTime = nil
		}
		info.Migration = &params.ModelMigrationStatus{
			Status: migration.StatusMessage(),
			Start:  &startTime,
			End:    endTime,
		}
	}

	fs, err := m.applicationService.GetSupportedFeatures(ctx)
	if err != nil {
		return params.ModelInfo{}, err
	}
	for _, feat := range fs.AsList() {
		mappedFeat := params.SupportedFeature{
			Name:        feat.Name,
			Description: feat.Description,
		}

		if feat.Version != nil {
			mappedFeat.Version = feat.Version.String()
		}

		info.SupportedFeatures = append(info.SupportedFeatures, mappedFeat)
	}
	return info, nil
}

// ModifyModelAccess changes the model access granted to users.
func (m *ModelManagerAPI) ModifyModelAccess(ctx context.Context, args params.ModifyModelAccessRequest) (result params.ErrorResults, _ error) {
	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}

	err := m.authorizer.HasPermission(ctx, permission.SuperuserAccess, m.state.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}
	canModifyController := err == nil

	if len(args.Changes) == 0 {
		return result, nil
	}

	for i, arg := range args.Changes {
		modelAccess := permission.Access(arg.Access)

		modelTag, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Annotate(err, "could not modify model access"))
			continue
		}
		err = m.authorizer.HasPermission(ctx, permission.AdminAccess, modelTag)
		if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return result, errors.Trace(err)
		}
		canModify := err == nil || canModifyController

		if !canModify {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		targetUserTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Annotate(err, "could not modify model access"))
			continue
		}
		err = m.accessService.UpdatePermission(ctx, access.UpdatePermissionArgs{
			AccessSpec: permission.AccessSpec{
				Target: permission.ID{
					ObjectType: permission.Model,
					Key:        modelTag.Id(),
				},
				Access: modelAccess,
			},
			Change:  permission.AccessChange(arg.Action),
			Subject: user.NameFromTag(targetUserTag),
		})

		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ModelDefaultsForClouds returns the default config values for the specified
// clouds.
func (m *ModelManagerAPI) ModelDefaultsForClouds(ctx context.Context, args params.Entities) (params.ModelDefaultsResults, error) {
	result := params.ModelDefaultsResults{}
	if !m.isAdmin {
		return result, apiservererrors.ErrPerm
	}
	result.Results = make([]params.ModelDefaultsResult, len(args.Entities))
	for i, entity := range args.Entities {
		cloudTag, err := names.ParseCloudTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = m.modelDefaults(cloudTag.Id())
	}
	return result, nil
}

func (m *ModelManagerAPI) modelDefaults(cloud string) params.ModelDefaultsResult {
	result := params.ModelDefaultsResult{}
	values, err := m.ctlrState.ModelConfigDefaultValues(cloud)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	result.Config = make(map[string]params.ModelDefaults)
	for attr, val := range values {
		settings := params.ModelDefaults{
			Controller: val.Controller,
			Default:    val.Default,
		}
		for _, v := range val.Regions {
			settings.Regions = append(
				settings.Regions, params.RegionDefaults{
					RegionName: v.Name,
					Value:      v.Value})
		}
		result.Config[attr] = settings
	}
	return result
}

// SetModelDefaults writes new values for the specified default model settings.
func (m *ModelManagerAPI) SetModelDefaults(ctx context.Context, args params.SetModelDefaults) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Config))}
	if err := m.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Trace(err)
	}
	for i, arg := range args.Config {
		results.Results[i].Error = apiservererrors.ServerError(
			m.setModelDefaults(ctx, arg),
		)
	}
	return results, nil
}

func (m *ModelManagerAPI) setModelDefaults(ctx context.Context, args params.ModelDefaultValues) error {
	if !m.isAdmin {
		return apiservererrors.ErrPerm
	}

	if err := m.check.ChangeAllowed(ctx); err != nil {
		return errors.Trace(err)
	}
	// Make sure we don't allow changing agent-version.
	if _, found := args.Config["agent-version"]; found {
		return errors.New("agent-version cannot have a default value")
	}

	var rspec *environscloudspec.CloudRegionSpec
	if args.CloudTag != "" {
		spec, err := m.makeRegionSpec(args.CloudTag, args.CloudRegion)
		if err != nil {
			return errors.Trace(err)
		}
		rspec = spec
	}
	return m.ctlrState.UpdateModelConfigDefaultValues(args.Config, nil, rspec)
}

// UnsetModelDefaults removes the specified default model settings.
func (m *ModelManagerAPI) UnsetModelDefaults(ctx context.Context, args params.UnsetModelDefaults) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Keys))}
	if !m.isAdmin {
		return results, apiservererrors.ErrPerm
	}

	if err := m.check.ChangeAllowed(ctx); err != nil {
		return results, errors.Trace(err)
	}

	for i, arg := range args.Keys {
		var rspec *environscloudspec.CloudRegionSpec
		if arg.CloudTag != "" {
			spec, err := m.makeRegionSpec(arg.CloudTag, arg.CloudRegion)
			if err != nil {
				results.Results[i].Error = apiservererrors.ServerError(
					errors.Trace(err))
				continue
			}
			rspec = spec
		}
		results.Results[i].Error = apiservererrors.ServerError(
			m.ctlrState.UpdateModelConfigDefaultValues(nil, arg.Keys, rspec),
		)
	}
	return results, nil
}

// makeRegionSpec is a helper method for methods that call
// state.UpdateModelConfigDefaultValues.
func (m *ModelManagerAPI) makeRegionSpec(cloudTag, r string) (*environscloudspec.CloudRegionSpec, error) {
	cTag, err := names.ParseCloudTag(cloudTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rspec, err := environscloudspec.NewCloudRegionSpec(cTag.Id(), r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return rspec, nil
}

// ChangeModelCredential changes cloud credential reference for models.
// These new cloud credentials must already exist on the controller.
func (m *ModelManagerAPI) ChangeModelCredential(ctx context.Context, args params.ChangeModelCredentialsParams) (params.ErrorResults, error) {
	if err := m.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	err := m.authorizer.HasPermission(ctx, permission.SuperuserAccess, m.state.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return params.ErrorResults{}, errors.Trace(err)
	}
	controllerAdmin := err == nil
	// Only controller or model admin can change cloud credential on a model.
	checkModelAccess := func(tag names.ModelTag) error {
		if controllerAdmin {
			return nil
		}
		return m.authorizer.HasPermission(ctx, permission.AdminAccess, tag)
	}

	replaceModelCredential := func(arg params.ChangeModelCredentialParams) error {
		modelTag, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			return errors.Trace(err)
		}
		if err := checkModelAccess(modelTag); err != nil {
			return errors.Trace(err)
		}
		credentialTag, err := names.ParseCloudCredentialTag(arg.CloudCredentialTag)
		if err != nil {
			return errors.Trace(err)
		}
		model, releaser, err := m.state.GetModel(modelTag.Id())
		if err != nil {
			return errors.Trace(err)
		}
		defer releaser()

		updated, err := model.SetCloudCredential(credentialTag)
		if err != nil {
			return errors.Trace(err)
		}
		if !updated {
			return errors.Errorf("model %v already uses credential %v", modelTag.Id(), credentialTag.Id())
		}
		return nil
	}

	results := make([]params.ErrorResult, len(args.Models))
	for i, arg := range args.Models {
		if err := replaceModelCredential(arg); err != nil {
			results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return params.ErrorResults{Results: results}, nil
}

// isModelAdmin checks if the user is a controller superuser or admin on the
// model.
func (m *ModelManagerAPI) isModelAdmin(ctx context.Context, modelTag names.ModelTag) bool {
	if m.isAdmin {
		return true
	}
	return m.authorizer.HasPermission(ctx, permission.AdminAccess, modelTag) == nil
}

// checkReadModelPermission checks if the user has controller superuser
// permissions or at least read permissions on the model.
func (m *ModelManagerAPI) checkReadModelPermission(ctx context.Context, modelUUID coremodel.UUID, name user.Name) (bool, error) {
	if m.isAdmin {
		return true, nil
	}
	target := permission.ID{
		ObjectType: permission.Model,
		Key:        modelUUID.String(),
	}
	perm, err := m.accessService.ReadUserAccessLevelForTarget(ctx, name, target)
	if err != nil && !errors.Is(err, accesserrors.AccessNotFound) {
		return false, errors.Trace(err)
	}
	if !perm.EqualOrGreaterModelAccessThan(permission.ReadAccess) {
		return false, nil
	}
	return true, nil
}
