// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades/upgradevalidation"
)

// SourcePrecheck checks the state of the source controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the model to be migrated.
func SourcePrecheck(
	ctx context.Context,
	backend PrecheckBackend,
	modelPresence ModelPresence, controllerPresence ModelPresence,
	environscloudspecGetter environsCloudSpecGetter,
	credentialService CredentialService,
	upgradeService UpgradeService,
) error {
	c := newPrecheckSource(backend, modelPresence, environscloudspecGetter, credentialService, upgradeService)
	if err := c.checkModel(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.checkMachines(ctx); err != nil {
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
	controllerCtx := newPrecheckTarget(controllerBackend, controllerPresence, upgradeService)
	if err := controllerCtx.checkController(ctx); err != nil {
		return errors.Annotate(err, "controller")
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
	presence ModelPresence,
	upgradeService UpgradeService,
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

	controllerVersion, err := backend.AgentVersion()
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

	controllerCtx := newPrecheckTarget(backend, presence, upgradeService)
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
	presence ModelPresence,
	upgradeService UpgradeService,
) *precheckTarget {
	return &precheckTarget{
		precheckContext: precheckContext{
			backend:        backend,
			presence:       presence,
			upgradeService: upgradeService,
		},
	}
}

type precheckContext struct {
	backend        PrecheckBackend
	presence       ModelPresence
	upgradeService UpgradeService
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
	modelVersion, err := c.backend.AgentVersion()
	if err != nil {
		return errors.Annotate(err, "retrieving model version")
	}

	machines, err := c.backend.AllMachines()
	if err != nil {
		return errors.Annotate(err, "retrieving machines")
	}
	modelPresenceContext := common.ModelPresenceContext{Presence: c.presence}
	for _, machine := range machines {
		if machine.Life() != state.Alive {
			return errors.Errorf("machine %s is %s", machine.Id(), machine.Life())
		}

		if statusInfo, err := machine.InstanceStatus(); err != nil {
			return errors.Annotatef(err, "retrieving machine %s instance status", machine.Id())
		} else if statusInfo.Status != status.Running {
			return newStatusError("machine %s not running", machine.Id(), statusInfo.Status)
		}

		if statusInfo, err := modelPresenceContext.MachineStatus(ctx, machine); err != nil {
			return errors.Annotatef(err, "retrieving machine %s status", machine.Id())
		} else if statusInfo.Status != status.Started {
			return newStatusError("machine %s agent not functioning at this time",
				machine.Id(), statusInfo.Status)
		}

		if rebootAction, err := machine.ShouldRebootOrShutdown(); err != nil {
			return errors.Annotatef(err, "retrieving machine %s reboot status", machine.Id())
		} else if rebootAction != state.ShouldDoNothing {
			return errors.Errorf("machine %s is scheduled to %s", machine.Id(), rebootAction)
		}

		if err := checkAgentTools(modelVersion, machine, "machine "+machine.Id()); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *precheckContext) checkApplications(ctx context.Context) (map[string][]PrecheckUnit, error) {
	modelVersion, err := c.backend.AgentVersion()
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
		if app.Life() != state.Alive {
			return nil, errors.Errorf("application %s is %s", app.Name(), app.Life())
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

func (c *precheckContext) checkUnits(ctx context.Context, app PrecheckApplication, units []PrecheckUnit, modelVersion version.Number, modelType state.ModelType) error {
	if len(units) < app.MinUnits() {
		return errors.Errorf("application %s is below its minimum units threshold", app.Name())
	}

	appCharmURL, _ := app.CharmURL()
	if appCharmURL == nil {
		return errors.Errorf("application charm url is nil")
	}

	for _, unit := range units {
		if unit.Life() != state.Alive {
			return errors.Errorf("unit %s is %s", unit.Name(), unit.Life())
		}

		if err := c.checkUnitAgentStatus(ctx, unit); err != nil {
			return errors.Trace(err)
		}

		if modelType == state.ModelTypeIAAS {
			if err := checkAgentTools(modelVersion, unit, "unit "+unit.Name()); err != nil {
				return errors.Trace(err)
			}
		}

		unitCharmURL := unit.CharmURL()
		if unitCharmURL == nil || *appCharmURL != *unitCharmURL {
			return errors.Errorf("unit %s is upgrading", unit.Name())
		}
	}
	return nil
}

func (c *precheckContext) checkUnitAgentStatus(ctx context.Context, unit PrecheckUnit) error {
	modelPresenceContext := common.ModelPresenceContext{Presence: c.presence}
	statusData, _ := modelPresenceContext.UnitStatus(ctx, unit)
	if statusData.Err != nil {
		return errors.Annotatef(statusData.Err, "retrieving unit %s status", unit.Name())
	}
	agentStatus := statusData.Status.Status
	switch agentStatus {
	case status.Idle, status.Executing:
		// These two are fine.
	default:
		return newStatusError("unit %s not idle or executing", unit.Name(), agentStatus)
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
	backend PrecheckBackend, presence ModelPresence, environscloudspecGetter environsCloudSpecGetter,
	credentialService CredentialService,
	upgradeService UpgradeService,
) *precheckSource {
	return &precheckSource{
		precheckContext: precheckContext{
			backend:        backend,
			presence:       presence,
			upgradeService: upgradeService,
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
		creds, err := ctx.credentialService.CloudCredential(stdCtx, credential.IdFromTag(credTag))
		if err != nil {
			return errors.Trace(err)
		}
		if creds.Revoked {
			return errors.New("model has revoked credentials")
		}
	}

	cloudspec, err := ctx.environscloudspecGetter(names.NewModelTag(model.UUID()))
	if err != nil {
		return errors.Trace(err)
	}
	validators := upgradevalidation.ValidatorsForModelMigrationSource(cloudspec)
	checker := upgradevalidation.NewModelUpgradeCheck(model.UUID(), nil, ctx.backend, model, validators...)
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

func checkAgentTools(modelVersion version.Number, agent agentToolsGetter, agentLabel string) error {
	tools, err := agent.AgentTools()
	if err != nil {
		return errors.Annotatef(err, "retrieving agent binaries for %s", agentLabel)
	}
	agentVersion := tools.Version.Number
	if agentVersion != modelVersion {
		return errors.Errorf("%s agent binaries don't match model (%s != %s)",
			agentLabel, agentVersion, modelVersion)
	}
	return nil
}

func newStatusError(format, id string, s status.Status) error {
	msg := fmt.Sprintf(format, id)
	if s != status.Empty {
		msg += fmt.Sprintf(" (%s)", s)
	}
	return errors.New(msg)
}

func controllerVersionCompatible(sourceVersion, targetVersion version.Number) bool {
	// Compare source controller version to target controller version, only
	// considering major and minor version numbers. Downgrades between
	// patch, build releases for a given major.minor release are
	// ok. Tag differences are ok too.
	sourceVersion = versionToMajMin(sourceVersion)
	targetVersion = versionToMajMin(targetVersion)
	return sourceVersion.Compare(targetVersion) <= 0
}

func versionToMajMin(ver version.Number) version.Number {
	ver.Patch = 0
	ver.Build = 0
	ver.Tag = ""
	return ver
}
