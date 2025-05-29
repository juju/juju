// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
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
	changestreamtesting.ControllerModelSuite

	modelID model.UUID
	userID  user.UUID
}

func TestKeyUpdaterSuite(t *stdtesting.T) {
	tc.Run(t, &keyUpdaterSuite{})
}

func (s *keyUpdaterSuite) SetUpTest(c *tc.C) {
	s.ControllerModelSuite.SetUpTest(c)

	s.SeedControllerUUID(c)

	s.userID = usertesting.GenUserUUID(c)

	accessState := accessstate.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := accessState.AddUser(
		c.Context(), s.userID,
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

	err = fn(c.Context(), s.ControllerTxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	credentialName := "test"
	fn = credentialbootstrap.InsertCredential(credential.Key{
		Cloud: cloudName,
		Name:  credentialName,
		Owner: user.AdminUserName,
	},
		cloud.NewCredential(cloud.EmptyAuthType, nil),
	)

	err = fn(c.Context(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
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
		Name:       "test",
		Qualifier:  "prod",
		AdminUsers: []user.UUID{s.userID},
	})
	s.modelID = modelUUID

	err = modelFn(c.Context(), s.ControllerTxnRunner(), s.ControllerSuite.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	err = modelbootstrap.CreateLocalModelRecord(modelUUID, uuid.MustNewUUID(), jujuversion.Current)(
		c.Context(), s.ControllerTxnRunner(), s.ModelTxnRunner(c, string(s.modelID)))
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

	modelTxnRunnerFactory := func() (database.TxnRunner, error) {
		return s.ModelTxnRunner(c, string(s.modelID)), nil
	}

	controllerSt := state.NewControllerState(s.ControllerSuite.TxnRunnerFactory())
	st := state.NewState(modelTxnRunnerFactory)
	_, idler := s.InitWatchableDB(c, database.ControllerNS)
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, database.ControllerNS),
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

	keyManagerSt := keymanagerstate.NewState(s.TxnRunnerFactory())
	keyManagerSvc := keymanagerservice.NewService(s.modelID, keyManagerSt)

	harness := watchertest.NewHarness(idler, watchertest.NewWatcherC(c, watcher))

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

	err = s.ModelTxnRunner(c, string(s.modelID)).Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
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
