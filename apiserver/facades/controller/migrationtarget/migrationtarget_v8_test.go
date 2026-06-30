// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"
	"time"

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
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/facades"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/export"
	v4_0_11 "github.com/juju/juju/domain/export/types/v4_0_11"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/migration"
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
	migrationImportService    *MockMigrationImportService

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
	s.migrationImportService = NewMockMigrationImportService(ctrl)

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
		s.migrationImportService,
	)
	c.Assert(err, tc.ErrorIsNil)
	return apiV8
}

// validPayload returns a v4_0_11 payload with an agent version below the target
// controller version used in tests.
func (s *v8Suite) validPayload() v4_0_11.ModelExport {
	return v4_0_11.ModelExport{
		AgentVersion: []v4_0_11.AgentVersion{{
			TargetVersion: "4.0.11",
		}},
		Application: []v4_0_11.Application{{
			UUID:      "app-uuid",
			Name:      "ubuntu",
			CharmUUID: "charm-uuid",
		}},
		CharmManifestBase: []v4_0_11.CharmManifestBase{{
			CharmUUID: "charm-uuid",
			Risk:      "stable",
		}},
	}
}

func (s *v8Suite) makeEnvelope(c *tc.C, payload v4_0_11.ModelExport) params.SerializedModelV2 {
	data, err := yaml.Marshal(payload)
	c.Assert(err, tc.ErrorIsNil)

	return params.SerializedModelV2{
		PayloadVersion: semversion.MustParse("4.0.11"),
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

// TestPrechecksSuccess runs the full precheck routine over a well-formed
// envelope: the controller-readiness checks pass and the environmental checks
// delegated to the modelmigration domain return no error. It also asserts the
// envelope's semantic fields are mapped onto the precheck arguments handed to
// the domain.
func (s *v8Suite) TestPrechecksSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectControllerReady()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.Users = []params.ModelUser{{Name: "alice"}, {Name: "bob"}}
	envelope.ModelCredential = &params.ModelCloudCredential{
		Cloud:   "my-cloud",
		Owner:   "carol",
		Name:    "foobar",
		Revoked: true,
	}
	envelope.SecretBackend = &params.ModelSecretBackend{Name: "myvault"}

	var captured modelmigration.ImportPrecheckArgs
	s.migrationImportService.EXPECT().PrecheckImport(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args modelmigration.ImportPrecheckArgs) error {
			captured = args
			return nil
		})

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(captured.ModelUUID, tc.Equals, s.modelUUID)
	c.Check(captured.ModelName, tc.Equals, "some-model")
	c.Check(captured.ModelQualifier, tc.Equals, "prod")
	c.Check(captured.Cloud, tc.Equals, "my-cloud")
	c.Check(captured.CloudRegion, tc.Equals, "my-region")
	c.Check(captured.Users, tc.DeepEquals, []string{"alice", "bob"})
	c.Assert(captured.Credential, tc.NotNil)
	c.Check(captured.Credential.Cloud, tc.Equals, "my-cloud")
	c.Check(captured.Credential.Owner, tc.Equals, "carol")
	c.Check(captured.Credential.Name, tc.Equals, "foobar")
	c.Check(captured.Credential.Revoked, tc.IsTrue)
	c.Check(captured.SecretBackend, tc.Equals, "myvault")
}

// TestPrechecksDomainErrorPropagates verifies that an environmental precheck
// error from the modelmigration domain is surfaced (wrapped) to the caller.
func (s *v8Suite) TestPrechecksDomainErrorPropagates(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectControllerReady()

	s.migrationImportService.EXPECT().PrecheckImport(gomock.Any(), gomock.Any()).Return(
		errors.Errorf("model's cloud %q not found on target controller", "my-cloud"))

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Check(err, tc.ErrorMatches,
		`migration target prechecks failed: model's cloud "my-cloud" not found on target controller`)
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
	c.Check(err, tc.ErrorMatches, `model UUID "not-a-uuid" not valid`)

	envelope = valid
	envelope.ModelInfo.Name = ""
	err = api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `empty model name not valid`)

	envelope = valid
	envelope.ModelInfo.Qualifier = ""
	err = api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `empty model qualifier not valid`)

	envelope = valid
	envelope.ModelInfo.SourceMigrationUUID = ""
	err = api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `empty source migration UUID not valid`)
}

// TestPrechecksPayloadDecodeError verifies that a malformed payload is
// rejected by the decoder with a coded validation error.
func (s *v8Suite) TestPrechecksPayloadDecodeError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.mustNewAPIV8(c)

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.Payload = []byte("\t: garbage")
	err := api.Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(err, tc.ErrorMatches, `decoding model export payload at version "4.0.11".*`)
}

