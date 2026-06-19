// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	"github.com/juju/juju/domain/export"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

// modelImporterV2Suite is a thin smoke test for ModelImporter.ImportModelV2,
// the public method the migrationtarget facade calls. The orchestration
// itself (decode, claim, bootstrap, per-domain controller-data writes) is
// exhaustively covered against the same real databases by
// internal/migration/v2's test suite, which calls migrationv2.ImportModel
// directly; this only proves the delegator resolves the migration scope for
// the envelope's model UUID and wires it through correctly.
type modelImporterV2Suite struct {
	schematesting.ControllerModelSuite

	cloudName string
}

func TestModelImporterV2Suite(t *testing.T) {
	tc.Run(t, &modelImporterV2Suite{})
}

func (s *modelImporterV2Suite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	controllerModelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "controller")
	s.SeedControllerTable(c, controllerModelUUID)

	adminUserUUID := tc.Must(c, coreuser.NewUUID)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	err := accessState.AddUser(c.Context(), adminUserUUID, coreuser.AdminUserName, coreuser.AdminUserName.Name(), false, adminUserUUID)
	c.Assert(err, tc.ErrorIsNil)

	s.cloudName = "test-cloud"
	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      s.cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})
	err = fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	modeltesting.CreateInternalSecretBackend(c, s.ControllerTxnRunner())
}

func (s *modelImporterV2Suite) TestImportModelV2(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	controllerFactory := s.TxnRunnerFactory()
	modelRunner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return modelRunner, nil
	}

	scope := func(coremodel.UUID) coremodelmigration.Scope {
		return coremodelmigration.NewScope(controllerFactory, modelFactory, nil, nil, modelUUID)
	}
	importer := migration.NewModelImporter(scope, nil, "controller-uuid", loggertesting.WrapCheckLog(c), clock.WallClock)

	envelope := params.SerializedModelV2{
		PayloadVersion: semversion.MustParse("4.0.6"),
		ModelInfo: params.SerializedModelInfo{
			UUID:                modelUUID.String(),
			Name:                "imported-model",
			Qualifier:           "prod",
			Type:                "iaas",
			Cloud:               s.cloudName,
			Life:                "alive",
			SourceMigrationUUID: uuid.MustNewUUID().String(),
		},
	}
	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}

	err := importer.ImportModelV2(c.Context(), envelope, view)
	c.Assert(err, tc.ErrorIsNil)

	// The claim landed against the same controller DB the scope resolved to.
	claimSt := migrationclaimstate.New(controllerFactory, clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.SourceMigrationUUID, tc.Equals, envelope.ModelInfo.SourceMigrationUUID)

	// A second call against the same scope is rejected as a duplicate claim,
	// proving the delegator re-resolves the scope per call rather than
	// caching stale state.
	err = importer.ImportModelV2(c.Context(), envelope, view)
	c.Check(err, tc.ErrorIs, coreerrors.AlreadyExists)
}
