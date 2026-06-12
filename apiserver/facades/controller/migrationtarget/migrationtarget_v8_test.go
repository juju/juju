// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/apiserver"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/migrationtarget"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/facades"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	accesserrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/domain/export"
	v4_0_6 "github.com/juju/juju/domain/export/types/v4_0_6"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
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
	userService               *MockUserService
	credentialService         *MockCredentialService
	secretBackendService      *MockSecretBackendService
	migrationImportService    *MockMigrationImportService

	controllerUUID string
	modelUUID      string

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
	s.userService = NewMockUserService(ctrl)
	s.credentialService = NewMockCredentialService(ctrl)
	s.secretBackendService = NewMockSecretBackendService(ctrl)
	s.migrationImportService = NewMockMigrationImportService(ctrl)

	s.controllerUUID = uuid.MustNewUUID().String()
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

	apiV8, err := migrationtarget.NewAPIV8(
		api,
		minter,
		s.controllerUUID,
		s.userService,
		s.credentialService,
		s.secretBackendService,
		s.migrationImportService,
	)
	c.Assert(err, tc.ErrorIsNil)
	return apiV8
}

// validPayload returns a v4_0_6 payload whose static checks pass: one
// application whose charm has a manifest base, no fan config, and an agent
// version below the target controller version used in tests.
func (s *v8Suite) validPayload() v4_0_6.ModelExport {
	return v4_0_6.ModelExport{
		AgentVersion: []v4_0_6.AgentVersion{{
			TargetVersion: "4.0.6",
		}},
		Application: []v4_0_6.Application{{
			UUID:      "app-uuid",
			Name:      "ubuntu",
			CharmUUID: "charm-uuid",
		}},
		CharmManifestBase: []v4_0_6.CharmManifestBase{{
			CharmUUID: "charm-uuid",
			Risk:      "stable",
		}},
	}
}

func (s *v8Suite) makeEnvelope(c *tc.C, payload v4_0_6.ModelExport) params.SerializedModelV2 {
	data, err := yaml.Marshal(payload)
	c.Assert(err, tc.ErrorIsNil)

	return params.SerializedModelV2{
		PayloadVersion: semversion.MustParse("4.0.6"),
		Payload:        data,
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

// expectSchemaReady primes the schema guard to pass.
func (s *v8Suite) expectSchemaReady() {
	s.migrationImportService.EXPECT().CheckTargetImportSchema(gomock.Any()).Return(nil).AnyTimes()
}

// expectControllerReady primes the target-controller readiness checks
// (upgrade in progress, controller machine health) and the target controller
// version used by the agent-version check.
func (s *v8Suite) expectControllerReady() {
	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(false, nil).AnyTimes()
	s.agentService.EXPECT().GetMachinesNotAtTargetAgentVersion(gomock.Any()).Return(nil, nil).AnyTimes()
	s.statusService.EXPECT().CheckMachineStatusesReadyForMigration(gomock.Any()).Return(nil).AnyTimes()
	s.machineService.EXPECT().AllMachineNames(gomock.Any()).Return(nil, nil).AnyTimes()
	s.agentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		semversion.MustParse("4.1.0"), nil).AnyTimes()
}

// expectCloud primes the cloud lookup with a cloud carrying the test region.
func (s *v8Suite) expectCloud() {
	s.cloudService.EXPECT().Cloud(gomock.Any(), "my-cloud").Return(&cloud.Cloud{
		Name:    "my-cloud",
		Regions: []cloud.Region{{Name: "my-region"}},
	}, nil).AnyTimes()
}

// expectNoCollisions primes the import-claim, model, namespace and model-list
// lookups so the collision checks pass.
func (s *v8Suite) expectNoCollisions() {
	s.migrationImportService.EXPECT().GetImportClaim(gomock.Any(), model.UUID(s.modelUUID)).Return(
		modelmigration.ImportClaim{}, modelmigrationerrors.ErrImportNotFound).AnyTimes()
	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(s.modelUUID)).Return(
		model.Model{}, modelerrors.NotFound).AnyTimes()
	s.migrationImportService.EXPECT().ModelNamespaceExists(gomock.Any(), model.UUID(s.modelUUID)).Return(
		false, nil).AnyTimes()
	s.modelService.EXPECT().GetAllModels(gomock.Any()).Return(nil, nil).AnyTimes()
}

