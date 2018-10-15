// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

// PrecheckBackend defines the interface to query Juju's state
// for migration prechecks.
type PrecheckBackend interface {
	AgentVersion() (version.Number, error)
	NeedsCleanup() (bool, error)
	Model() (PrecheckModel, error)
	AllModelUUIDs() ([]string, error)
	IsUpgrading() (bool, error)
	IsMigrationActive(string) (bool, error)
	AllMachines() ([]PrecheckMachine, error)
	AllApplications() ([]PrecheckApplication, error)
	AllRelations() ([]PrecheckRelation, error)
	ControllerBackend() (PrecheckBackend, error)
	CloudCredential(tag names.CloudCredentialTag) (state.Credential, error)
	ListPendingResources(string) ([]resource.Resource, error)
}

// Pool defines the interface to a StatePool used by the migration
// prechecks.
type Pool interface {
	GetModel(string) (PrecheckModel, func(), error)
}

// PrecheckModel describes the state interface a model as needed by
// the migration prechecks.
type PrecheckModel interface {
	UUID() string
	Name() string
	Type() state.ModelType
	Owner() names.UserTag
	Life() state.Life
	MigrationMode() state.MigrationMode
	CloudCredential() (names.CloudCredentialTag, bool)
}

// PrecheckMachine describes the state interface for a machine needed
// by migration prechecks.
type PrecheckMachine interface {
	Id() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	Status() (status.StatusInfo, error)
	AgentPresence() (bool, error)
	InstanceStatus() (status.StatusInfo, error)
	ShouldRebootOrShutdown() (state.RebootAction, error)
}

// PrecheckApplication describes the state interface for an
// application needed by migration prechecks.
type PrecheckApplication interface {
	Name() string
	Life() state.Life
	CharmURL() (*charm.URL, bool)
	AllUnits() ([]PrecheckUnit, error)
	MinUnits() int
}

// PrecheckUnit describes state interface for a unit needed by
// migration prechecks.
type PrecheckUnit interface {
	Name() string
	AgentTools() (*tools.Tools, error)
	Life() state.Life
	CharmURL() (*charm.URL, bool)
	AgentStatus() (status.StatusInfo, error)
	Status() (status.StatusInfo, error)
	AgentPresence() (bool, error)
	ShouldBeAssigned() bool
}

// PrecheckRelation describes the state interface for relations needed
// for prechecks.
type PrecheckRelation interface {
	String() string
	IsCrossModel() (bool, error)
	Endpoints() []state.Endpoint
	Unit(PrecheckUnit) (PrecheckRelationUnit, error)
}

// PrecheckRelationUnit describes the interface for relation units
// needed for migration prechecks.
type PrecheckRelationUnit interface {
	Valid() (bool, error)
	InScope() (bool, error)
}

// ModelPresence represents the API server connections for a model.
type ModelPresence interface {
	// For a given non controller agent, return the Status for that agent.
	AgentStatus(agent string) (presence.Status, error)
}

