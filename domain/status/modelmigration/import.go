// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v11"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/status/service"
	statecontroller "github.com/juju/juju/domain/status/state/controller"
	statemodel "github.com/juju/juju/domain/status/state/model"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(
	coordinator Coordinator,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		clock:  clock,
		logger: logger,
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	serviceGetter func(model.UUID) ImportService

	clock  clock.Clock
	logger logger.Logger
}

// ImportService provides a subset of the status domain service methods needed
// for importing status.
type ImportService interface {
	// SetMachineStatus sets the status of the specified machine.
	SetMachineStatus(context.Context, coremachine.Name, corestatus.StatusInfo) error

	// SetInstanceStatus sets the cloud specific instance status for this machine.
	SetInstanceStatus(context.Context, coremachine.Name, corestatus.StatusInfo) error

	// SetApplicationStatus saves the given application status, overwriting any
	// current status data.
	SetApplicationStatus(context.Context, string, corestatus.StatusInfo) error

	// SetUnitWorkloadStatus sets the workload status of the specified unit.
	SetUnitWorkloadStatus(context.Context, coreunit.Name, corestatus.StatusInfo) error

	// SetUnitAgentStatus sets the agent status of the specified unit.
	SetUnitAgentStatus(context.Context, coreunit.Name, corestatus.StatusInfo) error

	// ImportRelationStatus saves the given relation status, overwriting any
	// current status data. If returns an error satisfying
	// [statuserrors.RelationNotFound] if the relation doesn't exist.
	ImportRelationStatus(context.Context, int, corestatus.StatusInfo) error

	// SetRemoteApplicationOffererStatus sets the status of the specified remote
	// application in the local model.
	SetRemoteApplicationOffererStatus(context.Context, string, corestatus.StatusInfo) error

	// SetFilesystemStatus validates and sets the given filesystem status, overwriting any
	// current status data.
	SetFilesystemStatus(context.Context, string, corestatus.StatusInfo) error

	// SetVolumeStatus validates and sets the given volume status, overwriting any
	// current status data.
	SetVolumeStatus(context.Context, string, corestatus.StatusInfo) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import status"
}

// Setup the import operation.
// This will create a new service instance.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.serviceGetter = func(modelUUID model.UUID) ImportService {
		return service.NewService(
			statemodel.NewModelState(scope.ModelDB(), i.clock, i.logger),
			statecontroller.NewControllerState(scope.ControllerDB(), modelUUID),
			clusterDescriber{},
			// TODO(jack): This is currently the wrong logger. We should
			// construct the StatusHistory using the model logger, however, at
			// the moment, we cannot get the model logger until the model has
			// been imported. Once this has changed, refactor this to use the
			// model logger.
			domain.NewStatusHistory(i.logger, i.clock),
			func() (service.StatusHistoryReader, error) {
				return nil, errors.Errorf("status history reader not available")
			},
			i.clock,
			i.logger,
		)
	}
	return nil
}

// Execute the import, loading the statuses of the various entities out of the
// description representation, into the domain.
func (i *importOperation) Execute(ctx context.Context, m description.Model) error {
	modelUUID := model.UUID(m.UUID())
	service := i.serviceGetter(modelUUID)

	err := i.importMachineStatus(ctx, service, m)
	if err != nil {
		return errors.Errorf("importing machine status: %w", err)
	}

	err = i.importApplicationAndUnitStatus(ctx, service, m)
	if err != nil {
		return errors.Errorf("importing application and unit status: %w", err)
	}

	err = i.importRelationStatus(ctx, service, m)
	if err != nil {
		return errors.Errorf("importing relation status: %w", err)
	}

	err = i.importRemoteApplicationOffererStatus(ctx, service, m)
	if err != nil {
		return errors.Errorf("importing remote application offerer status: %w", err)
	}

	err = i.importFilesystemStatus(ctx, service, m)
	if err != nil {
		return errors.Errorf("importing filesystem status: %w", err)
	}

	err = i.importVolumeStatus(ctx, service, m)
	if err != nil {
		return errors.Errorf("importing volume status: %w", err)
	}

	return nil
}

func (i *importOperation) importMachineStatus(
	ctx context.Context,
	service ImportService,
	m description.Model,
) error {
	for _, machine := range m.Machines() {
		machineName := coremachine.Name(machine.Id())
		machineStatus := i.importStatus(machine.Status())
		instanceStatus := i.importStatus(machine.Instance().Status())

		if err := service.SetMachineStatus(ctx, machineName, machineStatus); err != nil {
			return errors.Errorf("setting status for machine %q: %w", machineName, err)
		}

		if err := service.SetInstanceStatus(ctx, machineName, instanceStatus); err != nil {
			return errors.Errorf("setting instance status for machine %q: %w", machineName, err)
		}
	}

	return nil
}

