// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/description"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("MigrationMaster", 1, newAPIForRegistration)
}

// API implements the API required for the model migration
// master worker.
type API struct {
	backend    Backend
	authorizer common.Authorizer
	resources  *common.Resources
}

// NewAPI creates a new API server endpoint for the model migration
// master worker.
func NewAPI(
	backend Backend,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	return &API{
		backend:    backend,
		authorizer: authorizer,
		resources:  resources,
	}, nil
}

// Watch starts watching for an active migration for the model
// associated with the API connection. The returned id should be used
// with the NotifyWatcher facade to receive events.
func (api *API) Watch() params.NotifyWatchResult {
	watch := api.backend.WatchForModelMigration()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}
	}
	return params.NotifyWatchResult{
		Error: common.ServerError(watcher.EnsureErr(watch)),
	}
}

// GetMigrationStatus returns the details and progress of the latest
// model migration.
func (api *API) GetMigrationStatus() (params.FullMigrationStatus, error) {
	empty := params.FullMigrationStatus{}

	mig, err := api.backend.GetModelMigration()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving model migration")
	}

	target, err := mig.TargetInfo()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving target info")
	}

	attempt, err := mig.Attempt()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving attempt")
	}

	phase, err := mig.Phase()
	if err != nil {
		return empty, errors.Annotate(err, "retrieving phase")
	}

	return params.FullMigrationStatus{
		Spec: params.ModelMigrationSpec{
			ModelTag: names.NewModelTag(mig.ModelUUID()).String(),
			TargetInfo: params.ModelMigrationTargetInfo{
				ControllerTag: target.ControllerTag.String(),
				Addrs:         target.Addrs,
				CACert:        target.CACert,
				AuthTag:       target.AuthTag.String(),
				Password:      target.Password,
			},
		},
		Attempt: attempt,
		Phase:   phase.String(),
	}, nil
}

// SetPhase sets the phase of the active model migration. The provided
// phase must be a valid phase value, for example QUIESCE" or
// "ABORT". See the core/migration package for the complete list.
func (api *API) SetPhase(args params.SetMigrationPhaseArgs) error {
	mig, err := api.backend.GetModelMigration()
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

// Export serializes the model associated with the API connection.
func (api *API) Export() (params.SerializedModel, error) {
	var serialized params.SerializedModel

	model, err := api.backend.Export()
	if err != nil {
		return serialized, err
	}

	bytes, err := description.Serialize(model)
	if err != nil {
		return serialized, err
	}
	serialized.Bytes = bytes
	serialized.Charms = getUsedCharms(model)
	serialized.Tools = getUsedTools(model)
	return serialized, nil
}

// Reap removes all documents for the model associated with the API
// connection.
func (api *API) Reap() error {
	return api.backend.RemoveExportingModelDocs()
}

func getUsedCharms(model description.Model) []string {
	result := set.NewStrings()
	for _, service := range model.Services() {
		result.Add(service.CharmURL())
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

	for _, service := range model.Services() {
		for _, unit := range service.Units() {
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
