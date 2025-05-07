// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs/config"
	internalerrors "github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var (
	logger = internallogger.GetLogger("juju.apiserver.modelmanager")
)

// StateBackend represents the mongo backend.
type StateBackend interface {
	GetBackend(string) (commonmodel.ModelManagerBackend, func() bool, error)
	NewModel(state.ModelArgs) (commonmodel.Model, commonmodel.ModelManagerBackend, error)
}

// ModelStatusAPI is the interface for the model status API.
type ModelStatusAPI interface {
	ModelStatus(ctx context.Context, req params.Entities) (params.ModelStatusResults, error)
}

// ModelManagerAPI implements the model manager interface and is
// the concrete implementation of the api end point.
type ModelManagerAPI struct {
	ModelStatusAPI

	// Access control.
	authorizer facade.Authorizer
	isAdmin    bool
	apiUser    names.UserTag

	// Legacy state access.
	state StateBackend

	check common.BlockCheckerInterface

	// Services required by the model manager.
	accessService        AccessService
	domainServicesGetter DomainServicesGetter
	applicationService   ApplicationService
	credentialService    CredentialService
	modelService         ModelService
	modelDefaultsService ModelDefaultsService
	secretBackendService SecretBackendService

	modelExporter func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (ModelExporter, error)
	store         objectstore.ObjectStore

	controllerUUID uuid.UUID
}

// NewModelManagerAPI creates a new api server endpoint for managing
// models.
func NewModelManagerAPI(
	ctx context.Context,
	st StateBackend,
	isAdmin bool,
	apiUser names.UserTag,
	modelStatusAPI ModelStatusAPI,
	modelExporter func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (ModelExporter, error),
	controllerUUID uuid.UUID,
	services Services,
	blockChecker common.BlockCheckerInterface,
	authorizer facade.Authorizer,
) *ModelManagerAPI {

	return &ModelManagerAPI{
		// TODO: remove mongo state from ModelStatusAPI entirely.
		ModelStatusAPI:       modelStatusAPI,
		state:                st,
		domainServicesGetter: services.DomainServicesGetter,
		modelExporter:        modelExporter,
		credentialService:    services.CredentialService,
		applicationService:   services.ApplicationService,
		store:                services.ObjectStore,
		check:                blockChecker,
		authorizer:           authorizer,
		apiUser:              apiUser,
		isAdmin:              isAdmin,
		modelService:         services.ModelService,
		modelDefaultsService: services.ModelDefaultsService,
		accessService:        services.AccessService,
		secretBackendService: services.SecretBackendService,
		controllerUUID:       controllerUUID,
	}
}

