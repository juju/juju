// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	accessservice "github.com/juju/juju/domain/access/service"
	accessstate "github.com/juju/juju/domain/access/state"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	credentialbootstrap "github.com/juju/juju/domain/credential/bootstrap"
	keymanagerservice "github.com/juju/juju/domain/keymanager/service"
	keymanagerstate "github.com/juju/juju/domain/keymanager/state"
	"github.com/juju/juju/domain/keyupdater/service"
	"github.com/juju/juju/domain/keyupdater/state"
	machinebootstrap "github.com/juju/juju/domain/machine/bootstrap"
	domainmodel "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	"github.com/juju/juju/domain/model/state/testing"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type keyUpdaterSuite struct {
	changestreamtesting.ControllerSuite
	changestreamtesting.ModelSuite

	modelID model.UUID
	userID  user.UUID
}

var _ = gc.Suite(&keyUpdaterSuite{})

func (s *keyUpdaterSuite) SetUpTest(c *gc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.ModelSuite.SetUpTest(c)

	s.SeedControllerUUID(c)

	s.userID = usertesting.GenUserUUID(c)

	accessState := accessstate.NewState(s.ControllerSuite.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := accessState.AddUser(
		context.Background(), s.userID,
		user.AdminUserName,
		user.AdminUserName.Name(),
		false,
		s.userID,
	)
	c.Assert(err, jc.ErrorIsNil)

	cloudName := "test"
	fn := cloudbootstrap.InsertCloud(user.AdminUserName, cloud.Cloud{
		Name:      cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	})

	err = fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	credentialName := "test"
	fn = credentialbootstrap.InsertCredential(credential.Key{
		Cloud: cloudName,
		Name:  credentialName,
		Owner: user.AdminUserName,
	},
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	err = fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	testing.CreateInternalSecretBackend(c, s.ControllerTxnRunner())

	modelUUID := modeltesting.GenModelUUID(c)
	modelFn := modelbootstrap.CreateGlobalModelRecord(modelUUID, domainmodel.GlobalModelCreationArgs{
		Cloud: cloudName,
		Credential: credential.Key{
			Cloud: cloudName,
			Name:  credentialName,
			Owner: user.AdminUserName,
		},
		Name:  "test",
		Owner: s.userID,
	})
	s.modelID = modelUUID

	err = modelFn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	err = modelbootstrap.CreateLocalModelRecord(modelUUID, uuid.MustNewUUID(), jujuversion.Current)(
		context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	err = machinebootstrap.InsertMachine("0")(
		context.Background(),
		s.ControllerTxnRunner(),
		s.ModelTxnRunner(),
	)
	c.Assert(err, jc.ErrorIsNil)
}

// TestWatchAuthorizedKeysForMachine is here to assert an integration test
// between all the components of key updating. Specifically we want to see that
// as users come and go from the system and also their private keys we get
// watcher events and the authorized keys reported for the machine in question
// is correct.
func (s *keyUpdaterSuite) TestWatchAuthorizedKeysForMachine(c *gc.C) {
	c.Skip("TODO: Simon fixing multi watchers")
	ctx, cancel := jujutesting.LongWaitContext()
	defer cancel()

	controllerSt := state.NewControllerState(s.ControllerSuite.TxnRunnerFactory())
	st := state.NewState(s.ModelSuite.TxnRunnerFactory())
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.ControllerSuite.GetWatchableDB, "model_authorized_keys"),
		loggertesting.WrapCheckLog(c),
	)

	svc := service.NewWatchableService(
		service.NewControllerKeyService(controllerSt),
		controllerSt,
		st,
		factory,
	)

	watcher, err := svc.WatchAuthorisedKeysForMachine(ctx, machine.Name("0"))
	c.Assert(err, jc.ErrorIsNil)

	keyManagerSt := keymanagerstate.NewState(s.ControllerSuite.TxnRunnerFactory())
	keyManagerSvc := keymanagerservice.NewService(s.modelID, keyManagerSt)

	harness := watchertest.NewHarness(&s.ControllerSuite, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *gc.C) {
		err = keyManagerSvc.AddPublicKeysForUser(
			ctx,
			s.userID,
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC one@juju.is",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe two@juju.is",
		)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *gc.C) {
		err = keyManagerSvc.DeleteKeysForUser(
			ctx,
			s.userID,
			"one@juju.is",
		)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	userSvc := accessservice.NewUserService(
		accessstate.NewUserState(s.ControllerSuite.TxnRunnerFactory()),
	)

	harness.AddTest(func(c *gc.C) {
		err = userSvc.DisableUserAuthentication(ctx, user.AdminUserName)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *gc.C) {
		keys, err := svc.GetAuthorisedKeysForMachine(ctx, machine.Name("0"))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(len(keys), gc.Equals, 0)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.AddTest(func(c *gc.C) {
		err = userSvc.EnableUserAuthentication(ctx, user.AdminUserName)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *gc.C) {
		keys, err := svc.GetAuthorisedKeysForMachine(ctx, machine.Name("0"))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(keys, jc.DeepEquals, []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe two@juju.is",
		})
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.AddTest(func(c *gc.C) {
		err = userSvc.RemoveUser(ctx, user.AdminUserName)
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *gc.C) {
		keys, err := svc.GetAuthorisedKeysForMachine(ctx, machine.Name("0"))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(len(keys), gc.Equals, 0)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}
