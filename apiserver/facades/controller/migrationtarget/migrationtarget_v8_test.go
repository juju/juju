// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type v8Suite struct {
	authorizer *apiservertesting.FakeAuthorizer

	cloudService              *MockCloudService
	controllerConfigService   *MockControllerConfigService
	externalControllerService *MockExternalControllerService
	modelService              *MockModelService
	upgradeService            *MockUpgradeService
	statusService             *MockStatusService
	machineService            *MockMachineService
	modelImporter             *MockModelImporter
	modelMigrationService     *MockModelMigrationService
	agentService              *MockModelAgentService
	removalService            *MockRemovalService

	modelUUID string

	facadeContext facadetest.ModelContext
}

func TestV8Suite(t *testing.T) {
	tc.Run(t, &v8Suite{})
}

func (s *v8Suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.cloudService = NewMockCloudService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.externalControllerService = NewMockExternalControllerService(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.upgradeService = NewMockUpgradeService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.modelImporter = NewMockModelImporter(ctrl)
	s.modelMigrationService = NewMockModelMigrationService(ctrl)
	s.agentService = NewMockModelAgentService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)

	s.modelUUID = uuid.MustNewUUID().String()

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:      names.NewUserTag("fred"),
		AdminTag: names.NewUserTag("fred"),
	}
	s.facadeContext = facadetest.ModelContext{
		Auth_:          s.authorizer,
		ModelImporter_: s.modelImporter,
	}

	return ctrl
}

func (s *v8Suite) mustNewAPIV8(c *tc.C) *migrationtarget.APIV8 {
	api, err := migrationtarget.NewAPI(
		&s.facadeContext,
		s.authorizer,
		s.cloudService,
		s.controllerConfigService,
		s.externalControllerService,
		s.modelService,
		s.upgradeService,
		s.statusService,
		s.machineService,
		func(context.Context, model.UUID) (migrationtarget.ModelAgentService, error) {
			return s.agentService, nil
		},
		func(context.Context, model.UUID) (migrationtarget.ModelMigrationService, error) {
			return s.modelMigrationService, nil
		},
		func(context.Context, model.UUID) (migrationtarget.RemovalService, error) {
			return s.removalService, nil
		},
		facades.FacadeVersions{},
		c.MkDir(),
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)

	apiV8, err := migrationtarget.NewAPIV8(api)
	c.Assert(err, tc.ErrorIsNil)
	return apiV8
}

func (s *v8Suite) makeEnvelope() params.SerializedModelV2 {
	return params.SerializedModelV2{
		PayloadVersion: semversion.MustParse("4.0.6"),
		ModelInfo: params.SerializedModelInfo{
			UUID:                s.modelUUID,
			Name:                "some-model",
			Qualifier:           "prod",
			Type:                "iaas",
			Cloud:               "my-cloud",
			CloudRegion:         "my-region",
			Life:                "alive",
			SourceMigrationUUID: uuid.MustNewUUID().String(),
		},
	}
}

// TestPrechecksInvalidModelInfo verifies the envelope identity validation:
// bad model UUID, empty name, empty qualifier and empty source migration
// UUID are all rejected before any service call.
func (s *v8Suite) TestPrechecksInvalidModelInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.mustNewAPIV8(c)

	valid := s.makeEnvelope()

	envelope := valid
	envelope.ModelInfo.UUID = "not-a-uuid"
	err := api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, `model UUID "not-a-uuid" not valid`)

	envelope = valid
	envelope.ModelInfo.Name = ""
	err = api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, `empty model name not valid`)

	envelope = valid
	envelope.ModelInfo.Qualifier = ""
	err = api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, `empty model qualifier not valid`)

	envelope = valid
	envelope.ModelInfo.SourceMigrationUUID = ""
	err = api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, `empty source migration UUID not valid`)
}

// TestPrechecksNoOpSuccess verifies the v8 Prechecks shell accepts a valid
// envelope and reports success without calling any service: the real precheck
// routine is not implemented yet.
func (s *v8Suite) TestPrechecksNoOpSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope())
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportNoOpSuccess verifies the v8 Import shell accepts a valid envelope
// and reports success without importing anything: the real import path is not
// implemented yet, and no service is called.
func (s *v8Suite) TestImportNoOpSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.mustNewAPIV8(c).Import(c.Context(), s.makeEnvelope())
	c.Assert(err, tc.ErrorIsNil)
}

// TestV8Registered asserts MigrationTarget v8 is advertised alongside the
// legacy versions so the new-path source worker can negotiate it.
func (s *v8Suite) TestV8Registered(c *tc.C) {
	for _, description := range apiserver.AllFacades().List() {
		if description.Name != "MigrationTarget" {
			continue
		}
		c.Assert(description.Versions, tc.DeepEquals, []int{4, 5, 6, 7, 8})
		return
	}
	c.Fatal("MigrationTarget facade not registered at all")
}