// authCheck checks if the user is acting on their own behalf, or if they
// are an administrator acting on behalf of another user.
func (m *ModelManagerAPI) authCheck(ctx context.Context, user names.UserTag) error {
	if m.isAdmin {
		logger.Tracef(ctx, "%q is a controller admin", m.apiUser.Id())
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

// ConfigSource describes a type that is able to provide config.
// Abstracted primarily for testing.
type ConfigSource interface {
	Config() (*config.Config, error)
}

func (m *ModelManagerAPI) checkAddModelPermission(ctx context.Context, cloudTag names.CloudTag) (bool, error) {
	if err := m.authorizer.HasPermission(ctx, permission.AddModelAccess, cloudTag); !m.isAdmin && err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

// reloadSpaces wraps the call to ReloadSpaces and its returned errors.
func reloadSpaces(ctx context.Context, modelNetworkService NetworkService) error {
	if err := modelNetworkService.ReloadSpaces(ctx); err != nil {
		if errors.Is(err, errors.NotSupported) {
			logger.Debugf(ctx, "Not performing spaces load on a non-networking environment")
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

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	// We need to get the controller's default cloud and credential. To help
	// Juju users when creating their first models we allow them to omit this
	// information from the model creation args. If they have done exactly this
	// we will try and apply the defaults where authorisation allows us to.
	defaultCloudName, defaultCloudRegion, err := m.modelService.DefaultModelCloudInfo(ctx)
	if err != nil {
		return result, errors.New("cannot find default model cloud and credential")
	}

	var cloudTag names.CloudTag
	if args.CloudTag != "" {
		var err error
		cloudTag, err = names.ParseCloudTag(args.CloudTag)
		if err != nil {
			return result, errors.Trace(err)
		}
	} else {
		cloudTag = names.NewCloudTag(defaultCloudName)
	}
	// We only set a cloud region default if the user has not supplied one and
	// the cloud to use is the same as that of defaultCloudName.
	if args.CloudRegion == "" && cloudTag.Id() == defaultCloudName {
		args.CloudRegion = defaultCloudRegion
	}

	if !m.isAdmin {
		canAddModel, err := m.checkAddModelPermission(ctx, cloudTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		if !canAddModel {
			return result, apiservererrors.ErrPerm
		}

		// a special case of ErrPerm will happen if the user has add-model permission but is trying to
		// create a model for another person, which is not yet supported.
		if ownerTag != m.apiUser {
			return result, internalerrors.Errorf(
				"%q permission does not permit creation of models for different owners",
				permission.AddModelAccess,
			).Add(apiservererrors.ErrPerm)
		}
	}

	// TODO (stickupkid): We need to create a saga (pattern) coordinator here,
	// to ensure that anything written to both databases are at least rollback
	// if there was an error. If a failure to rollback occurs, then the endpoint
	// should at least be somewhat idempotent.
	creationArgs := model.GlobalModelCreationArgs{
		CloudRegion: args.CloudRegion,
		Name:        args.Name,
		Cloud:       cloudTag.Id(),
	}
	if args.CloudCredentialTag != "" {
		cloudCredentialTag, err := names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		creationArgs.Credential = credential.KeyFromTag(cloudCredentialTag)
	}

	user, err := m.accessService.GetUserByName(ctx, user.NameFromTag(ownerTag))
	switch {
	case errors.Is(err, accesserrors.UserNotFound):
		return result, internalerrors.Errorf(
			"owner %q for model does not exist", ownerTag.Name(),
		).Add(coreerrors.NotFound)
	case errors.Is(err, accesserrors.UserNameNotValid):
		return result, internalerrors.New(
			"cannot create model with invalid owner",
		).Add(coreerrors.NotValid)
	case err != nil:
		return result, internalerrors.Errorf(
			"retrieving user %q for new model %q owner: %w",
			ownerTag.Name(), args.Name, err,
		)
	}
	creationArgs.Owner = user.UUID

	// Create the model in the controller database.
	modelUUID, activator, err := m.modelService.CreateModel(ctx, creationArgs)
	switch {
	case errors.Is(err, modelerrors.AlreadyExists):
		return result, internalerrors.Errorf(
			"model already exists for name %q and owner %q", args.Name, ownerTag.Name(),
		).Add(coreerrors.AlreadyExists)
	case errors.Is(err, modelerrors.CredentialNotValid):
		return result, internalerrors.Errorf(
			"cloud credential for new model is not valid",
		).Add(coreerrors.NotFound)
	case err != nil:
		return result, internalerrors.Errorf(
			"creating new model %q for owner %q: %w",
			args.Name, ownerTag.Name(), err,
		)
	}

	// We use the returned model UUID as we can guarantee that's the one that
	// was written to the database.
	modelDomainServices, err := m.domainServicesGetter.DomainServicesForModel(ctx, modelUUID)
	if err != nil {
		return result, errors.Trace(err)
	}

	// Create the model information in the model database.
	// modelInfoCreate will be calling one of the Create* funcs on the model
	// info service. When handling the error we need to handle the total set of
	// possabilities.
	err = m.createModelInfo(ctx, args.Config, modelDomainServices.ModelInfo())
	switch {
	case errors.Is(err, modelerrors.AlreadyExists):
		return result, apiservererrors.ParamsErrorf(
			params.CodeAlreadyExists,
			"model %q for owner %q already exists in model database",
			creationArgs.Name, ownerTag.Name(),
		)
	case errors.Is(err, modelerrors.AgentVersionNotSupported):
		return result, apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"supplied agent version for new model %q is not supported: %s",
			creationArgs.Name, err.Error(),
		)
	case errors.Is(err, coreerrors.NotValid):
		return result, apiservererrors.ParamsErrorf(
			params.CodeNotValid,
			"supplied agent stream for new model %q is not supported: %s",
			creationArgs.Name, err.Error(),
		)
	case err != nil:
		return result, internalerrors.Errorf(
			"creating information records for new model %q: %w",
			creationArgs.Name, err,
		)
	}

	modelConfigService := modelDomainServices.Config()
	if err := modelConfigService.SetModelConfig(ctx, args.Config); err != nil {
		return result, errors.Annotatef(err, "setting model config for model %q", creationArgs.Name)
	}

	if err := activator(ctx); err != nil {
		return result, errors.Annotatef(err, "finalising model %q", creationArgs.Name)
	}

	// Reload the substrate spaces for the newly created model.
	if err := reloadSpaces(ctx, modelDomainServices.Network()); err != nil {
		return result, errors.Annotatef(err, "reloading spaces for model %q", creationArgs.Name)
	}

	newConfig, err := modelDomainServices.Config().ModelConfig(ctx)
	if err != nil {
		return result, errors.Annotatef(err, "getting config for %q", creationArgs.Name)
	}

	modelInfo, err := m.getModelInfo(ctx, modelUUID)
	if err != nil {
		return result, err
	}

	cloudTag, err = names.ParseCloudTag(modelInfo.CloudTag)
	if err != nil {
		return result, errors.Trace(err)
	}
	credentialTag, err := names.ParseCloudCredentialTag(modelInfo.CloudCredentialTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	// TODO: remove model creation from the mongo state.
	_, st, err := m.state.NewModel(state.ModelArgs{
		Type:            state.ModelType(modelInfo.Type),
		CloudName:       cloudTag.Id(),
		CloudRegion:     modelInfo.CloudRegion,
		CloudCredential: credentialTag,
		Config:          newConfig,
		Owner:           ownerTag,
	})
	if err != nil {
		return result, errors.Annotatef(err, "creating new model for %q", creationArgs.Name)
	}
	defer st.Close()

	return modelInfo, nil
}

// createModelInfo establishes a new model within the model database.
// This is required when creating new models as it seeds the model's controller
// information to the model database and also establishes any provider resources
// required by the model.
func (m *ModelManagerAPI) createModelInfo(
	ctx context.Context,
	configArgs map[string]any,
	modelInfoService ModelInfoService,
) error {
	suppliedAgentVersion := semversion.Zero
	if agentVersionVal, exists := configArgs[config.AgentVersionKey]; exists {
		agentVersionStr, isStr := agentVersionVal.(string)
		if !isStr {
			return internalerrors.New(
				"cannot understand agent version value for new model",
			).Add(coreerrors.NotValid)
		}
		var err error
		suppliedAgentVersion, err = semversion.Parse(agentVersionStr)
		if err != nil {
			return internalerrors.Errorf(
				"parsing agent version value for new model: %w", err,
			)
		}
		delete(configArgs, config.AgentVersionKey)
	}

	suppliedAgentStream := coreagentbinary.AgentStream("")
	if agentStreamVal, exists := configArgs[config.AgentStreamKey]; exists {
		agentStreamStr, isStr := agentStreamVal.(string)
		if !isStr {
			return internalerrors.New(
				"cannot understand agent stream value for new model",
			).Add(coreerrors.NotValid)
		}
		suppliedAgentStream = coreagentbinary.AgentStream(agentStreamStr)
		delete(configArgs, config.AgentStreamKey)
	}

	// If the user has supplied both a target agent version and agent stream
	if suppliedAgentVersion != semversion.Zero &&
		suppliedAgentStream != coreagentbinary.AgentStream("") {
		return modelInfoService.CreateModelWithAgentVersionStream(
			ctx, suppliedAgentVersion, suppliedAgentStream,
		)
	}

	// If the user has supplied a target agent version but no agent stream
	if suppliedAgentVersion != semversion.Zero &&
		suppliedAgentStream == coreagentbinary.AgentStream("") {
		return modelInfoService.CreateModelWithAgentVersion(
			ctx, suppliedAgentVersion,
		)
	}

	// If the user has supplied an agent stream and not target agent version.
	if suppliedAgentVersion == semversion.Zero &&
		suppliedAgentStream != coreagentbinary.AgentStream("") {
		// TODO: We don't have a way to set just the agent stream.
	}

	// If the user has supplied nothing.
	return modelInfoService.CreateModel(ctx)
}

func (m *ModelManagerAPI) dumpModel(ctx context.Context, args params.Entity) ([]byte, error) {
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
	// TODO: remove mongo state from the mode exporter.
	modelExporter, err := m.modelExporter(ctx, coremodel.UUID(modelTag.Id()), modelState)
	if err != nil {
		return nil, errors.Trace(err)
	}

	model, err := modelExporter.ExportModelPartial(ctx, exportConfig, m.store)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bytes, err := description.Serialize(model)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bytes, nil
}

// DumpModels will export the models into the database agnostic
// representation. The user needs to either be a controller admin, or have
// admin privileges on the model itself.
func (m *ModelManagerAPI) DumpModels(ctx context.Context, args params.DumpModelRequest) params.StringResults {
	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		if args.Simplified {
			results.Results[i].Error = apiservererrors.ServerError(errors.NotSupportedf("simplified model dump"))
			continue
		}

		bytes, err := m.dumpModel(ctx, entity)
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
	for i := range args.Entities {
		results.Results[i].Error = apiservererrors.ServerError(errors.NotImplementedf("DumpModelsDB"))
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

	err = m.authCheck(ctx, userTag)
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
		summary, err := m.makeModelSummary(mi)
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
		summary, err := m.makeUserModelSummary(mi)
		if err != nil {
			result.Results[i] = params.ModelSummaryResult{Error: apiservererrors.ServerError(err)}
		} else {
			result.Results[i] = params.ModelSummaryResult{Result: summary}
		}
	}
	return result, nil
}

func (m *ModelManagerAPI) makeUserModelSummary(mi coremodel.UserModelSummary) (*params.ModelSummary, error) {
	userAccess, err := commonmodel.EncodeAccess(mi.UserAccess)
	if err != nil && !errors.Is(err, errors.NotValid) {
		return nil, errors.Trace(err)
	}
	ms, err := m.makeModelSummary(mi.ModelSummary)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ms.UserAccess = userAccess
	ms.UserLastConnection = mi.UserLastConnection
	return ms, nil
}

func (m *ModelManagerAPI) makeModelSummary(mi coremodel.ModelSummary) (*params.ModelSummary, error) {
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

		Status: params.EntityStatus{
			Status: mi.Status.Status,
			Info:   mi.Status.Message,
			Data:   mi.Status.Data,
			Since:  mi.Status.Since,
		},
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

	err = m.authCheck(ctx, userTag)
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
		modelTag, err := names.ParseModelTag(modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		if !m.isAdmin {
			if err := m.authorizer.HasPermission(ctx, permission.AdminAccess, modelTag); err != nil {
				return err
			}
		}

		domainServices, err := m.domainServicesGetter.DomainServicesForModel(ctx, coremodel.UUID(modelUUID))
		if err != nil {
			return errors.Trace(err)
		}

		st, releaseSt, err := m.state.GetBackend(modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		defer releaseSt()

		err = commonmodel.DestroyModel(
			ctx, st, // TODO: remove mongo state from the commonmodel.DestroyModel.
			domainServices.BlockCommand(), domainServices.ModelInfo(),
			destroyStorage, force, maxWait, timeout,
		)
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

			err = m.modelService.DeleteModel(ctx, coremodel.UUID(modelUUID))
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

	checkWritePermission := func(tag names.ModelTag) bool {
		if m.isAdmin {
			return true
		}
		if err := m.authorizer.HasPermission(ctx, permission.AdminAccess, tag); err == nil {
			return true
		}
		if err := m.authorizer.HasPermission(ctx, permission.WriteAccess, tag); err == nil {
			return true
		}
		return false
	}

	getModelInfo := func(arg params.Entity) (params.ModelInfo, error) {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			return params.ModelInfo{}, errors.Trace(err)
		}
		canWrite := checkWritePermission(tag)
		if !canWrite {
			// If the logged in user does not have at least read permission, we return an error.
			if err := m.authorizer.HasPermission(ctx, permission.WriteAccess, tag); err != nil {
				return params.ModelInfo{}, errors.Trace(apiservererrors.ErrPerm)
			}
		}

		modelInfo, err := m.getModelInfo(ctx, coremodel.UUID(tag.Id()))
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
		if canWrite {
			st, release, err := m.state.GetBackend(tag.Id())
			if errors.Is(err, errors.NotFound) {
				return params.ModelInfo{}, errors.Trace(apiservererrors.ErrPerm)
			} else if err != nil {
				return params.ModelInfo{}, errors.Trace(err)
			}
			defer release()

			modelUUID := coremodel.UUID(tag.Id())
			modelDomainServices, err := m.domainServicesGetter.DomainServicesForModel(ctx, modelUUID)
			if err != nil {
				return params.ModelInfo{}, errors.Trace(err)
			}
			// TODO: remove mongo state from the commonmodel.ModelMachineInfo.
			if modelInfo.Machines, err = commonmodel.ModelMachineInfo(ctx, st, modelDomainServices.Machine()); err != nil {
				return params.ModelInfo{}, err
			}

			backends, err := m.secretBackendService.BackendSummaryInfoForModel(ctx, modelUUID)
			if err != nil {
				return params.ModelInfo{}, errors.Trace(err)
			}
			for _, backend := range backends {
				name := backend.Name
				if name == kubernetes.BackendName {
					name = kubernetes.BuiltInName(modelInfo.Name)
				}
				modelInfo.SecretBackends = append(modelInfo.SecretBackends, params.SecretBackendResult{
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

func (m *ModelManagerAPI) getModelInfo(ctx context.Context, modelUUID coremodel.UUID) (params.ModelInfo, error) {
	modelTag := names.NewModelTag(modelUUID.String())

	modelDomainServices, err := m.domainServicesGetter.DomainServicesForModel(ctx, modelUUID)
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	modelInfoService := modelDomainServices.ModelInfo()
	modelInfo, err := modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}

	model, err := m.modelService.Model(ctx, modelUUID)
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}

	// At this point, if the user does not have write access, they must have
	// read access otherwise we would've returned on the initial check at the
	// beginning of this method.

	info := params.ModelInfo{
		Name:           model.Name,
		Type:           model.ModelType.String(),
		UUID:           modelUUID.String(),
		ControllerUUID: m.controllerUUID.String(),
		IsController:   modelInfo.IsControllerModel,
		OwnerTag:       names.NewUserTag(model.OwnerName.Name()).String(),
		Life:           model.Life,
		CloudTag:       names.NewCloudTag(model.Cloud).String(),
		CloudRegion:    model.CloudRegion,
		ProviderType:   model.CloudType,
	}

	if cloudCredentialTag, err := model.Credential.Tag(); err == nil {
		info.CloudCredentialTag = cloudCredentialTag.String()
	}

	modelAgentService := modelDomainServices.Agent()
	agentVersion, err := modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	info.AgentVersion = &agentVersion

	status, err := modelInfoService.GetStatus(ctx)
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	// Translate domain model status to params entity status. We put reason
	// into the data map as this is where the contract to the client expects
	// this value at the moment.
	info.Status = params.EntityStatus{
		Status: status.Status,
		Info:   status.Message,
		Data: map[string]interface{}{
			"reason": status.Reason,
		},
		Since: &status.Since,
	}

	if status.Status == corestatus.Busy {
		info.Migration = &params.ModelMigrationStatus{
			Status: status.Message,
			Start:  &status.Since,
		}
	}

	info.Users, err = commonmodel.ModelUserInfo(ctx, m.modelService, modelTag, user.NameFromTag(m.apiUser), m.isAdmin)
	if err != nil {
		return params.ModelInfo{}, errors.Annotate(err, "getting model user info")
	}

	fs, err := m.applicationService.GetSupportedFeatures(ctx)
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
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

	err := m.authorizer.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(m.controllerUUID.String()))
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
		result.Results[i] = m.modelDefaults(ctx, cloudTag.Id())
	}
	return result, nil
}

func (m *ModelManagerAPI) modelDefaults(ctx context.Context, cloud string) params.ModelDefaultsResult {
	result := params.ModelDefaultsResult{}
	modelDefaults, err := m.modelDefaultsService.CloudDefaults(ctx, cloud)
	if err != nil {
		if errors.Is(err, clouderrors.NotFound) {
			err = errors.NotFoundf("cloud %q", cloud)
		}
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	result.Config = make(map[string]params.ModelDefaults, len(modelDefaults))
	for attr, val := range modelDefaults {
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
	if !m.isAdmin {
		return results, apiservererrors.ErrPerm
	}

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
	if args.CloudTag == "" {
		return errors.New("missing cloud name")
	}
	cTag, err := names.ParseCloudTag(args.CloudTag)
	if err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("cloud tag %q not valid", args.CloudTag))
	}
	if args.CloudRegion == "" {
		err := m.modelDefaultsService.UpdateCloudDefaults(ctx, cTag.Id(), args.Config)
		if errors.Is(err, clouderrors.NotFound) {
			return errors.NotFoundf("cloud %q", cTag.Id())
		}
		return err
	}

	err = m.modelDefaultsService.UpdateCloudRegionDefaults(ctx, cTag.Id(), args.CloudRegion, args.Config)
	if errors.Is(err, clouderrors.NotFound) {
		return errors.NotFoundf("cloud %q region %q", cTag.Id(), args.CloudRegion)
	}
	return err
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
		results.Results[i].Error = apiservererrors.ServerError(
			m.unsetModelDefaults(ctx, arg),
		)
	}
	return results, nil
}

func (m *ModelManagerAPI) unsetModelDefaults(ctx context.Context, arg params.ModelUnsetKeys) error {
	if arg.CloudTag == "" {
		return errors.New("missing cloud name")
	}

	cTag, err := names.ParseCloudTag(arg.CloudTag)
	if err != nil {
		return errors.NewNotValid(err, fmt.Sprintf("cloud tag %q not valid", arg.CloudTag))
	}
	if arg.CloudRegion == "" {
		err := m.modelDefaultsService.RemoveCloudDefaults(ctx, cTag.Id(), arg.Keys)
		if errors.Is(err, clouderrors.NotFound) {
			return errors.NotFoundf("cloud %q", cTag.Id())
		}
		return err
	}
	err = m.modelDefaultsService.RemoveCloudRegionDefaults(ctx, cTag.Id(), arg.CloudRegion, arg.Keys)
	if errors.Is(err, clouderrors.NotFound) {
		return errors.NotFoundf("cloud %q region %q", cTag.Id(), arg.CloudRegion)
	}
	return err
}

// ChangeModelCredential changes cloud credential reference for models.
// These new cloud credentials must already exist on the controller.
func (m *ModelManagerAPI) ChangeModelCredential(ctx context.Context, args params.ChangeModelCredentialsParams) (params.ErrorResults, error) {
	if err := m.check.ChangeAllowed(ctx); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	err := m.authorizer.HasPermission(ctx, permission.SuperuserAccess, names.NewControllerTag(m.controllerUUID.String()))
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
		credentialKey := credential.KeyFromTag(credentialTag)
		if err := m.modelService.UpdateCredential(ctx, coremodel.UUID(modelTag.Id()), credentialKey); err != nil {
			return errors.Trace(err)
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
