// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/environs/config"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/state"
)

// SourcePrecheck checks the state of the source controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the model to be migrated.
func SourcePrecheck(
	ctx context.Context,
	backend PrecheckBackend,
	environscloudspecGetter environsCloudSpecGetter,
	credentialService CredentialService,
	upgradeService UpgradeService,
	applicationService ApplicationService,
	statusService StatusService,
	modelAgentService ModelAgentService,
) error {
	c := newPrecheckSource(backend, environscloudspecGetter, credentialService, upgradeService, applicationService, statusService, modelAgentService)
	if err := c.checkModel(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.checkMachines(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := statusService.CheckUnitStatusesReadyForMigration(ctx); err != nil {
		return errors.Trace(err)
	}

	appUnits, err := c.checkApplications(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if err := c.checkRelations(appUnits); err != nil {
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
	controllerCtx := newPrecheckTarget(controllerBackend, upgradeService, applicationService, statusService, modelAgentService)
	if err := controllerCtx.checkController(ctx); err != nil {
		return errors.Annotate(err, "controller")
	}
	return nil
}

// ImportPrecheck checks the data being imported to make sure preconditions for
// importing are met.
func ImportPrecheck(
	ctx context.Context,
	model description.Model,
) error {

	err := checkForCharmsWithNoManifest(model)
	if err != nil {
		return internalerrors.Errorf("checking model for charms without manifest.yaml: %w", err)
	}
	return nil
}

// TargetPrecheck checks the state of the target controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the target controller.
func TargetPrecheck(ctx context.Context,
	backend PrecheckBackend,
	pool Pool,
	modelInfo coremigration.ModelInfo,
	upgradeService UpgradeService,
	applicationService ApplicationService,
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

	controllerCtx := newPrecheckTarget(backend, upgradeService, applicationService, statusService, modelAgentService)
	if err := controllerCtx.checkController(ctx); err != nil {
		return errors.Trace(err)
	}

	if modelInfo.ModelDescription != nil {
		if err := checkNoFanConfig(modelInfo.ModelDescription.Config()); err != nil {
			return errors.Trace(err)
		}
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

		// If the model is importing then it's probably left behind
		// from a previous migration attempt. It will be removed
		// before the next import.
		if model.UUID() == modelInfo.UUID && model.MigrationMode() != state.MigrationModeImporting {
			return errors.Errorf("model with same UUID already exists (%s)", modelInfo.UUID)
		}
		if model.Name() == modelInfo.Name && model.Owner() == modelInfo.Owner {
			return errors.Errorf("model named %q already exists", model.Name())
		}
	}

	return nil
}

type precheckTarget struct {
	precheckContext
}

func newPrecheckTarget(
	backend PrecheckBackend,
	upgradeService UpgradeService,
	applicationService ApplicationService,
	statusService StatusService,
	modelAgentService ModelAgentService,
) *precheckTarget {
	return &precheckTarget{
		precheckContext: precheckContext{
			backend:            backend,
			upgradeService:     upgradeService,
			applicationService: applicationService,
			statusService:      statusService,
			modelAgentService:  modelAgentService,
		},
	}
}

type precheckContext struct {
	backend            PrecheckBackend
	upgradeService     UpgradeService
	applicationService ApplicationService
	statusService      StatusService
	modelAgentService  ModelAgentService
}

func (c *precheckContext) checkController(ctx context.Context) error {
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

func (c *precheckContext) checkMachines(ctx context.Context) error {
	machines, err := c.backend.AllMachines()
	if err != nil {
		return errors.Annotate(err, "retrieving machines")
	}
	for _, machine := range machines {
		if machine.Life() != state.Alive {
			return errors.Errorf("machine %s is %s", machine.Id(), machine.Life())
		}

		if statusInfo, err := machine.InstanceStatus(); err != nil {
			return errors.Annotatef(err, "retrieving machine %s instance status", machine.Id())
		} else if !status.IsInstancePresent(statusInfo) {
			return newStatusError("machine %s not running", machine.Id(), statusInfo.Status)
		}

		if statusInfo, err := machine.Status(); err != nil {
			return errors.Annotatef(err, "retrieving machine %s status", machine.Id())
		} else if !status.IsMachinePresent(statusInfo) {
			return newStatusError("machine %s agent not functioning at this time",
				machine.Id(), statusInfo.Status)
		}

		// TODO(gfouillet): Restore this once machine fully migrated to dqlite
		// if rebootAction, err := machine.ShouldRebootOrShutdown(); err != nil {
		// 	return errors.Annotatef(err, "retrieving machine %s reboot status", machine.Id())
		// } else if rebootAction != state.ShouldDoNothing {
		// 	return errors.Errorf("machine %s is scheduled to %s", machine.Id(), rebootAction)
		// }

		// TODO(tlm): Add this back in when we move agent tools into model
		// migration.
		//modelVersion, err := c.modelAgentService.GetModelTargetAgentVersion(ctx)
		//if err != nil {
		//	return errors.Annotate(err, "retrieving model version")
		//}
		//if err := checkAgentTools(modelVersion, machine, "machine "+machine.Id()); err != nil {
		//	return errors.Trace(err)
		//}
	}
	return nil
}

func (c *precheckContext) checkApplications(ctx context.Context) (map[string][]PrecheckUnit, error) {
	modelVersion, err := c.modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "retrieving model version")
	}
	apps, err := c.backend.AllApplications()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving applications")
	}

	model, err := c.backend.Model()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving model")
	}
	appUnits := make(map[string][]PrecheckUnit, len(apps))
	for _, app := range apps {
		appLife, err := c.applicationService.GetApplicationLife(ctx, app.Name())
		if err != nil {
			if errors.Is(err, applicationerrors.ApplicationNotFound) {
				err = errors.NotFoundf("application %s", app.Name())
			}
			return nil, errors.Annotatef(err, "retrieving life for %q", app.Name())
		}
		if appLife != life.Alive {
			return nil, errors.Errorf("application %s is %s", app.Name(), appLife)
		}
		units, err := app.AllUnits()
		if err != nil {
			return nil, errors.Annotatef(err, "retrieving units for %s", app.Name())
		}
		err = c.checkUnits(ctx, app, units, modelVersion, model.Type())
		if err != nil {
			return nil, errors.Trace(err)
		}
		appUnits[app.Name()] = units
	}
	return appUnits, nil
}

func (c *precheckContext) checkUnits(ctx context.Context, app PrecheckApplication, units []PrecheckUnit, modelVersion semversion.Number, modelType state.ModelType) error {
	appCharmURL, _ := app.CharmURL()
	if appCharmURL == nil {
		return errors.Errorf("application charm url is nil")
	}

	for _, unit := range units {
		if unit.Life() != state.Alive {
			return errors.Errorf("unit %s is %s", unit.Name(), unit.Life())
		}

		// TODO(tlm): Add this back in when we move agent tools into model
		// migration.
		//if modelType == state.ModelTypeIAAS {
		//	if err := checkAgentTools(modelVersion, unit, "unit "+unit.Name()); err != nil {
		//		return errors.Trace(err)
		//	}
		//}

		unitCharmURL := unit.CharmURL()
		if unitCharmURL == nil || *appCharmURL != *unitCharmURL {
			return errors.Errorf("unit %s is upgrading", unit.Name())
		}
	}
	return nil
}

func (c *precheckContext) checkRelations(appUnits map[string][]PrecheckUnit) error {
	relations, err := c.backend.AllRelations()
	if err != nil {
		return errors.Annotate(err, "retrieving model relations")
	}
	for _, rel := range relations {
		remoteAppName, crossModel, err := rel.RemoteApplication()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Annotatef(err, "checking whether relation %s is cross-model", rel)
		}

		checkRelationUnit := func(ru PrecheckRelationUnit) error {
			valid, err := ru.Valid()
			if err != nil {
				return errors.Trace(err)
			}
			if !valid {
				return nil
			}
			inScope, err := ru.InScope()
			if err != nil {
				return errors.Trace(err)
			}
			if !inScope {
				return errors.Errorf("unit %s hasn't joined relation %q yet", ru.UnitName(), rel)
			}
			return nil
		}

		for _, ep := range rel.Endpoints() {
			// The endpoint app is either local or cross model.
			// Handle each one as appropriate.
			if crossModel && ep.ApplicationName == remoteAppName {
				remoteUnits, err := rel.AllRemoteUnits(remoteAppName)
				if err != nil {
					return errors.Trace(err)
				}
				for _, ru := range remoteUnits {
					if err := checkRelationUnit(ru); err != nil {
						return errors.Trace(err)
					}
				}
			} else {
				for _, unit := range appUnits[ep.ApplicationName] {
					ru, err := rel.Unit(unit)
					if err != nil {
						return errors.Trace(err)
					}
					if err := checkRelationUnit(ru); err != nil {
						return errors.Trace(err)
					}
				}
			}
		}
	}
	return nil
}