func (s *v8Suite) expectHappyPath() {
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()
	s.expectNoCollisions()
}

// TestPrechecksSuccess runs the full precheck routine over a well-formed
// envelope: existing enabled user, matching existing credential, existing
// secret backend, an identical existing external controller plus a reference
// to the target controller itself (skipped), and no collisions.
func (s *v8Suite) TestPrechecksSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectHappyPath()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.Users = []params.ModelUser{
		{Name: "existing-user"},
		{Name: "missing-user"},
	}
	envelope.ModelCredential = &params.ModelCloudCredential{
		Cloud:      "my-cloud",
		Owner:      "existing-user",
		Name:       "foobar",
		AuthType:   "access-key",
		Attributes: map[string]string{"foo": "bar"},
	}
	envelope.SecretBackend = &params.ModelSecretBackend{Name: "myvault"}
	thirdPartyUUID := uuid.MustNewUUID().String()
	envelope.ExternalControllers = []params.ExternalControllerRef{
		{UUID: s.controllerUUID, CACert: "ignored"},
		{UUID: thirdPartyUUID, CACert: "ca-cert", Addresses: []string{"10.0.0.1:17070", "10.0.0.2:17070"}},
	}

	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "existing-user")).Return(
		user.User{Name: usertesting.GenNewName(c, "existing-user")}, nil)
	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "missing-user")).Return(
		user.User{}, accesserrors.UserNotFound)
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), gomock.Any()).Return(
		cloud.NewCredential("access-key", map[string]string{"foo": "bar"}), nil)
	s.secretBackendService.EXPECT().GetSecretBackendByName(gomock.Any(), "myvault").Return(
		&secretbackend.SecretBackend{ID: "backend-uuid", Name: "myvault"}, nil)
	// Addresses compare as sets, so a different order is not a conflict.
	s.externalControllerService.EXPECT().Controller(gomock.Any(), thirdPartyUUID).Return(
		&crossmodel.ControllerInfo{
			ControllerUUID: thirdPartyUUID,
			CACert:         "ca-cert",
			Addrs:          []string{"10.0.0.2:17070", "10.0.0.1:17070"},
		}, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIsNil)
}

// TestPrechecksInvalidModelInfo verifies the envelope identity validation:
// bad model UUID, empty name, empty qualifier and empty source migration
// UUID are all rejected before any service call.
func (s *v8Suite) TestPrechecksInvalidModelInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.mustNewAPIV8(c)

	valid := s.makeEnvelope(c, s.validPayload())

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

// TestPrechecksSchemaNotReady verifies the runtime schema guard error is
// surfaced before any payload work.
func (s *v8Suite) TestPrechecksSchemaNotReady(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.migrationImportService.EXPECT().CheckTargetImportSchema(gomock.Any()).Return(
		schemaNotReadyError())

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Assert(err, tc.ErrorMatches, "target schema not ready for migrationtarget v8.*")
}

// TestPrechecksPayloadTooLarge verifies that an oversized payload is rejected
// before any YAML decode: the same garbage bytes under the limit produce a
// decode error instead.
// TestPrechecksPayloadDecodeError verifies that a malformed payload is
// rejected by the decoder with a coded validation error.
func (s *v8Suite) TestPrechecksPayloadDecodeError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	api := s.mustNewAPIV8(c)

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.Payload = []byte("\t: garbage")
	err := api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Assert(err, tc.ErrorMatches, `decoding model export payload at version "4.0.6".*`)
}

