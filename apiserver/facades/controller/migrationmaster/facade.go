// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"encoding/json"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/naturalsort"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

// API implements the API required for the model migration
// master worker.
type API struct {
	controllerState         ControllerState
	backend                 Backend
	precheckBackend         migration.PrecheckBackend
	pool                    migration.Pool
	authorizer              facade.Authorizer
	resources               facade.Resources
	presence                facade.Presence
	environscloudspecGetter func(names.ModelTag) (environscloudspec.CloudSpec, error)
	leadership              leadership.Reader
}

// NewAPI creates a new API server endpoint for the model migration
// master worker.
func NewAPI(
	controllerState ControllerState,
	backend Backend,
	precheckBackend migration.PrecheckBackend,
	pool migration.Pool,
	resources facade.Resources,
	authorizer facade.Authorizer,
	presence facade.Presence,
	environscloudspecGetter func(names.ModelTag) (environscloudspec.CloudSpec, error),
	leadership leadership.Reader,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		controllerState:         controllerState,
		backend:                 backend,
		precheckBackend:         precheckBackend,
		pool:                    pool,
		authorizer:              authorizer,
		resources:               resources,
		presence:                presence,
		environscloudspecGetter: environscloudspecGetter,
		leadership:              leadership,
	}, nil
}

// Watch starts watching for an active migration for the model
// associated with the API connection. The returned id should be used
// with the NotifyWatcher facade to receive events.
func (api *API) Watch() params.NotifyWatchResult {
	watch := api.backend.WatchForMigration()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}
	}
	return params.NotifyWatchResult{
		Error: apiservererrors.ServerError(watcher.EnsureErr(watch)),
	}
}

// MigrationStatus returns the details and progress of the latest
// model migration.
func (api *API) MigrationStatus() (params.MasterMigrationStatus, error) {
	empty := params.MasterMigrationStatus{}

	mig, err := api.backend.LatestMigration()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving model migration")
	}
	target, err := mig.TargetInfo()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving target info")
	}
	phase, err := mig.Phase()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving phase")
	}
	macsJSON, err := json.Marshal(target.Macaroons)
	if err != nil {
		return empty, errors.Annotate(err, "marshalling macaroons")
	}
	return params.MasterMigrationStatus{
		Spec: params.MigrationSpec{
			ModelTag: names.NewModelTag(mig.ModelUUID()).String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag:  target.ControllerTag.String(),
				Addrs:          target.Addrs,
				CACert:         target.CACert,
				AuthTag:        target.AuthTag.String(),
				Password:       target.Password,
				Macaroons:      string(macsJSON),
				SkipUserChecks: target.SkipUserChecks,
				Token:          target.Token,
			},
		},
		MigrationId:      mig.Id(),
		Phase:            phase.String(),
		PhaseChangedTime: mig.PhaseChangedTime(),
	}, nil
}

// ModelInfo returns essential information about the model to be
// migrated.
func (api *API) ModelInfo() (params.MigrationModelInfo, error) {
	empty := params.MigrationModelInfo{}

	name, err := api.backend.ModelName()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving model name")
	}

	owner, err := api.backend.ModelOwner()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving model owner")
	}

	vers, err := api.backend.AgentVersion()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving agent version")
	}

	leaders, err := api.leadership.Leaders()
	if err != nil {
		return empty, errors.Annotatef(err, "retrieving leaders")
	}

	model, err := api.backend.Export(leaders)
	if err != nil {
		return empty, errors.Annotate(err, "retrieving model")
	}

	modelDescription, err := description.Serialize(model)
	if err != nil {
		return empty, errors.Annotate(err, "serializing model")
	}

	return params.MigrationModelInfo{
		UUID:             api.backend.ModelUUID(),
		Name:             name,
		OwnerTag:         owner.String(),
		AgentVersion:     vers,
		ModelDescription: modelDescription,
	}, nil
}

