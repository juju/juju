// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

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
	"github.com/juju/juju/domain/life"
	domainmodel "github.com/juju/juju/domain/model"
	modelbootstrap "github.com/juju/juju/domain/model/bootstrap"
	"github.com/juju/juju/domain/model/state/testing"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
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

var _ = tc.Suite(&keyUpdaterSuite{})

func (s *keyUpdaterSuite) SetUpTest(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	cloudName := "test"
	fn := cloudbootstrap.InsertCloud(user.AdminUserName, cloud.Cloud{
		Name:      cloudName,
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.EmptyAuthType},
	})

	err = fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	credentialName := "test"
	fn = credentialbootstrap.InsertCredential(credential.Key{
		Cloud: cloudName,
		Name:  credentialName,
		Owner: user.AdminUserName,
	},
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	err = fn(context.Background(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

	err = modelbootstrap.CreateLocalModelRecord(modelUUID, uuid.MustNewUUID(), jujuversion.Current)(
		context.Background(), s.ControllerTxnRunner(), s.ModelTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	s.createMachine(c, "0")
}

// TestWatchAuthorizedKeysForMachine is here to assert an integration test
// between all the components of key updating. Specifically we want to see that
// as users come and go from the system and also their private keys we get
// watcher events and the authorized keys reported for the machine in question
// is correct.
func (s *keyUpdaterSuite) TestWatchAuthorizedKeysForMachine(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)

	keyManagerSt := keymanagerstate.NewState(s.ControllerSuite.TxnRunnerFactory())
	keyManagerSvc := keymanagerservice.NewService(s.modelID, keyManagerSt)

	harness := watchertest.NewHarness(&s.ControllerSuite, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *tc.C) {
		err = keyManagerSvc.AddPublicKeysForUser(
			ctx,
			s.userID,
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC one@juju.is",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe two@juju.is",
		)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *tc.C) {
		err = keyManagerSvc.DeleteKeysForUser(
			ctx,
			s.userID,
			"one@juju.is",
		)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	userSvc := accessservice.NewUserService(
		accessstate.NewUserState(s.ControllerSuite.TxnRunnerFactory()),
	)

	harness.AddTest(func(c *tc.C) {
		err = userSvc.DisableUserAuthentication(ctx, user.AdminUserName)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *tc.C) {
		keys, err := svc.GetAuthorisedKeysForMachine(ctx, machine.Name("0"))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(len(keys), tc.Equals, 0)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.AddTest(func(c *tc.C) {
		err = userSvc.EnableUserAuthentication(ctx, user.AdminUserName)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *tc.C) {
		keys, err := svc.GetAuthorisedKeysForMachine(ctx, machine.Name("0"))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(keys, tc.DeepEquals, []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe two@juju.is",
		})
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.AddTest(func(c *tc.C) {
		err = userSvc.RemoveUser(ctx, user.AdminUserName)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *tc.C) {
		keys, err := svc.GetAuthorisedKeysForMachine(ctx, machine.Name("0"))
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(len(keys), tc.Equals, 0)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *keyUpdaterSuite) createMachine(c *tc.C, machineId machine.Name) {
	nodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	query := `
INSERT INTO machine (*)
VALUES ($createMachine.*)
`
	machine := createMachine{
		MachineUUID: machine.UUID(machineUUID.String()),
		NetNodeUUID: nodeUUID.String(),
		Name:        machineId,
		LifeID:      life.Alive,
	}

	createMachineStmt, err := sqlair.Prepare(query, machine)
	c.Assert(err, tc.ErrorIsNil)

	createNode := `INSERT INTO net_node (uuid) VALUES ($createMachine.net_node_uuid)`
	createNodeStmt, err := sqlair.Prepare(createNode, machine)
	c.Assert(err, tc.ErrorIsNil)

	err = s.ModelTxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, createNodeStmt, machine).Run(); err != nil {
			return errors.Errorf("creating net node row for bootstrap machine %q: %w", machineId, err)
		}
		if err := tx.Query(ctx, createMachineStmt, machine).Run(); err != nil {
			return errors.Errorf("creating machine row for bootstrap machine %q: %w", machineId, err)
		}
		return nil
	})

	c.Assert(err, tc.ErrorIsNil)
}

type createMachine struct {
	MachineUUID machine.UUID `db:"uuid"`
	NetNodeUUID string       `db:"net_node_uuid"`
	Name        machine.Name `db:"name"`
	LifeID      life.Life    `db:"life_id"`
}
