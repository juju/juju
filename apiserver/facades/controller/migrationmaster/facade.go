// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"
	"encoding/json"

	"github.com/juju/collections/set"
	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/naturalsort"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/core/leadership"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/rpc/params"
)

// ModelExporter exports a model to a description.Model.
type ModelExporter interface {
	// ExportModel exports a model to a description.Model.
	// It requires a known set of leaders to be passed in, so that applications
	// can have their leader set correctly once imported.
	// The objectstore is used to retrieve charms and resources for export.
	ExportModel(context.Context, objectstore.ObjectStore) (description.Model, error)
}

// APIV4 implements the API V4.
type APIV4 struct {
	*API
}

// API implements the API required for the model migration
// master worker.
type API struct {
	modelExporter               ModelExporter
	authorizer                  facade.Authorizer
	watcherRegistry             facade.WatcherRegistry
	leadership                  leadership.Reader
	modelMigrationServiceGetter func(context.Context, coremodel.UUID) (ModelMigrationService, error)
	credentialServiceGetter     func(context.Context, coremodel.UUID) (CredentialService, error)
	upgradeServiceGetter        func(context.Context, coremodel.UUID) (UpgradeService, error)
	applicationServiceGetter    func(context.Context, coremodel.UUID) (ApplicationService, error)
	relationServiceGetter       func(context.Context, coremodel.UUID) (RelationService, error)
	statusServiceGetter         func(context.Context, coremodel.UUID) (StatusService, error)
	modelAgentServiceGetter     func(context.Context, coremodel.UUID) (ModelAgentService, error)
	machineServiceGetter        func(context.Context, coremodel.UUID) (MachineService, error)
	controllerConfigService     ControllerConfigService
	controllerNodeService       ControllerNodeService
	modelInfoService            ModelInfoService
	modelService                ModelService
	modelMigrationService       ModelMigrationService
	store                       objectstore.ObjectStore
	controllerModelUUID         coremodel.UUID
}

// NewAPI creates a new API server endpoint for the model migration
// master worker.
func NewAPI(
	modelExporter ModelExporter,
	store objectstore.ObjectStore,
	controllerModelUUID coremodel.UUID,
	watcherRegistry facade.WatcherRegistry,
	authorizer facade.Authorizer,
	leadership leadership.Reader,
	modelMigrationServiceGetter func(context.Context, coremodel.UUID) (ModelMigrationService, error),
	credentialServiceGetter func(context.Context, coremodel.UUID) (CredentialService, error),
	upgradeServiceGetter func(context.Context, coremodel.UUID) (UpgradeService, error),
	applicationServiceGetter func(context.Context, coremodel.UUID) (ApplicationService, error),
	relationServiceGetter func(context.Context, coremodel.UUID) (RelationService, error),
	statusServiceGetter func(context.Context, coremodel.UUID) (StatusService, error),
	modelAgentServiceGetter func(context.Context, coremodel.UUID) (ModelAgentService, error),
	machineServiceGetter func(context.Context, coremodel.UUID) (MachineService, error),
	controllerConfigService ControllerConfigService,
	controllerNodeService ControllerNodeService,
	modelInfoService ModelInfoService,
	modelService ModelService,
	modelMigrationService ModelMigrationService,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		modelExporter:               modelExporter,
		store:                       store,
		controllerModelUUID:         controllerModelUUID,
		authorizer:                  authorizer,
		watcherRegistry:             watcherRegistry,
		leadership:                  leadership,
		modelMigrationServiceGetter: modelMigrationServiceGetter,
		credentialServiceGetter:     credentialServiceGetter,
		upgradeServiceGetter:        upgradeServiceGetter,
		applicationServiceGetter:    applicationServiceGetter,
		relationServiceGetter:       relationServiceGetter,
		statusServiceGetter:         statusServiceGetter,
		modelAgentServiceGetter:     modelAgentServiceGetter,
		machineServiceGetter:        machineServiceGetter,
		controllerConfigService:     controllerConfigService,
		controllerNodeService:       controllerNodeService,
		modelInfoService:            modelInfoService,
		modelService:                modelService,
		modelMigrationService:       modelMigrationService,
	}, nil
}

