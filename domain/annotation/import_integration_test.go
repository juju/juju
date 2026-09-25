// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotation_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v12"
	"github.com/juju/tc"

	"github.com/juju/juju/core/annotations"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	annotationmodelmigration "github.com/juju/juju/domain/annotation/modelmigration"
	annotationservice "github.com/juju/juju/domain/annotation/service"
	annotationstate "github.com/juju/juju/domain/annotation/state"
	applicationmodelmigration "github.com/juju/juju/domain/application/modelmigration"
	machinemodelmigration "github.com/juju/juju/domain/machine/modelmigration"
	migrationtesting "github.com/juju/juju/domain/modelmigration/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	schematesting.ModelSuite

	scope modelmigration.Scope
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.scope = modelmigration.NewScope(nil, s.TxnRunnerFactory(), nil, nil, model.UUID(s.ModelUUID()))

	c.Cleanup(func() {
		s.scope = modelmigration.Scope{}
	})
}

// TestImportAnnotations checks that annotations on every entity kind
// (model, machine, application and unit) survive a
// complete import, for both IAAS and CAAS model types. The exact maps
// are compared through the annotation service after the migration has
// run.
func (s *importSuite) TestImportAnnotations(c *tc.C) {
	for _, modelType := range []model.ModelType{model.IAAS, model.CAAS} {
		// The model database is shared across the loop, so qualify the
		// application and unit names with the model type to keep the two
		// imports independent.
		qualifier := modelType.String()
		desc := description.NewModel(description.ModelArgs{
			Type:   qualifier,
			Config: map[string]any{"uuid": s.ModelUUID()},
		})
		want := map[annotations.ID]map[string]string{
			{Kind: annotations.KindModel, Name: desc.UUID()}: {
				"owner": "model-owner", "purpose": "production",
			},
			{Kind: annotations.KindApplication, Name: "client-" + qualifier}: {
				"owner": "application-owner", "team": "database",
			},
			{Kind: annotations.KindUnit, Name: "client-" + qualifier + "/0"}: {
				"owner": "unit-owner", "ticket": "42",
			},
		}
		desc.SetAnnotations(want[annotations.ID{Kind: annotations.KindModel, Name: desc.UUID()}])

		machineName := ""
		if modelType == model.IAAS {
			machineName = "0"
			machine := desc.AddMachine(description.MachineArgs{Id: "0", Base: "ubuntu@22.04"})
			container := machine.AddContainer(description.MachineArgs{
				Id: "0/lxd/0", Base: "ubuntu@22.04", ContainerType: "lxd",
			})
			for _, m := range []description.Machine{machine, container} {
				id := annotations.ID{Kind: annotations.KindMachine, Name: m.Id()}
				want[id] = map[string]string{"owner": "machine-" + m.Id(), "rack": "rack-3"}
				m.SetAnnotations(want[id])
			}
		}

		name := "client-" + qualifier
		app := desc.AddApplication(description.ApplicationArgs{
			Name: name, CharmURL: "ch:client-1",
		})
		// The application import also imports the charm, so origin, metadata
		// and manifest must be present for it to succeed.
		app.SetCharmOrigin(description.CharmOriginArgs{
			Source: "charm-hub", ID: "client-id", Hash: "client-hash",
			Revision: 1, Channel: "latest/stable", Platform: "amd64/ubuntu/22.04",
		})
		app.SetCharmMetadata(description.CharmMetadataArgs{Name: "client"})
		app.SetCharmManifest(description.CharmManifestArgs{
			Bases: []description.CharmManifestBase{migrationtesting.ManifestBase{
				Name_: "ubuntu", Channel_: "22.04/stable", Architectures_: []string{"amd64"},
			}},
		})
		app.SetAnnotations(want[annotations.ID{Kind: annotations.KindApplication, Name: name}])
		unit := app.AddUnit(description.UnitArgs{
			Name: name + "/0", Type: qualifier, Machine: machineName,
		})
		unit.SetAnnotations(want[annotations.ID{Kind: annotations.KindUnit, Name: unit.Name()}])

		// Use the production operations that the annotation import depends
		// on, in their production order: machines, then applications (which
		// import units), then annotations.
		coordinator := modelmigration.NewCoordinator(loggertesting.WrapCheckLog(c))
		machinemodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
		applicationmodelmigration.RegisterImport(coordinator, clock.WallClock, loggertesting.WrapCheckLog(c))
		annotationmodelmigration.RegisterImport(coordinator)
		c.Assert(coordinator.Perform(c.Context(), s.scope, desc), tc.ErrorIsNil)

		svc := annotationservice.NewService(annotationstate.NewState(s.scope.ModelDB()))
		for id, expected := range want {
			got, err := svc.GetAnnotations(c.Context(), id)
			c.Assert(err, tc.ErrorIsNil)
			c.Check(got, tc.DeepEquals, expected, tc.Commentf("annotations for %s", id))
		}
	}
}