// SourcePrecheck checks the state of the source controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the model to be migrated.
func SourcePrecheck(
	backend PrecheckBackend,
	modelPresence ModelPresence,
	controllerPresence ModelPresence,
) error {
	ctx := precheckContext{backend, modelPresence}
	if err := ctx.checkModel(); err != nil {
		return errors.Trace(err)
	}

	if err := ctx.checkMachines(); err != nil {
		return errors.Trace(err)
	}

	appUnits, err := ctx.checkApplications()
	if err != nil {
		return errors.Trace(err)
	}

	if err := ctx.checkRelations(appUnits); err != nil {
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
	controllerCtx := precheckContext{controllerBackend, controllerPresence}
	if err := controllerCtx.checkController(); err != nil {
		return errors.Annotate(err, "controller")
	}
	return nil
}

type precheckContext struct {
	backend  PrecheckBackend
	presence ModelPresence
}

func (ctx *precheckContext) checkModel() error {
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
	if credTag, found := model.CloudCredential(); found {
		creds, err := ctx.backend.CloudCredential(credTag)
		if err != nil {
			return errors.Trace(err)
		}
		if creds.Revoked {
			return errors.New("model has revoked credentials")
		}
	}
	return nil
}

// TargetPrecheck checks the state of the target controller to make
// sure that the preconditions for model migration are met. The
// backend provided must be for the target controller.
func TargetPrecheck(backend PrecheckBackend, pool Pool, modelInfo coremigration.ModelInfo, presence ModelPresence) error {
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

	controllerCtx := precheckContext{backend, presence}
	if err := controllerCtx.checkController(); err != nil {
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

func (ctx *precheckContext) checkController() error {
	model, err := ctx.backend.Model()
	if err != nil {
		return errors.Annotate(err, "retrieving model")
	}
	if model.Life() != state.Alive {
		return errors.Errorf("model is %s", model.Life())
	}

	if upgrading, err := ctx.backend.IsUpgrading(); err != nil {
		return errors.Annotate(err, "checking for upgrades")
	} else if upgrading {
		return errors.New("upgrade in progress")
	}

	return errors.Trace(ctx.checkMachines())
}

func (ctx *precheckContext) checkMachines() error {
	modelVersion, err := ctx.backend.AgentVersion()
	if err != nil {
		return errors.Annotate(err, "retrieving model version")
	}

	machines, err := ctx.backend.AllMachines()
	if err != nil {
		return errors.Annotate(err, "retrieving machines")
	}
	modelPresenceContext := common.ModelPresenceContext{ctx.presence}
	for _, machine := range machines {
		if machine.Life() != state.Alive {
			return errors.Errorf("machine %s is %s", machine.Id(), machine.Life())
		}

		if statusInfo, err := machine.InstanceStatus(); err != nil {
			return errors.Annotatef(err, "retrieving machine %s instance status", machine.Id())
		} else if statusInfo.Status != status.Running {
			return newStatusError("machine %s not running", machine.Id(), statusInfo.Status)
		}

		if statusInfo, err := modelPresenceContext.MachineStatus(machine); err != nil {
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

func (ctx *precheckContext) checkApplications() (map[string][]PrecheckUnit, error) {
	modelVersion, err := ctx.backend.AgentVersion()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving model version")
	}
	apps, err := ctx.backend.AllApplications()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving applications")
	}

	model, err := ctx.backend.Model()
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
		err = ctx.checkUnits(app, units, modelVersion, model.Type())
		if err != nil {
			return nil, errors.Trace(err)
		}
		appUnits[app.Name()] = units
	}
	return appUnits, nil
}

func (ctx *precheckContext) checkUnits(app PrecheckApplication, units []PrecheckUnit, modelVersion version.Number, modelType state.ModelType) error {
	if len(units) < app.MinUnits() {
		return errors.Errorf("application %s is below its minimum units threshold", app.Name())
	}

	appCharmURL, _ := app.CharmURL()

	for _, unit := range units {
		if unit.Life() != state.Alive {
			return errors.Errorf("unit %s is %s", unit.Name(), unit.Life())
		}

		if err := ctx.checkUnitAgentStatus(unit); err != nil {
			return errors.Trace(err)
		}

		if modelType == state.ModelTypeIAAS {
			if err := checkAgentTools(modelVersion, unit, "unit "+unit.Name()); err != nil {
				return errors.Trace(err)
			}
		}

		unitCharmURL, _ := unit.CharmURL()
		if appCharmURL.String() != unitCharmURL.String() {
			return errors.Errorf("unit %s is upgrading", unit.Name())
		}
	}
	return nil
}

func (ctx *precheckContext) checkUnitAgentStatus(unit PrecheckUnit) error {
	modelPresenceContext := common.ModelPresenceContext{ctx.presence}
	statusData, _ := modelPresenceContext.UnitStatus(unit)
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

type agentToolsGetter interface {
	AgentTools() (*tools.Tools, error)
}

func newStatusError(format, id string, s status.Status) error {
	msg := fmt.Sprintf(format, id)
	if s != status.Empty {
		msg += fmt.Sprintf(" (%s)", s)
	}
	return errors.New(msg)
}

func (ctx *precheckContext) checkRelations(appUnits map[string][]PrecheckUnit) error {
	relations, err := ctx.backend.AllRelations()
	if err != nil {
		return errors.Annotate(err, "retrieving model relations")
	}
	for _, rel := range relations {
		// We expect a relationScope and settings for each of the
		// units of the specified application, unless it is a
		// remote application.
		crossModel, err := rel.IsCrossModel()
		if err != nil {
			return errors.Annotatef(err, "checking whether relation %s is cross-model", rel)
		}
		if crossModel {
			continue
		}
		for _, ep := range rel.Endpoints() {
			for _, unit := range appUnits[ep.ApplicationName] {
				ru, err := rel.Unit(unit)
				if err != nil {
					return errors.Trace(err)
				}
				valid, err := ru.Valid()
				if err != nil {
					return errors.Trace(err)
				}
				if !valid {
					continue
				}
				inScope, err := ru.InScope()
				if err != nil {
					return errors.Trace(err)
				}
				if !inScope {
					return errors.Errorf("unit %s hasn't joined relation %s yet", unit.Name(), rel)
				}
			}
		}
	}
	return nil
}