// Watch starts watching for an active migration for the model
// associated with the API connection. The returned id should be used
// with the NotifyWatcher facade to receive events.
func (api *API) Watch(ctx context.Context) params.NotifyWatchResult {
	result := params.NotifyWatchResult{}

	w, err := api.modelMigrationService.WatchForMigration(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, w)
	result.Error = apiservererrors.ServerError(err)
	return result
}

// MigrationStatus returns the details and progress of the latest
// model migration.
func (api *API) MigrationStatus(ctx context.Context) (params.MasterMigrationStatus, error) {
	empty := params.MasterMigrationStatus{}

	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return empty, errors.Trace(err)
	}

	migrationInfo, err := api.modelMigrationService.Migration(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "retrieving model migration")
	}
	if migrationInfo.Phase == coremigration.NONE {
		return empty, errors.NotFoundf("migration")
	}

	target := migrationInfo.Target
	macsJSON, err := json.Marshal(target.Macaroons)
	if err != nil {
		return empty, errors.Annotate(err, "marshalling macaroons")
	}
	return params.MasterMigrationStatus{
		Spec: params.MigrationSpec{
			ModelTag: names.NewModelTag(modelInfo.UUID.String()).String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(target.ControllerUUID).String(),
				Addrs:         target.Addrs,
				CACert:        target.CACert,
				AuthTag:       names.NewUserTag(target.User).String(),
				Password:      target.Password,
				Macaroons:     string(macsJSON),
				Token:         target.Token,
			},
		},
		MigrationId:      migrationInfo.UUID,
		Phase:            migrationInfo.Phase.String(),
		PhaseChangedTime: migrationInfo.PhaseChangedTime,
	}, nil
}

// ModelInfo returns essential information about the model to be
// migrated.
func (api *API) ModelInfo(ctx context.Context) (params.MigrationModelInfo, error) {
	empty := params.MigrationModelInfo{}

	modelInfo, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "retrieving model info")
	}

	model, err := api.modelExporter.ExportModel(ctx, api.store)
	if err != nil {
		return empty, errors.Annotate(err, "retrieving model")
	}

	modelDescription, err := description.Serialize(model)
	if err != nil {
		return empty, errors.Annotate(err, "serializing model")
	}

	return params.MigrationModelInfo{
		UUID:             modelInfo.UUID.String(),
		Name:             modelInfo.Name,
		Qualifier:        modelInfo.Qualifier.String(),
		AgentVersion:     modelInfo.AgentVersion,
		ModelDescription: modelDescription,
	}, nil
}

// SourceControllerInfo returns the details required to connect to
// the source controller for model migration.
func (api *API) SourceControllerInfo(ctx context.Context) (params.MigrationSourceInfo, error) {
	empty := params.MigrationSourceInfo{}

	cfg, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return empty, errors.Annotate(err, "retrieving controller config")
	}
	cacert, _ := cfg.CACert()

	apiAddresses, err := api.controllerNodeService.GetAllAPIAddressesForClients(ctx)
	if err != nil {
		return empty, errors.Trace(err)
	}

	return params.MigrationSourceInfo{
		ControllerTag:   names.NewControllerTag(cfg.ControllerUUID()).String(),
		ControllerAlias: cfg.ControllerName(),
		Addrs:           apiAddresses,
		CACert:          cacert,
	}, nil
}

// SetPhase sets the phase of the active model migration. The provided
// phase must be a valid phase value, for example QUIESCE" or
// "ABORT". See the core/migration package for the complete list.
func (api *API) SetPhase(ctx context.Context, args params.SetMigrationPhaseArgs) error {
	phase, ok := coremigration.ParsePhase(args.Phase)
	if !ok {
		return errors.Errorf("invalid phase: %q", args.Phase)
	}
	err := api.modelMigrationService.SetMigrationPhase(ctx, phase)
	if err != nil {
		return errors.Annotate(err, "failed to set phase")
	}
	return nil
}

