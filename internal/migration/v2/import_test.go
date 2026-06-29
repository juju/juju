// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coredatabase "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	corelease "github.com/juju/juju/core/lease"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	corepermission "github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	accesserrors "github.com/juju/juju/domain/access/errors"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	cloudimagemetadataservice "github.com/juju/juju/domain/cloudimagemetadata/service"
	cloudimagemetadatastate "github.com/juju/juju/domain/cloudimagemetadata/state"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	"github.com/juju/juju/domain/export"
	keymanagerservice "github.com/juju/juju/domain/keymanager/service"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	leaseservice "github.com/juju/juju/domain/lease/service"
	leasestate "github.com/juju/juju/domain/lease/state"
	modelstatecontroller "github.com/juju/juju/domain/model/state/controller"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	migrationdomain "github.com/juju/juju/domain/modelmigration"
	migrationclaimstate "github.com/juju/juju/domain/modelmigration/state/controller"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	migrationv2 "github.com/juju/juju/internal/migration/v2"
	"github.com/juju/juju/internal/uuid"
)

// importV2Suite exercises [migrationv2.ImportControllerModelInfo] end-to-end
// against real controller and model databases: the decode, the claim, the
// target-local bootstrap, and the controller-data import steps. It does not
// exercise model-DB content import (Tasks 7-9) or activation (Task 10).
type importV2Suite struct {
	schematesting.ControllerModelSuite

	adminUserUUID  coreuser.UUID
	cloudName      string
	credentialName string
}

func TestImportV2Suite(t *testing.T) {
	tc.Run(t, &importV2Suite{})
}

func (s *importV2Suite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	// ImportControllerModelInfo refuses to create a model unless the
	// controller's own model exists and is alive, so a real (activated) model
	// is required here, not just a bare controller row.
	controllerModelUUID := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "controller")
	controllerUUID := s.SeedControllerTable(c, controllerModelUUID)

	var err error
	s.adminUserUUID, err = coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	accessState := accessstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	err = accessState.AddUserWithPermission(
		c.Context(), s.adminUserUUID,
		coreuser.AdminUserName,
		coreuser.AdminUserName.Name(),
		false,
		s.adminUserUUID,
		corepermission.AccessSpec{
			Access: corepermission.SuperuserAccess,
			Target: corepermission.ID{ObjectType: corepermission.Controller, Key: controllerUUID},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	// everyone@external is normally created during controller bootstrap and
	// is required as the creator when ImportExternalUsers creates users.
	everyoneName := tc.Must1(c, coreuser.NewName, "everyone@external")
	everyoneUUID, err := coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	err = accessState.AddUser(c.Context(), everyoneUUID, everyoneName, "everyone@external", true, s.adminUserUUID)
	c.Assert(err, tc.ErrorIsNil)

	s.cloudName = "test-cloud"
	fn := cloudbootstrap.InsertCloud(coreuser.AdminUserName, cloud.Cloud{
		Name:      s.cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType},
	})
	err = fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	s.credentialName = "test-cred"
	fn = credentialbootstrap.InsertCredential(
		corecredential.Key{Cloud: s.cloudName, Name: s.credentialName, Owner: coreuser.AdminUserName},
		cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"access-key": "val"}),
	)
	err = fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	modeltesting.CreateInternalSecretBackend(c, s.ControllerTxnRunner())
}

// deps returns the [migrationv2.Deps] together with the controller/model
// txn-runner factories backing it, so tests can build companion services
// against the same databases.
func (s *importV2Suite) deps(c *tc.C, modelUUID coremodel.UUID) (migrationv2.Deps, coredatabase.TxnRunnerFactory, coredatabase.TxnRunnerFactory) {
	controllerFactory := s.TxnRunnerFactory()
	modelRunner := s.ModelTxnRunner(c, modelUUID.String())
	modelFactory := func(context.Context) (coredatabase.TxnRunner, error) {
		return modelRunner, nil
	}

	return migrationv2.Deps{
		ControllerDB: controllerFactory,
		ModelDB:      modelFactory,
		Clock:        clock.WallClock,
		Logger:       loggertesting.WrapCheckLog(c),
	}, controllerFactory, modelFactory
}