// TestPrechecksPayloadVersionNewerThanTarget verifies the
// payload-version-ahead-of-target rejection with the actionable upgrade
// message, even for versions unknown to the decoder registry.
func (s *v8Suite) TestPrechecksPayloadVersionNewerThanTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.PayloadVersion = semversion.MustParse("9.9.9")

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf(
		`source payload version "9.9.9" is newer than target %q; upgrade the target controller first.*`,
		export.LatestSupportedPayloadVersion()))
}

// TestPrechecksPayloadVersionUnknown verifies that a version at or below the
// target but unknown to the decoder registry yields a clean error.
func (s *v8Suite) TestPrechecksPayloadVersionUnknown(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.PayloadVersion = semversion.MustParse("4.0.5")

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Assert(err, tc.ErrorMatches, `model export payload version "4.0.5" not supported`)
}

// TestPrechecksCharmWithoutManifest verifies the static charm-manifest check
// runs over the decoded payload.
func (s *v8Suite) TestPrechecksCharmWithoutManifest(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()

	payload := s.validPayload()
	payload.CharmManifestBase = nil

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, payload))
	c.Assert(err, tc.ErrorMatches,
		`migration import prechecks: checking model for charms without manifest.yaml: all charms now require a manifest.yaml file, this model hosts charm\(s\) with no manifest.yaml file: ubuntu`)
}

// TestPrechecksFanConfig verifies the static fan-config check runs over the
// decoded payload's model config.
func (s *v8Suite) TestPrechecksFanConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()

	payload := s.validPayload()
	payload.ModelConfig = []v4_0_6.ModelConfig{
		{Key: "fan-config", Value: "10.0.0.0/8=252.0.0.0/8"},
	}

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, payload))
	c.Assert(err, tc.ErrorMatches, `.*fan networking not supported.*`)
}

// TestPrechecksModelVersionNewerThanController verifies that the model's
// agent version from the payload must not be ahead of the target controller.
func (s *v8Suite) TestPrechecksModelVersionNewerThanController(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()

	payload := s.validPayload()
	payload.AgentVersion = []v4_0_6.AgentVersion{{TargetVersion: "4.2.0"}}

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, payload))
	c.Assert(err, tc.ErrorMatches,
		`migration target prechecks failed: model has higher version than target controller \(4.2.0 > 4.1.0\)`)
}

// TestPrechecksUpgradeInProgress verifies the target-controller readiness
// check rejects a controller that is mid-upgrade.
func (s *v8Suite) TestPrechecksUpgradeInProgress(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()

	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(true, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorMatches, `migration target prechecks failed: upgrade in progress`)
}

// TestPrechecksCloudNotFound verifies the model's cloud must exist on the
// target controller.
func (s *v8Suite) TestPrechecksCloudNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()

	s.cloudService.EXPECT().Cloud(gomock.Any(), "my-cloud").Return(nil, clouderrors.NotFound)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorMatches,
		`migration target prechecks failed: model's cloud "my-cloud" not found on target controller`)
}

// TestPrechecksCloudRegionNotFound verifies the model's cloud region must be
// valid for the target cloud.
func (s *v8Suite) TestPrechecksCloudRegionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()

	s.cloudService.EXPECT().Cloud(gomock.Any(), "my-cloud").Return(&cloud.Cloud{
		Name:    "my-cloud",
		Regions: []cloud.Region{{Name: "other-region"}},
	}, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorMatches,
		`migration target prechecks failed: model's cloud region "my-region" not valid for cloud "my-cloud" on target controller: .*`)
}

// TestPrechecksUserDisabled verifies that a model user that exists on the
// target but is disabled blocks the migration.
func (s *v8Suite) TestPrechecksUserDisabled(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.Users = []params.ModelUser{{Name: "blocked-user"}}

	s.userService.EXPECT().GetUserByName(gomock.Any(), usertesting.GenNewName(c, "blocked-user")).Return(
		user.User{Name: usertesting.GenNewName(c, "blocked-user"), Disabled: true}, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorMatches,
		`migration target prechecks failed: model user "blocked-user" is disabled on the target controller`)
}