// TestPrechecksPayloadVersionNewerThanTarget verifies the
// payload-version-ahead-of-target rejection with the actionable upgrade
// message, even for versions unknown to the decoder registry.
func (s *v8Suite) TestPrechecksPayloadVersionNewerThanTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.PayloadVersion = semversion.MustParse("9.9.9")

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Check(err, tc.ErrorMatches, fmt.Sprintf(
		`source payload version "9.9.9" is newer than target %q; upgrade the target controller first.*`,
		export.LatestSupportedPayloadVersion()))
}

// TestPrechecksPayloadVersionUnknown verifies that a version at or below the
// target but unknown to the decoder registry yields a clean error.
func (s *v8Suite) TestPrechecksPayloadVersionUnknown(c *tc.C) {
	defer s.setupMocks(c).Finish()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.PayloadVersion = semversion.MustParse("4.0.5")

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
	c.Check(err, tc.ErrorMatches, `model export payload version "4.0.5": not supported`)
}

// TestPrechecksModelVersionNewerThanController verifies that the model's
// agent version from the payload must not be ahead of the target controller.
func (s *v8Suite) TestPrechecksModelVersionNewerThanController(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectControllerReady()

	payload := s.validPayload()
	payload.AgentVersion = []v4_0_11.AgentVersion{{TargetVersion: "4.2.0"}}

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, payload))
	c.Check(err, tc.ErrorMatches,
		`migration target prechecks failed: model has higher version than target controller \(4.2.0 > 4.1.0\)`)
}

// TestPrechecksUpgradeInProgress verifies the target-controller readiness
// check rejects a controller that is mid-upgrade.
func (s *v8Suite) TestPrechecksUpgradeInProgress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.upgradeService.EXPECT().IsUpgrading(gomock.Any()).Return(true, nil)

	err := s.mustNewAPIV8(c).Prechecks(c.Context(), s.makeEnvelope(c, s.validPayload()))
	c.Check(err, tc.ErrorMatches, `migration target prechecks failed: upgrade in progress`)
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

