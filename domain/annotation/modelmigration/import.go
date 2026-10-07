// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v12"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/annotation/service"
	"github.com/juju/juju/domain/annotation/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface used to register migration operations.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport registers annotation import. Models, machines, applications and
// units must be imported first so their names can be resolved to UUIDs.
func RegisterImport(coordinator Coordinator) {
	coordinator.Add(&importOperation{})
}

// ImportService persists annotations on imported entities.
type ImportService interface {
	SetAnnotations(context.Context, annotations.ID, map[string]string) error
}

type importOperation struct {
	modelmigration.BaseOperation
	service ImportService
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import annotations"
}

// Setup creates the annotation service for the model being imported.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ModelDB()))
	return nil
}

// Execute imports annotations for each entity kind exported by Juju 3.6.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	if err := i.importAnnotations(ctx, annotations.KindModel, model.UUID(), model.Annotations()); err != nil {
		return err
	}
	for _, machine := range model.Machines() {
		if err := i.importMachineAnnotations(ctx, machine); err != nil {
			return err
		}
	}
	for _, app := range model.Applications() {
		if err := i.importAnnotations(ctx, annotations.KindApplication, app.Name(), app.Annotations()); err != nil {
			return err
		}
		for _, unit := range app.Units() {
			if err := i.importAnnotations(ctx, annotations.KindUnit, unit.Name(), unit.Annotations()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (i *importOperation) importMachineAnnotations(ctx context.Context, machine description.Machine) error {
	if err := i.importAnnotations(ctx, annotations.KindMachine, machine.Id(), machine.Annotations()); err != nil {
		return err
	}
	for _, container := range machine.Containers() {
		if err := i.importMachineAnnotations(ctx, container); err != nil {
			return err
		}
	}
	return nil
}

func (i *importOperation) importAnnotations(ctx context.Context, kind annotations.Kind, name string, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	if err := i.service.SetAnnotations(ctx, annotations.ID{Kind: kind, Name: name}, values); err != nil {
		return errors.Errorf("importing annotations for %s: %w", annotations.ID{Kind: kind, Name: name}, err)
	}
	return nil
}