// Prechecks performs pre-migration checks on the model and
// (source) controller.
func (api *API) Prechecks(ctx context.Context, arg params.PrechecksArgs) error {
	// Check the model exists, this can be moved into the migration service
	// code, but for now keep it here.
	model, err := api.modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return errors.Annotate(err, "retrieving model info")
	}

	modelMigrationServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.ModelMigrationService, error) {
		return api.modelMigrationServiceGetter(ctx, modelUUID)
	}
	credentialServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.CredentialService, error) {
		return api.credentialServiceGetter(ctx, modelUUID)
	}
	upgradeServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.UpgradeService, error) {
		return api.upgradeServiceGetter(ctx, modelUUID)
	}
	applicationServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.ApplicationService, error) {
		return api.applicationServiceGetter(ctx, modelUUID)
	}
	relationServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.RelationService, error) {
		return api.relationServiceGetter(ctx, modelUUID)
	}
	statusServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.StatusService, error) {
		return api.statusServiceGetter(ctx, modelUUID)
	}
	modelAgentServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.ModelAgentService, error) {
		return api.modelAgentServiceGetter(ctx, modelUUID)
	}
	machineServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.MachineService, error) {
		return api.machineServiceGetter(ctx, modelUUID)
	}

	return migration.SourcePrecheck(
		ctx,
		model.UUID,
		api.controllerModelUUID,
		api.modelService,
		modelMigrationServiceGetterShim,
		credentialServiceGetterShim,
		upgradeServiceGetterShim,
		applicationServiceGetterShim,
		relationServiceGetterShim,
		statusServiceGetterShim,
		modelAgentServiceGetterShim,
		machineServiceGetterShim,
	)
}

// SetStatusMessage sets a human readable status message containing
// information about the migration's progress. This will be shown in
// status output shown to the end user.
func (api *API) SetStatusMessage(ctx context.Context, args params.SetMigrationStatusMessageArgs) error {
	err := api.modelMigrationService.SetMigrationStatusMessage(ctx, args.Message)
	if err != nil {
		return errors.Annotate(err, "failed to set status message")
	}
	return nil
}

// Export serializes the model associated with the API connection.
func (api *API) Export(ctx context.Context) (params.SerializedModel, error) {
	var serialized params.SerializedModel

	model, err := api.modelExporter.ExportModel(ctx, api.store)
	if err != nil {
		return serialized, err
	}

	bytes, err := description.Serialize(model)
	if err != nil {
		return serialized, err
	}
	serialized.Bytes = bytes
	serialized.Charms = getUsedCharms(model)
	serialized.Resources = getUsedResources(model)
	if model.Type() == string(coremodel.IAAS) {
		serialized.Tools = getUsedTools(model)
	}
	return serialized, nil
}

// ProcessRelations processes any relations that need updating after an export.
// This should help fix any remoteApplications that have been migrated.
func (api *API) ProcessRelations(ctx context.Context, args params.ProcessRelations) error {
	return nil
}

// Reap removes all documents for the model associated with the API
// connection.
func (api *API) Reap(ctx context.Context) error {
	// TODO(modelmigration): handle setting model redirection/marking the model
	// as gone.

	// We need to mark the migration as complete here, since removing
	// the model might kill the worker before it has a chance to set
	// the phase itself.
	err := api.modelMigrationService.SetMigrationPhase(ctx, coremigration.DONE)
	if err != nil {
		return errors.Annotate(err, "failed to set phase")
	}
	return nil
}

// WatchMinionReports sets up a watcher which reports when a report
// for a migration minion has arrived.
func (api *API) WatchMinionReports(ctx context.Context) params.NotifyWatchResult {
	result := params.NotifyWatchResult{}

	w, err := api.modelMigrationService.WatchMinionReports(ctx)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, w)
	result.Error = apiservererrors.ServerError(err)
	return result
}

