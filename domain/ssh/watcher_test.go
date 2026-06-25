// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainssh "github.com/juju/juju/domain/ssh"
	sshmodelservice "github.com/juju/juju/domain/ssh/service/model"
	sshmodelstate "github.com/juju/juju/domain/ssh/state/model"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) TestWatchSSHConnRequest(c *tc.C) {
	svc := s.setupService(c)
	machineUUID := s.addMachine(c, "1")
	_ = machineUUID

	// Prime the change stream so the watcher sees a clean initial state.
	s.AssertChangeStreamIdle(c, "ssh watcher test")

	w, err := svc.WatchSSHConnRequest(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, w))

	tunnelID := internaluuid.MustNewUUID().String()
	now := clock.WallClock.Now()
	req := domainssh.SSHConnRequest{
		TunnelID:            tunnelID,
		MachineName:         "1",
		Expires:             now.Add(time.Minute),
		SSHUsername:         "juju-reverse-tunnel",
		SSHPassword:         "secret",
		ControllerAddresses: network.NewSpaceAddresses("10.0.0.1"),
		UnitPort:            22,
		EphemeralPublicKey:  []byte("pubkey"),
	}

	// Assert watcher fires on insert.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.InsertSSHConnRequest(c.Context(), req)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Assert watcher fires on remove.
	harness.AddTest(c, func(c *tc.C) {
		err := svc.RemoveSSHConnRequest(c.Context(), tunnelID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Assert no spurious changes.
	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) setupService(c *tc.C) *sshmodelservice.WatchableService {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "ssh_connection_request")

	return sshmodelservice.NewWatchableService(
		sshmodelstate.NewState(modelDB),
		coremodel.UUID(s.ModelUUID()),
		clock.WallClock,
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}

func (s *watcherSuite) addMachine(c *tc.C, name string) string {
	machineUUID := internaluuid.MustNewUUID().String()
	netNodeUUID := internaluuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO machine (uuid, name, net_node_uuid, life_id)
VALUES (?, ?, ?, (SELECT id FROM life WHERE value = 'alive'))
`, machineUUID, name, netNodeUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}