// SourceControllerInfo returns the details required to connect to
// the source controller for model migration.
func (api *API) SourceControllerInfo() (params.MigrationSourceInfo, error) {
	empty := params.MigrationSourceInfo{}

	localRelatedModels, err := api.backend.AllLocalRelatedModels()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving local related models")
	}

	cfg, err := api.backend.ControllerConfig()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving controller config")
	}
	cacert, _ := cfg.CACert()

	hostports, err := api.controllerState.APIHostPortsForClients()
	if err != nil {
		return empty, errors.Trace(err)
	}
	var addr []string
	for _, section := range hostports {
		for _, hostport := range section {
			addr = append(addr, hostport.String())
		}
	}

	return params.MigrationSourceInfo{
		LocalRelatedModels: localRelatedModels,
		ControllerTag:      names.NewControllerTag(cfg.ControllerUUID()).String(),
		ControllerAlias:    cfg.ControllerName(),
		Addrs:              addr,
		CACert:             cacert,
	}, nil
}

// SetPhase sets the phase of the active model migration. The provided
// phase must be a valid phase value, for example QUIESCE" or
// "ABORT". See the core/migration package for the complete list.
func (api *API) SetPhase(args params.SetMigrationPhaseArgs) error {
	mig, err := api.backend.LatestMigration()
	if err != nil {
		return errors.Annotate(err, "could not get migration")
	}

	phase, ok := coremigration.ParsePhase(args.Phase)
	if !ok {
		return errors.Errorf("invalid phase: %q", args.Phase)
	}

	err = mig.SetPhase(phase)
	return errors.Annotate(err, "failed to set phase")
}

// Prechecks performs pre-migration checks on the model and
// (source) controller.
func (api *API) Prechecks(arg params.PrechecksArgs) error {
	model, err := api.precheckBackend.Model()
	if err != nil {
		return errors.Annotate(err, "retrieving model")
	}
	backend, err := api.precheckBackend.ControllerBackend()
	if err != nil {
		return errors.Trace(err)
	}
	controllerModel, err := backend.Model()
	if err != nil {
		return errors.Trace(err)
	}
	return migration.SourcePrecheck(
		api.precheckBackend,
		api.presence.ModelPresence(model.UUID()),
		api.presence.ModelPresence(controllerModel.UUID()),
		api.environscloudspecGetter,
	)
}

// SetStatusMessage sets a human readable status message containing
// information about the migration's progress. This will be shown in
// status output shown to the end user.
func (api *API) SetStatusMessage(args params.SetMigrationStatusMessageArgs) error {
	mig, err := api.backend.LatestMigration()
	if err != nil {
		return errors.Annotate(err, "could not get migration")
	}
	err = mig.SetStatusMessage(args.Message)
	return errors.Annotate(err, "failed to set status message")
}

// Export serializes the model associated with the API connection.
func (api *API) Export() (params.SerializedModel, error) {
	var serialized params.SerializedModel

	leaders, err := api.leadership.Leaders()
	if err != nil {
		return serialized, err
	}

	model, err := api.backend.Export(leaders)
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
func (api *API) ProcessRelations(args params.ProcessRelations) error {
	return nil
}

// Reap removes all documents for the model associated with the API
// connection.
func (api *API) Reap() error {
	mig, err := api.backend.LatestMigration()
	if err != nil {
		return errors.Trace(err)
	}
	err = api.backend.RemoveExportingModelDocs()
	if err != nil {
		return errors.Trace(err)
	}
	// We need to mark the migration as complete here, since removing
	// the model might kill the worker before it has a chance to set
	// the phase itself.
	return errors.Trace(mig.SetPhase(coremigration.DONE))
}

// WatchMinionReports sets up a watcher which reports when a report
// for a migration minion has arrived.
func (api *API) WatchMinionReports() params.NotifyWatchResult {
	mig, err := api.backend.LatestMigration()
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}
	}

	watch, err := mig.WatchMinionReports()
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}
	}

	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}
	}
	return params.NotifyWatchResult{
		Error: apiservererrors.ServerError(watcher.EnsureErr(watch)),
	}
}