// MinionReports returns details of the reports made by migration
// minions to the controller for the current migration phase.
func (api *API) MinionReports(ctx context.Context) (params.MinionReports, error) {
	var out params.MinionReports

	migration, err := api.modelMigrationService.Migration(ctx)
	if err != nil {
		return out, errors.Trace(err)
	}
	out.MigrationId = migration.UUID
	out.Phase = migration.Phase.String()

	reports, err := api.modelMigrationService.MinionReports(ctx)
	if err != nil {
		return out, errors.Trace(err)
	}
	out.SuccessCount = reports.SuccessCount
	out.UnknownCount = reports.UnknownCount

	out.Failed = make([]string, 0,
		len(reports.FailedApplications)+
			len(reports.FailedMachines)+
			len(reports.FailedUnits),
	)
	for _, applicationName := range reports.FailedApplications {
		out.Failed = append(out.Failed,
			names.NewApplicationTag(applicationName).String())
	}
	for _, machineId := range reports.FailedMachines {
		out.Failed = append(out.Failed,
			names.NewMachineTag(machineId).String())
	}
	for _, unitName := range reports.FailedUnits {
		out.Failed = append(out.Failed,
			names.NewUnitTag(unitName).String())
	}
	naturalsort.Sort(out.Failed)

	unknown := make([]string, 0,
		len(reports.SomeUnknownApplications)+
			len(reports.SomeUnknownMachines)+
			len(reports.SomeUnknownUnits),
	)
	for _, applicationName := range reports.SomeUnknownApplications {
		unknown = append(unknown,
			names.NewApplicationTag(applicationName).String())
	}
	for _, machineId := range reports.SomeUnknownMachines {
		unknown = append(unknown,
			names.NewMachineTag(machineId).String())
	}
	for _, unitName := range reports.SomeUnknownUnits {
		unknown = append(unknown,
			names.NewUnitTag(unitName).String())
	}
	naturalsort.Sort(unknown)

	// Limit the number of unknowns reported
	numSamples := len(unknown)
	if numSamples > 10 {
		numSamples = 10
	}
	out.UnknownSample = unknown[:numSamples]

	return out, nil
}

// MinionReportTimeout returns the configuration value for this controller that
// indicates how long the migration master worker should wait for minions to
// reported on phases of a migration.
func (api *API) MinionReportTimeout(ctx context.Context) (params.StringResult, error) {
	cfg, err := api.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return params.StringResult{Error: apiservererrors.ServerError(err)}, nil
	}
	return params.StringResult{Result: cfg.MigrationMinionWaitMax().String()}, nil
}

func getUsedCharms(model description.Model) []string {
	result := set.NewStrings()
	for _, application := range model.Applications() {
		result.Add(application.CharmURL())
	}
	return result.Values()
}

func getUsedTools(model description.Model) []params.SerializedModelTools {
	// Iterate through the model for all tools, and make a map of them.
	tools := map[string]params.SerializedModelTools{}

	addTools := func(agentTools description.AgentTools) {
		if _, exists := tools[agentTools.SHA256()]; exists {
			return
		}

		tools[agentTools.SHA256()] = params.SerializedModelTools{
			Version: agentTools.Version(),
			SHA256:  agentTools.SHA256(),
			URI:     common.ToolsURL("", agentTools.Version()),
		}
	}

	for _, machine := range model.Machines() {
		addTools(machine.Tools())
		for _, container := range machine.Containers() {
			addTools(container.Tools())
		}
	}
	for _, application := range model.Applications() {
		for _, unit := range application.Units() {
			addTools(unit.Tools())
		}
	}

	out := make([]params.SerializedModelTools, 0, len(tools))
	for _, v := range tools {
		out = append(out, v)
	}
	return out
}

func getUsedResources(model description.Model) []params.SerializedModelResource {
	var out []params.SerializedModelResource
	for _, app := range model.Applications() {
		for _, resource := range app.Resources() {
			out = append(out, resourceToSerialized(app.Name(), resource))
		}

	}
	return out
}

func resourceToSerialized(app string, desc description.Resource) params.SerializedModelResource {
	res := params.SerializedModelResource{
		Application: app,
		Name:        desc.Name(),
	}
	rr := desc.ApplicationRevision()
	if rr == nil {
		return res
	}
	res.Revision = rr.Revision()
	res.Type = rr.Type()
	res.Origin = rr.Origin()
	res.FingerprintHex = rr.SHA384()
	res.Size = rr.Size()
	res.Timestamp = rr.Timestamp()
	res.Username = rr.RetrievedBy()
	return res
}