func (i *importOperation) importApplicationAndUnitStatus(
	ctx context.Context,
	service ImportService,
	m description.Model,
) error {
	for _, app := range m.Applications() {
		appStatus := i.importStatus(app.Status())
		if err := service.SetApplicationStatus(ctx, app.Name(), appStatus); err != nil {
			return err
		}

		for _, unit := range app.Units() {
			unitName, err := coreunit.NewName(unit.Name())
			if err != nil {
				return err
			}
			unitAgentStatus := i.importStatus(unit.AgentStatus())
			if err := service.SetUnitAgentStatus(ctx, unitName, unitAgentStatus); err != nil {
				return errors.Errorf("setting agent status for unit %q: %w", unitName, err)
			}

			unitWorkloadStatus := i.importStatus(unit.WorkloadStatus())
			if err := service.SetUnitWorkloadStatus(ctx, unitName, unitWorkloadStatus); err != nil {
				return errors.Errorf("setting workload status for unit %q: %w", unitName, err)
			}
		}
	}

	return nil
}

func (i *importOperation) importRelationStatus(
	ctx context.Context,
	service ImportService,
	model description.Model,
) error {
	remoteApplications := model.RemoteApplications()
	for _, relation := range model.Relations() {
		if isRemoteConsumerRelation(relation, remoteApplications) {
			// Remote consumer relations are imported as part of the
			// crossmodelrelation domain, so we skip them here.
			continue
		}

		relationStatus := i.importStatus(relation.Status())
		if err := service.ImportRelationStatus(ctx, relation.Id(), relationStatus); err != nil {
			return errors.Errorf("importing status for relation %d: %w", relation.Id(), err)
		}
	}
	return nil
}

func (i *importOperation) importRemoteApplicationOffererStatus(
	ctx context.Context,
	service ImportService,
	model description.Model,
) error {
	for _, remoteApp := range model.RemoteApplications() {
		// Skip remote applications, we only want offerers here.
		if remoteApp.IsConsumerProxy() {
			continue
		}

		offererStatus := i.importStatus(remoteApp.Status())
		if err := service.SetRemoteApplicationOffererStatus(ctx, remoteApp.Name(), offererStatus); err != nil {
			return errors.Errorf("setting offerer status for remote application %q: %w", remoteApp.Name(), err)
		}
	}

	return nil
}

func (i *importOperation) importFilesystemStatus(
	ctx context.Context,
	service ImportService,
	model description.Model,
) error {
	for _, fs := range model.Filesystems() {
		fsStatus := i.importStatus(fs.Status())
		if err := service.SetFilesystemStatus(ctx, fs.ID(), fsStatus); err != nil {
			return errors.Errorf("setting status for filesystem %q: %w", fs.ID(), err)
		}
	}
	return nil
}

func (i *importOperation) importVolumeStatus(
	ctx context.Context,
	service ImportService,
	model description.Model,
) error {
	for _, vol := range model.Volumes() {
		volStatus := i.importStatus(vol.Status())
		if err := service.SetVolumeStatus(ctx, vol.ID(), volStatus); err != nil {
			return errors.Errorf("setting status for volume %q: %w", vol.ID(), err)
		}
	}
	return nil
}

func (i *importOperation) importStatus(s description.Status) corestatus.StatusInfo {
	// Older versions of Juju would pass through NeverSet() on the status
	// description for application statuses that hadn't been explicitly
	// set by the lead unit. If that is the case, we make the status what
	// the new code expects.
	if s == nil || s.NeverSet() {
		now := i.clock.Now()
		return corestatus.StatusInfo{
			Status: corestatus.Unset,
			Since:  &now,
		}
	}

	return corestatus.StatusInfo{
		Status:  corestatus.Status(s.Value()),
		Message: s.Message(),
		Data:    s.Data(),
		Since:   ptr(s.Updated()),
	}
}

func isRemoteConsumerRelation(rel description.Relation, remoteApps []description.RemoteApplication) bool {
	// If there are no remote applications, then there can't be any remote
	// relations.
	if len(remoteApps) == 0 {
		return false
	}

	// The only way to really know if a relation is a remote relation is to
	// cross reference the relation endpoints with the remote applications. If
	// any of the relation endpoints belong to a remote application, that is
	// a consumer proxy remote application, then we return true.
	remoteConsumers := make(map[string]struct{})
	for _, app := range remoteApps {
		if !app.IsConsumerProxy() {
			continue
		}

		remoteConsumers[app.Name()] = struct{}{}
	}

	for _, endpoint := range rel.Endpoints() {
		appName := endpoint.ApplicationName()
		if _, ok := remoteConsumers[appName]; ok {
			return true
		}
	}

	return false
}

func ptr[T any](v T) *T {
	return &v
}

type clusterDescriber struct{}

// ClusterDetails returns the details of the dqlite cluster nodes. For
// migrations it's ok that this is a no-op.
func (c clusterDescriber) ClusterDetails(ctx context.Context) ([]database.ClusterNodeInfo, error) {
	return nil, nil
}