// TestPrechecksCredentialAuthTypeMismatch verifies an existing target
// credential with a different auth type is a conflict.
func (s *v8Suite) TestPrechecksCredentialAuthTypeMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.ModelCredential = &params.ModelCloudCredential{
		Cloud:    "my-cloud",
		Owner:    "someone",
		Name:     "foobar",
		AuthType: "access-key",
	}

	s.credentialService.EXPECT().CloudCredential(gomock.Any(), gomock.Any()).Return(
		cloud.NewCredential("userpass", nil), nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorMatches,
		`.*already exists on the target controller with auth-type "userpass", not "access-key"`)
}

// TestPrechecksCredentialAttributesMismatch verifies an existing target
// credential with different attributes is a conflict.
func (s *v8Suite) TestPrechecksCredentialAttributesMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.ModelCredential = &params.ModelCloudCredential{
		Cloud:      "my-cloud",
		Owner:      "someone",
		Name:       "foobar",
		AuthType:   "access-key",
		Attributes: map[string]string{"foo": "bar"},
	}

	s.credentialService.EXPECT().CloudCredential(gomock.Any(), gomock.Any()).Return(
		cloud.NewCredential("access-key", map[string]string{"foo": "different"}), nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorMatches,
		`.*already exists on the target controller with different attributes`)
}

// TestPrechecksCredentialRevoked verifies an existing revoked target
// credential blocks the migration when the incoming credential is live.
func (s *v8Suite) TestPrechecksCredentialRevoked(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.ModelCredential = &params.ModelCloudCredential{
		Cloud:    "my-cloud",
		Owner:    "someone",
		Name:     "foobar",
		AuthType: "access-key",
	}

	s.credentialService.EXPECT().CloudCredential(gomock.Any(), gomock.Any()).Return(
		cloud.NewNamedCredential("foobar", "access-key", nil, true), nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorMatches, `.*is revoked on the target controller`)
}

// TestPrechecksCredentialMissingIsFine verifies a credential absent from the
// target passes prechecks (it is created on import).
func (s *v8Suite) TestPrechecksCredentialMissingIsFine(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectHappyPath()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.ModelCredential = &params.ModelCloudCredential{
		Cloud:    "my-cloud",
		Owner:    "someone",
		Name:     "foobar",
		AuthType: "access-key",
	}

	s.credentialService.EXPECT().CloudCredential(gomock.Any(), gomock.Any()).Return(
		cloud.Credential{}, credentialerrors.NotFound)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
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

	expectedErr := stderrors.New("mint failed")
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

// TestPrechecksSecretBackendNotFound verifies the model's secret backend
// must exist on the target controller.
func (s *v8Suite) TestPrechecksSecretBackendNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.SecretBackend = &params.ModelSecretBackend{Name: "myvault"}

	s.secretBackendService.EXPECT().GetSecretBackendByName(gomock.Any(), "myvault").Return(
		nil, secretbackenderrors.NotFound)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorMatches,
		`migration target prechecks failed: model's secret backend "myvault" not found on target controller`)
}

// TestPrechecksExternalControllerConflict verifies that an existing external
// controller record with a different CA cert or addresses is a conflict.
func (s *v8Suite) TestPrechecksExternalControllerConflict(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	thirdPartyUUID := uuid.MustNewUUID().String()
	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.ExternalControllers = []params.ExternalControllerRef{
		{UUID: thirdPartyUUID, CACert: "ca-cert", Addresses: []string{"10.0.0.1:17070"}},
	}

	s.externalControllerService.EXPECT().Controller(gomock.Any(), thirdPartyUUID).Return(
		&crossmodel.ControllerInfo{
			ControllerUUID: thirdPartyUUID,
			CACert:         "different-ca-cert",
			Addrs:          []string{"10.0.0.1:17070"},
		}, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, modelmigrationerrors.ErrExternalControllerConflict)
}