func (s *importV2Suite) baseControllerModelInfo(modelUUID coremodel.UUID) coremodelmigration.ControllerModelInfo {
	return coremodelmigration.ControllerModelInfo{
		ModelInfo: coremodelmigration.ModelIdentityInfo{
			UUID:      modelUUID.String(),
			Name:      "imported-model",
			Qualifier: "prod",
			Type:      "iaas",
			Cloud:     s.cloudName,
			Life:      "alive",
		},
	}
}

func (s *importV2Suite) TestImportModelHappyPath(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	deps, controllerFactory, _ := s.deps(c, modelUUID)

	bobLastLogin := time.Now().UTC().Truncate(time.Second)
	offerUUID := uuid.MustNewUUID().String()

	sourceMigrationUUID := uuid.MustNewUUID().String()
	info := s.baseControllerModelInfo(modelUUID)
	info.ModelCredential = &coremodelmigration.ModelCloudCredential{
		Cloud:      s.cloudName,
		Owner:      coreuser.AdminUserName.Name(),
		Name:       s.credentialName,
		AuthType:   string(cloud.AccessKeyAuthType),
		Attributes: map[string]string{"access-key": "val"},
	}
	info.Users = []coremodelmigration.ModelUser{
		{Name: coreuser.AdminUserName.Name()},
		{Name: "bob@external", DisplayName: "Bob", External: true, CreatedAt: time.Now().UTC(), LastLogin: &bobLastLogin},
		{Name: "alice@external", DisplayName: "Alice", External: true, Removed: true, CreatedAt: time.Now().UTC()},
		{Name: "carol", DisplayName: "Carol"},
	}
	info.Permissions = []coremodelmigration.ModelPermission{
		{ObjectType: "model", GrantOn: modelUUID.String(), SubjectName: "bob@external", Access: "read"},
		{ObjectType: "model", GrantOn: modelUUID.String(), SubjectName: "carol", Access: "read"},
		{ObjectType: "offer", GrantOn: offerUUID, SubjectName: "bob@external", Access: "consume"},
	}
	info.AuthorizedKeys = []coremodelmigration.ModelAuthorizedKey{
		{Username: "bob@external", PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC bob@host"},
		{Username: "carol", PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe carol@host"},
	}
	info.Leaders = []coremodelmigration.ApplicationLeadership{
		{Application: "myapp", Leader: "myapp/0"},
	}
	info.CloudImageMetadata = []coremodelmigration.CloudImageMetadata{
		{Stream: "released", Region: s.cloudName, Version: "22.04", Arch: "amd64", Source: "custom", Priority: 10, ImageID: "ami-1234"},
	}

	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}

	err := migrationv2.ImportControllerModelInfo(c.Context(), deps, sourceMigrationUUID, info, view)
	c.Assert(err, tc.ErrorIsNil)

	// The claim must still be in the "importing" phase: activation is a
	// later task and must not be triggered by this method.
	claimSt := migrationclaimstate.New(controllerFactory, clock.WallClock)
	claim, err := claimSt.GetImportClaim(c.Context(), modelUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(claim.Phase, tc.Equals, migrationdomain.ImportPhaseImporting)
	c.Check(claim.SourceMigrationUUID, tc.Equals, sourceMigrationUUID)

	// The controller-DB model row exists with the bootstrap identity.
	modelSt := modelstatecontroller.NewState(controllerFactory)
	seed, err := modelSt.GetModelSeedInformation(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seed.Name, tc.Equals, info.ModelInfo.Name)
	c.Check(seed.Cloud, tc.Equals, s.cloudName)

	accessSvc := accessservice.NewService(accessstate.NewState(controllerFactory, clock.WallClock, loggertesting.WrapCheckLog(c)), clock.WallClock)

	bobName := tc.Must1(c, coreuser.NewName, "bob@external")
	aliceName := tc.Must1(c, coreuser.NewName, "alice@external")
	carolName := tc.Must1(c, coreuser.NewName, "carol")

	// bob@external is missing on the target, so it is created.
	bobUser, err := accessSvc.GetUserByName(c.Context(), bobName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(bobUser.DisplayName, tc.Equals, "Bob")

	// alice@external is created then immediately disabled (removed on the
	// source): GetUserByName must report her as not found, like any other
	// removed user.
	_, err = accessSvc.GetUserByName(c.Context(), aliceName)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)

	// carol was never created (local users are never auto-created), so her
	// permission and authorized-key entries above must have been silently
	// skipped without erroring the whole import.
	_, err = accessSvc.GetUserByName(c.Context(), carolName)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)

	// bob's model permission landed.
	access, err := accessSvc.ReadUserAccessForTarget(c.Context(), bobName,
		corepermission.ID{ObjectType: corepermission.Model, Key: modelUUID.String()})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(access.Access, tc.Equals, corepermission.ReadAccess)

	// bob's offer permission landed via the batched ImportOfferAccess call.
	offerAccess, err := accessSvc.ReadUserAccessForTarget(c.Context(), bobName,
		corepermission.ID{ObjectType: corepermission.Offer, Key: offerUUID})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(offerAccess.Access, tc.Equals, corepermission.ConsumeAccess)

	// bob's authorized key landed; carol's was skipped.
	bobUUID, err := accessSvc.GetUserUUIDByName(c.Context(), bobName)
	c.Assert(err, tc.ErrorIsNil)
	keyManagerSvc := keymanagerservice.NewService(modelUUID, keymanagerstate.NewState(controllerFactory))
	keys, err := keyManagerSvc.ListPublicKeysForUser(c.Context(), bobUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(keys, tc.HasLen, 1)

	// bob's last login landed.
	lastLogin, err := accessSvc.LastModelLogin(c.Context(), bobName, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lastLogin.Equal(bobLastLogin), tc.IsTrue, tc.Commentf("got %s, want %s", lastLogin, bobLastLogin))

	// The leadership lease was claimed fresh.
	leaseSvc := leaseservice.NewService(leasestate.NewState(controllerFactory, loggertesting.WrapCheckLog(c)))
	leaseKey := corelease.Key{ModelUUID: modelUUID.String(), Namespace: corelease.ApplicationLeadershipNamespace, Lease: "myapp"}
	leases, err := leaseSvc.Leases(c.Context(), leaseKey)
	c.Assert(err, tc.ErrorIsNil)
	leaseInfo, ok := leases[leaseKey]
	c.Assert(ok, tc.IsTrue)
	c.Check(leaseInfo.Holder, tc.Equals, "myapp/0")

	// The custom cloud image metadata row was recreated.
	imageMetadataSvc := cloudimagemetadataservice.NewService(
		cloudimagemetadatastate.NewState(controllerFactory, clock.WallClock, loggertesting.WrapCheckLog(c)),
	)
	allMetadata, err := imageMetadataSvc.AllCloudImageMetadata(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allMetadata, tc.HasLen, 1)
}

// TestImportModelDuplicateClaim verifies a second ImportControllerModelInfo call
// for the same model UUID fails with a coded AlreadyExists error rather than
// silently re-running (or corrupting) the first import's writes.
func (s *importV2Suite) TestImportModelDuplicateClaim(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	deps, _, _ := s.deps(c, modelUUID)

	sourceMigrationUUID := uuid.MustNewUUID().String()
	info := s.baseControllerModelInfo(modelUUID)
	view := export.ProjectionView{AgentTargetVersion: jujuversion.Current}

	err := migrationv2.ImportControllerModelInfo(c.Context(), deps, sourceMigrationUUID, info, view)
	c.Assert(err, tc.ErrorIsNil)

	err = migrationv2.ImportControllerModelInfo(c.Context(), deps, sourceMigrationUUID, info, view)
	c.Check(err, tc.ErrorIs, coreerrors.AlreadyExists)
}