// TestImportRunsGuardsThenDelegatesToImportModelV2 verifies the v8 Import
// runs the mandatory pre-write guards (envelope validation and payload
// version/decode) and then delegates to ModelImporter.ImportModelV2 with the
// decoded projection view. Import must NOT run the environmental prechecks,
// so the precheck service is never primed: any environmental call would
// surface as an unexpected gomock call.
func (s *v8Suite) TestImportRunsGuardsThenDelegatesToImportModelV2(c *tc.C) {
	defer s.setupMocks(c).Finish()

	envelope := s.makeEnvelope(c, s.validPayload())
	createdAt := time.Now().UTC().Truncate(time.Second)
	lastLogin := createdAt.Add(time.Minute)
	rootStorageSize := uint64(42)
	envelope.ModelInfo.CredentialName = "cred"
	envelope.ModelInfo.CredentialOwner = "admin"
	envelope.Users = []params.ModelUser{{
		Name:        "bob@external",
		DisplayName: "Bob",
		CreatedBy:   "admin",
		CreatedAt:   createdAt,
		External:    true,
		LastLogin:   &lastLogin,
	}}
	envelope.ModelCredential = &params.ModelCloudCredential{
		Cloud:      "my-cloud",
		Owner:      "admin",
		Name:       "cred",
		AuthType:   "access-key",
		Attributes: map[string]string{"access-key": "value"},
	}
	envelope.Permissions = []params.ModelPermission{{
		ObjectType: "model", GrantOn: s.modelUUID, SubjectName: "bob@external", Access: "read",
	}}
	envelope.AuthorizedKeys = []params.ModelAuthorizedKey{{
		Username: "bob@external", PublicKey: "ssh-ed25519 AAAA bob@host",
	}}
	envelope.SecretBackend = &params.ModelSecretBackend{Name: "vault", BackendType: "vault"}
	envelope.SecretBackendRefs = []params.SecretBackendReference{{
		BackendName: "vault", SecretRevisionUUID: "secret-rev-uuid", SecretID: "secret:abc",
	}}
	envelope.Leases = []params.Lease{
		{Type: corelease.ApplicationLeadershipNamespace, Name: "ubuntu", Holder: "ubuntu/0"},
		{Type: "singular", Name: "ignored", Holder: "ignored"},
	}
	envelope.CloudImageMetadata = []params.ModelCloudImageMetadata{{
		Stream:          "released",
		Region:          "my-region",
		Version:         "22.04",
		Arch:            "amd64",
		VirtType:        "kvm",
		RootStorageType: "ssd",
		RootStorageSize: &rootStorageSize,
		Source:          "custom",
		Priority:        10,
		ImageId:         "ami-1234",
		CreatedAt:       createdAt,
	}}
	envelope.ExternalControllers = []params.ExternalControllerRef{{
		UUID: "controller-uuid", Alias: "prod", CACert: "cert",
		Addresses: []string{"10.0.0.1:17070"}, ConsumedModels: []string{"remote-model-uuid"},
	}}

	expected := migration.ImportModelArgs{
		SourceMigrationUUID: envelope.ModelInfo.SourceMigrationUUID,
		ControllerModelInfo: coremodelmigration.ControllerModelInfo{
			ModelInfo: coremodelmigration.ModelIdentityInfo{
				UUID:            s.modelUUID,
				Name:            "some-model",
				Qualifier:       "prod",
				Type:            "iaas",
				Cloud:           "my-cloud",
				CloudRegion:     "my-region",
				CredentialName:  "cred",
				CredentialOwner: "admin",
				Life:            "alive",
			},
			Users: []coremodelmigration.ModelUser{{
				Name:        "bob@external",
				DisplayName: "Bob",
				CreatedBy:   "admin",
				CreatedAt:   createdAt,
				External:    true,
				LastLogin:   &lastLogin,
			}},
			ModelCredential: &coremodelmigration.ModelCloudCredential{
				Cloud:      "my-cloud",
				Owner:      "admin",
				Name:       "cred",
				AuthType:   "access-key",
				Attributes: map[string]string{"access-key": "value"},
			},
			Permissions: []coremodelmigration.ModelPermission{{
				ObjectType: "model", GrantOn: s.modelUUID, SubjectName: "bob@external", Access: "read",
			}},
			AuthorizedKeys: []coremodelmigration.ModelAuthorizedKey{{
				Username: "bob@external", PublicKey: "ssh-ed25519 AAAA bob@host",
			}},
			SecretBackend: &coremodelmigration.ModelSecretBackend{
				Name: "vault", BackendType: "vault",
			},
			SecretBackendRefs: []coremodelmigration.SecretBackendReference{{
				BackendName: "vault", SecretRevisionUUID: "secret-rev-uuid", SecretID: "secret:abc",
			}},
			Leaders: []coremodelmigration.ApplicationLeadership{{
				Application: "ubuntu", Leader: "ubuntu/0",
			}},
			CloudImageMetadata: []coremodelmigration.CloudImageMetadata{{
				Stream:          "released",
				Region:          "my-region",
				Version:         "22.04",
				Arch:            "amd64",
				VirtType:        "kvm",
				RootStorageType: "ssd",
				RootStorageSize: &rootStorageSize,
				Source:          "custom",
				Priority:        10,
				ImageID:         "ami-1234",
				CreatedAt:       createdAt,
			}},
			ExternalControllers: []coremodelmigration.ExternalController{{
				UUID: "controller-uuid", Alias: "prod", CACert: "cert",
				Addresses: []string{"10.0.0.1:17070"}, ConsumedModels: []string{"remote-model-uuid"},
			}},
		},
	}
	var got migration.ImportModelArgs
	s.modelImporter.EXPECT().ImportModelV2(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, args migration.ImportModelArgs, _ export.ProjectionView) error {
			got = args
			return nil
		})

	err := s.mustNewAPIV8(c).Import(c.Context(), envelope)
	c.Assert(err, tc.ErrorIsNil)

	// The facade decodes and transforms the model-DB payload to the target
	// version and threads it through; its exact contents are covered by the
	// transformer tests, so here we only assert it is populated and then
	// compare the controller-scoped args verbatim.
	c.Check(got.ModelDBPayload, tc.NotNil)
	got.ModelDBPayload = nil
	c.Check(got, tc.DeepEquals, expected)
}

// TestImportGuardFailure verifies a guard failure in the v8 Import is
// returned as-is, without ever calling ImportModelV2.
func (s *v8Suite) TestImportGuardFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	envelope := s.makeEnvelope(c, s.validPayload())
	envelope.PayloadVersion = semversion.MustParse("9.9.9")

	err := s.mustNewAPIV8(c).Import(c.Context(), envelope)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

// TestImportPropagatesImportModelV2Error verifies an error from
// ImportModelV2 is returned as-is by Import.
func (s *v8Suite) TestImportPropagatesImportModelV2Error(c *tc.C) {
	defer s.setupMocks(c).Finish()

	envelope := s.makeEnvelope(c, s.validPayload())
	s.modelImporter.EXPECT().ImportModelV2(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.Errorf("boom"))

	err := s.mustNewAPIV8(c).Import(c.Context(), envelope)
	c.Assert(err, tc.ErrorMatches, "boom")
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