type precheckSource struct {
	precheckContext
	environscloudspecGetter environsCloudSpecGetter
	credentialService       CredentialService
}

func newPrecheckSource(
	backend PrecheckBackend, environscloudspecGetter environsCloudSpecGetter,
	credentialService CredentialService,
	upgradeService UpgradeService,
	applicationService ApplicationService,
	statusService StatusService,
	modelAgentService ModelAgentService,
) *precheckSource {
	return &precheckSource{
		precheckContext: precheckContext{
			backend:            backend,
			upgradeService:     upgradeService,
			applicationService: applicationService,
			statusService:      statusService,
			modelAgentService:  modelAgentService,
		},
		environscloudspecGetter: environscloudspecGetter,
		credentialService:       credentialService,
	}
}

func (ctx *precheckSource) checkModel(stdCtx context.Context) error {
	model, err := ctx.backend.Model()
	if err != nil {
		return errors.Annotate(err, "retrieving model")
	}
	if model.Life() != state.Alive {
		return errors.Errorf("model is %s", model.Life())
	}
	if model.MigrationMode() == state.MigrationModeImporting {
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

	cloudspec, err := ctx.environscloudspecGetter(stdCtx, names.NewModelTag(model.UUID()))
	if err != nil {
		return errors.Trace(err)
	}
	validators := upgradevalidation.ValidatorsForModelMigrationSource(cloudspec)
	checker := upgradevalidation.NewModelUpgradeCheck(nil, ctx.backend, model, ctx.modelAgentService, validators...)
	blockers, err := checker.Validate()
	if err != nil {
		return errors.Trace(err)
	}
	if blockers == nil {
		return nil
	}
	return errors.NewNotSupported(nil, fmt.Sprintf("cannot migrate to controller due to issues:\n%s", blockers))
}

type agentToolsGetter interface {
	AgentTools() (*tools.Tools, error)
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

// TODO(tlm): Add this back in when we move agent tools into model migration.
//func checkAgentTools(modelVersion semversion.Number, agent agentToolsGetter, agentLabel string) error {
//	tools, err := agent.AgentTools()
//	if err != nil {
//		return errors.Annotatef(err, "retrieving agent binaries for %s", agentLabel)
//	}
//	agentVersion := tools.Version.Number
//	if agentVersion != modelVersion {
//		return errors.Errorf("%s agent binaries don't match model (%s != %s)",
//			agentLabel, agentVersion, modelVersion)
//	}
//	return nil
//}

func newStatusError(format, id string, s status.Status) error {
	msg := fmt.Sprintf(format, id)
	if s != status.Empty {
		msg += fmt.Sprintf(" (%s)", s)
	}
	return errors.New(msg)
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
		if len(app.CharmManifest().Bases()) == 0 {
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