// MinionReports returns details of the reports made by migration
// minions to the controller for the current migration phase.
func (api *API) MinionReports() (params.MinionReports, error) {
	var out params.MinionReports

	mig, err := api.backend.LatestMigration()
	if err != nil {
		return out, errors.Trace(err)
	}

	reports, err := mig.MinionReports()
	if err != nil {
		return out, errors.Trace(err)
	}

	out.MigrationId = mig.Id()
	phase, err := mig.Phase()
	if err != nil {
		return out, errors.Trace(err)
	}
	out.Phase = phase.String()

	out.SuccessCount = len(reports.Succeeded)

	out.Failed = make([]string, len(reports.Failed))
	for i := 0; i < len(out.Failed); i++ {
		out.Failed[i] = reports.Failed[i].String()
	}
	naturalsort.Sort(out.Failed)

	out.UnknownCount = len(reports.Unknown)

	unknown := make([]string, len(reports.Unknown))
	for i := 0; i < len(unknown); i++ {
		unknown[i] = reports.Unknown[i].String()
	}
	naturalsort.Sort(unknown)

	// Limit the number of unknowns reported
	numSamples := out.UnknownCount
	if numSamples > 10 {
		numSamples = 10
	}
	out.UnknownSample = unknown[:numSamples]

	return out, nil
}

// MinionReportTimeout returns the configuration value for this controller that
// indicates how long the migration master worker should wait for minions to
// reported on phases of a migration.
func (api *API) MinionReportTimeout() (params.StringResult, error) {
	cfg, err := api.backend.ControllerConfig()
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
	usedVersions := make(map[version.Binary]bool)
	// It is most likely that the preconditions will limit the number of
	// tools versions in use, but that is not relied on here.
	for _, machine := range model.Machines() {
		addToolsVersionForMachine(machine, usedVersions)
	}

	for _, application := range model.Applications() {
		for _, unit := range application.Units() {
			tools := unit.Tools()
			usedVersions[tools.Version()] = true
		}
	}

	out := make([]params.SerializedModelTools, 0, len(usedVersions))
	for v := range usedVersions {
		out = append(out, params.SerializedModelTools{
			Version: v.String(),
			URI:     common.ToolsURL("", v),
		})
	}
	return out
}

func addToolsVersionForMachine(machine description.Machine, usedVersions map[version.Binary]bool) {
	tools := machine.Tools()
	usedVersions[tools.Version()] = true
	for _, container := range machine.Containers() {
		addToolsVersionForMachine(container, usedVersions)
	}
}

func getUsedResources(model description.Model) []params.SerializedModelResource {
	var out []params.SerializedModelResource
	for _, app := range model.Applications() {
		for _, resource := range app.Resources() {
			outRes := resourceToSerialized(app.Name(), resource)

			// Hunt through the application's units and look for
			// revisions of this resource. This is particularly
			// efficient or clever but will be fine even with 1000's
			// of units and 10's of resources.
			outRes.UnitRevisions = make(map[string]params.SerializedModelResourceRevision)
			for _, unit := range app.Units() {
				for _, unitResource := range unit.Resources() {
					if unitResource.Name() == resource.Name() {
						outRes.UnitRevisions[unit.Name()] = revisionToSerialized(unitResource.Revision())
					}
				}
			}

			out = append(out, outRes)
		}

	}
	return out
}

func resourceToSerialized(app string, desc description.Resource) params.SerializedModelResource {
	return params.SerializedModelResource{
		Application:         app,
		Name:                desc.Name(),
		ApplicationRevision: revisionToSerialized(desc.ApplicationRevision()),
		CharmStoreRevision:  revisionToSerialized(desc.CharmStoreRevision()),
	}
}

func revisionToSerialized(rr description.ResourceRevision) params.SerializedModelResourceRevision {
	if rr == nil {
		return params.SerializedModelResourceRevision{}
	}
	return params.SerializedModelResourceRevision{
		Revision:       rr.Revision(),
		Type:           rr.Type(),
		Path:           rr.Path(),
		Description:    rr.Description(),
		Origin:         rr.Origin(),
		FingerprintHex: rr.FingerprintHex(),
		Size:           rr.Size(),
		Timestamp:      rr.Timestamp(),
		Username:       rr.Username(),
	}
}
