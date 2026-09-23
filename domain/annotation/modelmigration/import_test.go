// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/internal/errors"
)

type importSuite struct{}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportEmptyAnnotations(c *tc.C) {
	service := NewMockImportService(gomock.NewController(c))
	desc := description.NewModel(description.ModelArgs{})
	desc.AddMachine(description.MachineArgs{Id: "0"})
	app := desc.AddApplication(description.ApplicationArgs{Name: "client"})
	app.AddUnit(description.UnitArgs{Name: "client/0"})

	// No calls are made for entities without annotations.
	op := &importOperation{service: service}
	c.Assert(op.Execute(c.Context(), desc), tc.ErrorIsNil)
}

func (s *importSuite) TestImportParentAndChildAnnotations(c *tc.C) {
	service := NewMockImportService(gomock.NewController(c))
	desc := description.NewModel(description.ModelArgs{})
	machine := desc.AddMachine(description.MachineArgs{Id: "0"})
	machineAnnotations := map[string]string{"zone": "zone-1"}
	machine.SetAnnotations(machineAnnotations)
	container := machine.AddContainer(description.MachineArgs{Id: "0/lxd/0"})
	containerAnnotations := map[string]string{"rack": "rack-3"}
	container.SetAnnotations(containerAnnotations)
	app := desc.AddApplication(description.ApplicationArgs{Name: "client"})
	unit := app.AddUnit(description.UnitArgs{Name: "client/0"})
	unitAnnotations := map[string]string{"ticket": "42"}
	unit.SetAnnotations(unitAnnotations)

	service.EXPECT().SetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindMachine, Name: "0",
	}, machineAnnotations).Return(nil)
	service.EXPECT().SetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindMachine, Name: "0/lxd/0",
	}, containerAnnotations).Return(nil)
	service.EXPECT().SetAnnotations(gomock.Any(), annotations.ID{
		Kind: annotations.KindUnit, Name: "client/0",
	}, unitAnnotations).Return(nil)
	op := &importOperation{service: service}
	c.Assert(op.Execute(c.Context(), desc), tc.ErrorIsNil)
}

func (s *importSuite) TestImportErrors(c *tc.C) {
	for _, kind := range []annotations.Kind{
		annotations.KindModel, annotations.KindMachine,
		annotations.KindApplication, annotations.KindUnit,
	} {
		service := NewMockImportService(gomock.NewController(c))
		desc := description.NewModel(description.ModelArgs{
			Config: map[string]any{"uuid": "model-uuid"},
		})
		values := map[string]string{"owner": "test-owner"}
		var name string
		switch kind {
		case annotations.KindModel:
			name = desc.UUID()
			desc.SetAnnotations(values)
		case annotations.KindMachine:
			name = "0/lxd/0"
			machine := desc.AddMachine(description.MachineArgs{Id: "0"})
			machine.AddContainer(description.MachineArgs{Id: name}).SetAnnotations(values)
		case annotations.KindApplication:
			name = "client"
			desc.AddApplication(description.ApplicationArgs{Name: name}).SetAnnotations(values)
		case annotations.KindUnit:
			name = "client/0"
			app := desc.AddApplication(description.ApplicationArgs{Name: "client"})
			app.AddUnit(description.UnitArgs{Name: name}).SetAnnotations(values)
		}
		failure := errors.ConstError("cannot persist annotations")
		service.EXPECT().SetAnnotations(gomock.Any(), annotations.ID{
			Kind: kind, Name: name,
		}, values).Return(failure)

		op := &importOperation{service: service}
		err := op.Execute(c.Context(), desc)
		c.Check(err, tc.ErrorIs, failure)
		c.Check(err, tc.ErrorMatches, `importing annotations for \d+/.*: cannot persist annotations`)
	}
}
