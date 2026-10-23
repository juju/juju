// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"sync"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

// State supplies recovery persistence for one database.
type State interface {
	// Load methods atomically replace tables selected by service policy,
	// rebinding every object store placement to the supplied target node.
	LoadControllerDump(context.Context, *domainrecovery.ControllerDump, string) error
	LoadModelDump(context.Context, *domainrecovery.ModelDump, string) error
	// Object queries describe the files required by persisted metadata.
	GetObjectMetadataPaths(context.Context) ([]domainrecovery.ObjectMetadataPath, error)
	// Machine lookup returns the first live controller machine in name order.
	GetControllerMachineName(context.Context) (string, error)
	// Patches complete their writes before returning. Failure rolls back.
	PatchControllerMachine(context.Context, domainrecovery.MachinePatch) error
	PatchControllerUnitAddresses(context.Context, domainrecovery.ControllerUnitAddressPatch) error
	// Summary queries return persisted observations after recovery patches.
	GetOtherControllerMachines(context.Context, string) ([]string, error)
	GetRecoveryCounts(context.Context) (domainrecovery.RecoveryCounts, error)
}

// Service coordinates persistence during exclusive recovery bootstrap. The
// database session owns its runners and must outlive this service. Loading is
// sequential: controller, models, object installation, patches, then report.
// A failure can leave earlier databases committed; discard the recovery target.
type Service struct {
	controller State
	openModel  func(context.Context, string) (State, error)
	nodeID     string
	mu         sync.Mutex
	models     map[string]State
}

// NewService requires both dependencies before recovery can begin. The node
// ID identifies the provisioned replacement's object store node; every
// placement rebuilt during the loads rebinds to it.
func NewService(controller State, openModel func(context.Context, string) (State, error), nodeID string) (*Service, error) {
	if controller == nil || openModel == nil {
		return nil, errors.New("recovery requires controller state and a model state factory")
	}
	return &Service{controller: controller, openModel: openModel, nodeID: nodeID, models: make(map[string]State)}, nil
}

// GetObjectInventory retrieves the required files after all dumps are loaded.
func (s *Service) GetObjectInventory(ctx context.Context, models []coremodel.UUID) ([]domainrecovery.ObjectInventory, error) {
	controller, err := objectInventory(ctx, database.ControllerNS, s.controller)
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := []domainrecovery.ObjectInventory{controller}
	for _, model := range models {
		st, err := s.modelState(ctx, model)
		if err != nil {
			return nil, errors.Capture(err)
		}
		inventory, err := objectInventory(ctx, model.String(), st)
		if err != nil {
			return nil, errors.Errorf("model %q object inventory: %w", model, err)
		}
		result = append(result, inventory)
	}
	return result, nil
}

// PatchControllerMachine resolves the live archived machine and applies the
// replacement's physical facts, returning the chosen machine name.
func (s *Service) PatchControllerMachine(ctx context.Context, model coremodel.UUID, patch domainrecovery.MachinePatch) (string, error) {
	st, err := s.modelState(ctx, model)
	if err != nil {
		return "", errors.Capture(err)
	}
	name, err := st.GetControllerMachineName(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	patch.MachineName = name
	if err := st.PatchControllerMachine(ctx, patch); err != nil {
		return "", errors.Errorf("patching controller machine %q: %w", name, err)
	}
	return name, nil
}

// PatchControllerUnitAddresses prepares identifiers before persistence starts.
func (s *Service) PatchControllerUnitAddresses(ctx context.Context, model coremodel.UUID, values []string) error {
	st, err := s.modelState(ctx, model)
	if err != nil {
		return errors.Capture(err)
	}
	patch, err := NewControllerUnitAddressPatch(values)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(st.PatchControllerUnitAddresses(ctx, patch))
}

// BuildSummary aggregates persisted observations after patches are applied.
func (s *Service) BuildSummary(ctx context.Context, info *domainrecovery.ArchiveInfo, models []coremodel.UUID, machineName string, objectsCopied int) (*domainrecovery.Summary, error) {
	st, err := s.modelState(ctx, coremodel.UUID(info.ControllerModelUUID))
	if err != nil {
		return nil, errors.Capture(err)
	}
	dead, err := st.GetOtherControllerMachines(ctx, machineName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	summary := &domainrecovery.Summary{
		ControllerUUID: info.ControllerUUID, ControllerName: info.ControllerName,
		SourceAgentVersion: info.AgentVersion.String(), BackupFinished: info.BackupFinished,
		HANodes: info.HANodes, Models: len(models), ObjectsCopied: objectsCopied,
		DeadControllerMachines: dead,
	}
	for _, model := range models {
		st, err := s.modelState(ctx, model)
		if err != nil {
			return nil, errors.Capture(err)
		}
		counts, err := st.GetRecoveryCounts(ctx)
		if err != nil {
			return nil, errors.Errorf("model %q recovery counts: %w", model, err)
		}
		summary.MachinesNotAlive += counts.MachinesNotAlive
		summary.ApplicationsNotAlive += counts.ApplicationsNotAlive
		summary.UnitsNotAlive += counts.UnitsNotAlive
		summary.PendingRemovals += counts.PendingRemovals
	}
	return summary, nil
}

func (s *Service) modelState(ctx context.Context, model coremodel.UUID) (State, error) {
	if err := model.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Capture(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.models[model.String()]; ok {
		return st, nil
	}
	st, err := s.openModel(ctx, model.String())
	if err != nil {
		return nil, errors.Errorf("opening database for model %q: %w", model, err)
	}
	if st == nil {
		return nil, errors.Errorf("model %q state factory returned nil state", model)
	}
	s.models[model.String()] = st
	return st, nil
}

func objectInventory(ctx context.Context, namespace string, st State) (domainrecovery.ObjectInventory, error) {
	paths, err := st.GetObjectMetadataPaths(ctx)
	if err != nil {
		return domainrecovery.ObjectInventory{}, errors.Capture(err)
	}
	return domainrecovery.ObjectInventory{Namespace: namespace, Paths: paths}, nil
}
