// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"context"
	"errors"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	apiservertesting "github.com/juju/juju/apiserver/testing"
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
	return s.mustNewAPIV8WithMinter(c, nil)
}

func (s *v8Suite) mustNewAPIV8WithMinter(c *tc.C, minter facade.LocalMacaroonMinter) *migrationtarget.APIV8 {
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

	apiV8, err := migrationtarget.NewAPIV8(api, minter)
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

// TestPrechecksNoOpSuccess verifies that v8 Prechecks accepts any envelope
// and returns success: the real precheck routine is not implemented yet.
func (s *v8Suite) TestPrechecksNoOpSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope())
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportNoOpSuccess verifies that v8 Import accepts any envelope and
// returns success without importing anything: the real import path is not
// implemented yet.
func (s *v8Suite) TestImportNoOpSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.mustNewAPIV8(c).Import(c.Context(), s.makeEnvelope())
	c.Assert(err, tc.ErrorIsNil)
}

// TestCreateMigrationMacaroonSuccess verifies v8 delegates macaroon minting
// for the authenticated local user using the latest bakery version.
func (s *v8Suite) TestCreateMigrationMacaroonSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	mac := newMacaroon(c)
	minter := &testLocalMacaroonMinter{result: mac}
	ctx := c.Context()

	result, err := s.mustNewAPIV8WithMinter(c, minter).CreateMigrationMacaroon(ctx)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(result.Macaroon, tc.Equals, mac)
	c.Check(minter.calls, tc.Equals, 1)
	c.Check(minter.ctx, tc.Equals, ctx)
	c.Check(minter.tag, tc.Equals, names.NewUserTag("fred"))
	c.Check(minter.version, tc.Equals, bakery.LatestVersion)
}

func (s *v8Suite) TestCreateMigrationMacaroonMinterError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedErr := errors.New("mint failed")
	minter := &testLocalMacaroonMinter{err: expectedErr}

	result, err := s.mustNewAPIV8WithMinter(c, minter).CreateMigrationMacaroon(c.Context())
	c.Assert(err, tc.ErrorIs, expectedErr)

	c.Check(result.Macaroon, tc.IsNil)
	c.Check(minter.calls, tc.Equals, 1)
}

func (s *v8Suite) TestCreateMigrationMacaroonNonUserPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.Tag = names.NewMachineTag("0")
	minter := &testLocalMacaroonMinter{}

	result, err := s.mustNewAPIV8WithMinter(c, minter).CreateMigrationMacaroon(c.Context())
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)

	c.Check(result.Macaroon, tc.IsNil)
	c.Check(minter.calls, tc.Equals, 0)
}

func (s *v8Suite) TestCreateMigrationMacaroonRemoteUserPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.Tag = names.NewUserTag("fred@external")
	minter := &testLocalMacaroonMinter{}

	result, err := s.mustNewAPIV8WithMinter(c, minter).CreateMigrationMacaroon(c.Context())
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)

	c.Check(result.Macaroon, tc.IsNil)
	c.Check(minter.calls, tc.Equals, 0)
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

type testLocalMacaroonMinter struct {
	result  *macaroon.Macaroon
	err     error
	calls   int
	ctx     context.Context
	tag     names.UserTag
	version bakery.Version
}

func (m *testLocalMacaroonMinter) CreateMigrationMacaroon(
	ctx context.Context,
	tag names.UserTag,
	version bakery.Version,
) (*macaroon.Macaroon, error) {
	m.calls++
	m.ctx = ctx
	m.tag = tag
	m.version = version
	return m.result, m.err
}

func newMacaroon(c *tc.C) *macaroon.Macaroon {
	mac, err := macaroon.New(
		[]byte("root key"),
		[]byte("id"),
		"migration target",
		macaroon.LatestVersion,
	)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
