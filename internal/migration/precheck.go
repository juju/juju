// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/description/v10"
	"github.com/juju/errors"

	"github.com/juju/juju/core/credential"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/environs/config"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/state"
)

// SourcePrecheck checks the state of the source controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the model to be migrated.
func SourcePrecheck(
	ctx context.Context,
	backend PrecheckBackend,
	modelUUID coremodel.UUID,
	controllerModelUUID coremodel.UUID,
	credentialServiceGetter func(context.Context, coremodel.UUID) (CredentialService, error),
	upgradeServiceGetter func(context.Context, coremodel.UUID) (UpgradeService, error),
	applicationServiceGetter func(context.Context, coremodel.UUID) (ApplicationService, error),
	relationServiceGetter func(context.Context, coremodel.UUID) (RelationService, error),
	statusServiceGetter func(context.Context, coremodel.UUID) (StatusService, error),
	modelAgentServiceGetter func(context.Context, coremodel.UUID) (ModelAgentService, error),
) error {

	modelCredentialService, err := credentialServiceGetter(ctx, modelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	modelApplicationService, err := applicationServiceGetter(ctx, modelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	modelRelationService, err := relationServiceGetter(ctx, modelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	modelStatusService, err := statusServiceGetter(ctx, modelUUID)
	if err != nil {
		return errors.Trace(err)
	} else if modelStatusService == nil {
		return errors.Errorf("status service for model %q not found", modelUUID)
	}
	modelModelAgentService, err := modelAgentServiceGetter(ctx, modelUUID)
	if err != nil {
		return errors.Trace(err)
	}

	c := newPrecheckModel(backend, modelCredentialService, modelApplicationService, modelRelationService, modelStatusService, modelModelAgentService)
	if err := c.checkModel(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.checkMachines(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.checkApplications(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.checkRelations(ctx); err != nil {
		return errors.Trace(err)
	}

	if cleanupNeeded, err := backend.NeedsCleanup(); err != nil {
		return errors.Annotate(err, "checking cleanups")
	} else if cleanupNeeded {
		return errors.New("cleanup needed")
	}

	// Check the source controller.
	controllerBackend, err := backend.ControllerBackend()
	if err != nil {
		return errors.Trace(err)
	}
	controllerUpgradeService, err := upgradeServiceGetter(ctx, controllerModelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	controllerStatusService, err := statusServiceGetter(ctx, controllerModelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	controllerModelAgentService, err := modelAgentServiceGetter(ctx, controllerModelUUID)
	if err != nil {
		return errors.Trace(err)
	}

	controllerCtx := newPrecheckController(controllerBackend, controllerUpgradeService, controllerStatusService, controllerModelAgentService)
	if err := controllerCtx.checkController(ctx); err != nil {
		return errors.Annotate(err, "controller")
	}
	return nil
}

// ImportDescriptionPrecheck checks the data being imported to make sure
// preconditions for importing are met. This performs static checks on the
// received model description, without requiring a connection to state.
func ImportDescriptionPrecheck(
	ctx context.Context,
	model description.Model,
) error {
	err := checkForCharmsWithNoManifest(model)
	if err != nil {
		return internalerrors.Errorf("checking model for charms without manifest.yaml: %w", err)
	}

	if err := checkNoFanConfig(model.Config()); err != nil {
		return internalerrors.Errorf("checking model config for fan config: %w", err)
	}

	return nil
}

// TargetPrecheck checks the state of the target controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the target controller.
func TargetPrecheck(
	ctx context.Context,
	backend PrecheckBackend,
	pool Pool,
	modelInfo coremigration.ModelInfo,
	upgradeService UpgradeService,
	statusService StatusService,
	modelAgentService ModelAgentService,
) error {
	if err := modelInfo.Validate(); err != nil {
		return errors.Trace(err)
	}

	// This check is necessary because there is a window between the
	// REAP phase and then end of the DONE phase where a model's
	// documents have been deleted but the migration isn't quite done
	// yet. Migrating a model back into the controller during this
	// window can upset the migrationmaster worker.
	//
	// See also https://lpad.tv/1611391
	if migrating, err := backend.IsMigrationActive(modelInfo.UUID); err != nil {
		return errors.Annotate(err, "checking for active migration")
	} else if migrating {
		return errors.New("model is being migrated out of target controller")
	}

	controllerVersion, err := modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return errors.Annotate(err, "retrieving model version")
	}

	if controllerVersion.Compare(modelInfo.AgentVersion) < 0 {
		return errors.Errorf("model has higher version than target controller (%s > %s)",
			modelInfo.AgentVersion, controllerVersion)
	}

	if !controllerVersionCompatible(modelInfo.ControllerAgentVersion, controllerVersion) {
		return errors.Errorf("source controller has higher version than target controller (%s > %s)",
			modelInfo.ControllerAgentVersion, controllerVersion)
	}

	controllerCtx := newPrecheckController(backend, upgradeService, statusService, modelAgentService)
	if err := controllerCtx.checkController(ctx); err != nil {
		return errors.Trace(err)
	}

	// Check for conflicts with existing models
	modelUUIDs, err := backend.AllModelUUIDs()
	if err != nil {
		return errors.Annotate(err, "retrieving models")
	}
	for _, modelUUID := range modelUUIDs {
		model, release, err := pool.GetModel(modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		defer release()

		mode, err := model.MigrationMode()
		if err != nil {
			return errors.Trace(err)
		}

		// If the model is importing then it's probably left behind
		// from a previous migration attempt. It will be removed
		// before the next import.
		if model.UUID() == modelInfo.UUID && mode != state.MigrationModeImporting {
			return errors.Errorf("model with same UUID already exists (%s)", modelInfo.UUID)
		}
		// TODO - model owner comes from mongo and will be replace by a Qualifier.
		if model.Name() == modelInfo.Name && coremodel.QualifierFromUserTag(model.Owner()) == modelInfo.Qualifier {
			return errors.Errorf("model named %q already exists", model.Name())
		}
	}

	return nil
}

type precheckContext struct {
	backend           PrecheckBackend
	statusService     StatusService
	modelAgentService ModelAgentService
}

func (c *precheckContext) checkMachines(ctx context.Context) error {
	machines, err := c.backend.AllMachines()
	if err != nil {
		return errors.Annotate(err, "retrieving machines")
	}

	agentLaggingMachines, err := c.modelAgentService.GetMachinesNotAtTargetAgentVersion(ctx)
	if err != nil {
		return internalerrors.Errorf(
			"getting machines that are not running the target agent version of the model: %w",
			err,
		)
	}
	if len(agentLaggingMachines) > 0 {
		return internalerrors.Errorf(
			"there exists machines in the model that are not running the target agent version of the model %v",
			agentLaggingMachines,
		)
	}

	if err := c.statusService.CheckMachineStatusesReadyForMigration(ctx); err != nil {
		return internalerrors.Errorf("pre-checking machine statuses for migration: %w", err)
	}

	for _, machine := range machines {
		if machine.Life() != state.Alive {
			return errors.Errorf("machine %s is %s", machine.Id(), machine.Life())
		}

		// TODO(gfouillet): Restore this once machine fully migrated to dqlite
		// if rebootAction, err := machine.ShouldRebootOrShutdown(); err != nil {
		// 	return errors.Annotatef(err, "retrieving machine %s reboot status", machine.Id())
		// } else if rebootAction != state.ShouldDoNothing {
		// 	return errors.Errorf("machine %s is scheduled to %s", machine.Id(), rebootAction)
		// }
	}
	return nil
}

type precheckController struct {
	precheckContext
	upgradeService UpgradeService
}

func newPrecheckController(
	backend PrecheckBackend,
	upgradeService UpgradeService,
	statusService StatusService,
	modelAgentService ModelAgentService,
) *precheckController {
	return &precheckController{
		precheckContext: precheckContext{
			backend:           backend,
			statusService:     statusService,
			modelAgentService: modelAgentService,
		},
		upgradeService: upgradeService,
	}
}

func (c *precheckController) checkController(ctx context.Context) error {
	model, err := c.backend.Model()
	if err != nil {
		return errors.Annotate(err, "retrieving model")
	}
	if model.Life() != state.Alive {
		return errors.Errorf("model is %s", model.Life())
	}

	if upgrading, err := c.upgradeService.IsUpgrading(ctx); err != nil {
		return errors.Annotate(err, "checking for upgrades")
	} else if upgrading {
		return errors.New("upgrade in progress")
	}

	return errors.Trace(c.checkMachines(ctx))
}

type precheckModel struct {
	precheckContext
	credentialService  CredentialService
	applicationService ApplicationService
	relationService    RelationService
}

func newPrecheckModel(
	backend PrecheckBackend,
	credentialService CredentialService,
	applicationService ApplicationService,
	relationService RelationService,
	statusService StatusService,
	modelAgentService ModelAgentService,
) *precheckModel {
	return &precheckModel{
		precheckContext: precheckContext{
			backend:           backend,
			statusService:     statusService,
			modelAgentService: modelAgentService,
		},
		applicationService: applicationService,
		credentialService:  credentialService,
		relationService:    relationService,
	}
}

func (c *precheckModel) checkApplications(ctx context.Context) error {
	// We check all units in the model for every application. This checks to see
	// that there agent versions are what we expect.
	agentLaggingUnits, err := c.modelAgentService.GetUnitsNotAtTargetAgentVersion(ctx)
	if err != nil {
		return internalerrors.Errorf(
			"getting units that are not running the target agent version for the model: %w", err,
		)
	}
	if len(agentLaggingUnits) > 0 {
		return internalerrors.Errorf(
			"there exists units in the model that are not running the target agent version of the model %v",
			agentLaggingUnits,
		)
	}

	if err := c.statusService.CheckUnitStatusesReadyForMigration(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.applicationService.CheckAllApplicationsAndUnitsAreAlive(ctx); err != nil {
		return internalerrors.Errorf("pre-checking applications for migration: %w", err)
	}

	// TODO(aflynn): 2025-05-24 check if any units are mid-upgrade.

	return nil
}

// checkRelations verify that all units involved in a relation are actually in
// scope and valid.
func (c *precheckModel) checkRelations(ctx context.Context) error {
	// TODO(gfouillet): Handle crossmodel relation
	//  This code doesn't rely on future crossmodel domain, but similar check
	//  would be required on remote units.

	// TODO(jack-w-shaw): Push this entire check into a service method
	relations, err := c.relationService.GetAllRelationDetails(ctx)
	if err != nil {
		return errors.Annotate(err, "retrieving model relations")
	}

	unitsCache := make(map[string][]coreunit.Name)

	for _, rel := range relations {
		for _, ep := range rel.Endpoints {
			unitNames, ok := unitsCache[ep.ApplicationName]
			if !ok {
				unitNames, err = c.applicationService.GetUnitNamesForApplication(ctx, ep.ApplicationName)
				if err != nil {
					return errors.Annotatef(err, "retrieving unit names for application %s", ep.ApplicationName)
				}
				unitsCache[ep.ApplicationName] = unitNames
			}
			for _, unitName := range unitNames {
				ok, err := c.relationService.RelationUnitInScopeByID(ctx, rel.ID, unitName)
				if err != nil {
					return errors.Annotatef(err, "retrieving relation unit %s", unitName)
				}
				if !ok {
					// means the unit is not in scope
					key, err := relation.NewKey(transform.Slice(rel.Endpoints,
						domainrelation.Endpoint.EndpointIdentifier))
					if err != nil {
						return errors.Trace(err)
					}
					return errors.Errorf("unit %s hasn't joined relation %q yet", unitName, key)
				}
			}
		}
	}
	return nil
}

func (ctx *precheckModel) checkModel(stdCtx context.Context) error {
	model, err := ctx.backend.Model()
	if err != nil {
		return errors.Annotate(err, "retrieving model")
	}
	if model.Life() != state.Alive {
		return errors.Errorf("model is %s", model.Life())
	}
	mode, err := model.MigrationMode()
	if err != nil {
		return errors.Trace(err)
	}
	if mode == state.MigrationModeImporting {
		return errors.New("model is being imported as part of another migration")
	}
	if credTag, found := model.CloudCredentialTag(); found {
		creds, err := ctx.credentialService.CloudCredential(stdCtx, credential.KeyFromTag(credTag))
		if err != nil {
			return errors.Trace(err)
		}
		if creds.Revoked {
			return errors.New("model has revoked credentials")
		}
	}

	validators := upgradevalidation.ValidatorsForModelMigrationSource()
	modelGroupedName := fmt.Sprintf("%s/%s", model.Owner().Id(), model.Name())
	checker := upgradevalidation.NewModelUpgradeCheck(ctx.backend, modelGroupedName, ctx.modelAgentService, validators...)
	blockers, err := checker.Validate()
	if err != nil {
		return errors.Trace(err)
	}
	if blockers == nil {
		return nil
	}
	return errors.NewNotSupported(nil, fmt.Sprintf("cannot migrate to controller due to issues:\n%s", blockers))
}

const (
	fanConfigKey = "fan-config"
)

// checkNoFanConfig makes sure that no fan config was used in the config of the
// model being migrated.
func checkNoFanConfig(modelConfig map[string]interface{}) error {
	if modelConfig[fanConfigKey] != nil && modelConfig[fanConfigKey] != "" {
		return errors.Errorf("fan networking not supported, remove fan-config %q from migrating model config", modelConfig[fanConfigKey])
	}
	if modelConfig[config.ContainerNetworkingMethodKey] != nil && modelConfig[config.ContainerNetworkingMethodKey] == "fan" {
		return errors.Errorf("fan networking not supported, remove container-networking-method %q from migrating model config", modelConfig[config.ContainerNetworkingMethodKey])
	}
	return nil
}

func controllerVersionCompatible(sourceVersion, targetVersion semversion.Number) bool {
	// Compare source controller version to target controller version, only
	// considering major and minor version numbers. Downgrades between
	// patch, build releases for a given major.minor release are
	// ok. Tag differences are ok too.
	sourceVersion = versionToMajMin(sourceVersion)
	targetVersion = versionToMajMin(targetVersion)
	return sourceVersion.Compare(targetVersion) <= 0
}

func versionToMajMin(ver semversion.Number) semversion.Number {
	ver.Patch = 0
	ver.Build = 0
	ver.Tag = ""
	return ver
}

// checkForCharmsWithNoManifest checks the model for applications that use charms
// with no bases listed in the manifest.
func checkForCharmsWithNoManifest(model description.Model) error {
	result := set.NewStrings()
	for _, app := range model.Applications() {
		if app == nil {
			return internalerrors.Errorf("model contains nil application")
		}

		manifest := app.CharmManifest()
		if manifest == nil {
			result.Add(app.Name())
			continue
		}
		if len(manifest.Bases()) == 0 {
			result.Add(app.Name())
		}
	}
	if !result.IsEmpty() {
		return internalerrors.Errorf("all charms now require a manifest.yaml file, this model hosts charm(s) with no manifest.yaml file: %s",
			strings.Join(result.SortedValues(), ", "),
		)
	}
	return nil
}