// TestPrechecksImportClaimExists verifies the occupied-import-slot rejection
// reports the claim phase.
func (s *v8Suite) TestPrechecksImportClaimExists(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	s.migrationImportService.EXPECT().GetImportClaim(gomock.Any(), model.UUID(s.modelUUID)).Return(
		modelmigration.ImportClaim{
			SourceMigrationUUID: "previous-migration",
			Phase:               modelmigration.ImportPhaseAborting,
		}, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorMatches,
		`.*already has an import claim on this controller \(phase "aborting", source migration "previous-migration", updated .*\)`)
}

// TestPrechecksModelExistsWithoutClaim verifies a live model row without an
// import claim is a hard UUID collision.
func (s *v8Suite) TestPrechecksModelExistsWithoutClaim(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	s.migrationImportService.EXPECT().GetImportClaim(gomock.Any(), model.UUID(s.modelUUID)).Return(
		modelmigration.ImportClaim{}, modelmigrationerrors.ErrImportNotFound)
	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(s.modelUUID)).Return(
		model.Model{UUID: model.UUID(s.modelUUID)}, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorMatches,
		`migration target prechecks failed: model with same UUID already exists \(`+s.modelUUID+`\)`)
}

// TestPrechecksNamespaceExistsWithoutClaim verifies a live model_namespace
// row without an import claim is a hard collision.
func (s *v8Suite) TestPrechecksNamespaceExistsWithoutClaim(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	s.migrationImportService.EXPECT().GetImportClaim(gomock.Any(), model.UUID(s.modelUUID)).Return(
		modelmigration.ImportClaim{}, modelmigrationerrors.ErrImportNotFound)
	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(s.modelUUID)).Return(
		model.Model{}, modelerrors.NotFound)
	s.migrationImportService.EXPECT().ModelNamespaceExists(gomock.Any(), model.UUID(s.modelUUID)).Return(
		true, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorMatches,
		`migration target prechecks failed: model database namespace for .* already exists on target controller`)
}

// TestPrechecksNameConflict verifies a model with the same name and qualifier
// on the target is a conflict.
func (s *v8Suite) TestPrechecksNameConflict(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()
	s.expectControllerReady()
	s.expectCloud()

	s.migrationImportService.EXPECT().GetImportClaim(gomock.Any(), model.UUID(s.modelUUID)).Return(
		modelmigration.ImportClaim{}, modelmigrationerrors.ErrImportNotFound)
	s.modelService.EXPECT().Model(gomock.Any(), model.UUID(s.modelUUID)).Return(
		model.Model{}, modelerrors.NotFound)
	s.migrationImportService.EXPECT().ModelNamespaceExists(gomock.Any(), model.UUID(s.modelUUID)).Return(
		false, nil)
	s.modelService.EXPECT().GetAllModels(gomock.Any()).Return([]model.Model{{
		UUID:      model.UUID(uuid.MustNewUUID().String()),
		Name:      "some-model",
		Qualifier: "prod",
	}}, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorMatches,
		`migration target prechecks failed: model named "some-model" already exists`)
}

// TestImportRunsGuardsThenNoOpSuccess verifies the v8 Import shell runs the
// mandatory pre-write guards and then succeeds as a no-op until the real
// import path lands. Only the schema guard is primed: Import must NOT run the
// static or environmental prechecks, so any environmental lookup would surface
// here as an unexpected gomock call.
func (s *v8Suite) TestImportRunsGuardsThenNoOpSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSchemaReady()

	err := s.mustNewAPIV8(c).Import(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorIsNil)
}

// TestImportGuardFailure verifies a guard failure in the v8 Import shell is
// returned as-is, not masked by the no-op success tail.
func (s *v8Suite) TestImportGuardFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.migrationImportService.EXPECT().CheckTargetImportSchema(gomock.Any()).Return(
		schemaNotReadyError())

	err := s.mustNewAPIV8(c).Import(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
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

// schemaNotReadyError mirrors the coded error returned by the state-layer
// schema guard when the v8 import schema objects are missing.
func schemaNotReadyError() error {
	return errors.Errorf("target schema not ready for migrationtarget v8 %w", coreerrors.NotSupported)
}
