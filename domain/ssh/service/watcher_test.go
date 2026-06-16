// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/ssh"
	sshstatemodel "github.com/juju/juju/domain/ssh/state/model"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	svc *Service
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "ssh_connection_request")
	s.svc = NewService(
		sshstatemodel.NewState(func(ctx context.Context) (coredatabase.TxnRunner, error) {
			return factory(ctx)
		}),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		clock.WallClock,
	)
}

func (s *watcherSuite) TestWatchSSHConnRequest(c *tc.C) {
	s.addMachine(c, "0")
	req := ssh.SSHConnRequest{
		TunnelID:            "tunnel-0",
		MachineID:           "0",
		Expires:             time.Now().UTC().Add(time.Minute),
		Username:            "juju-reverse-tunnel",
		Password:            "secret",
		ControllerAddresses: network.NewSpaceAddresses("10.0.0.1"),
		EphemeralPublicKey:  []byte("pub"),
	}

	err := s.svc.InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)

	s.AssertChangeStreamIdle(c, "before watcher start")

	watcher, err := s.svc.WatchSSHConnRequest(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	w := watchertest.NewStringsWatcherC(c, watcher)
	w.AssertChange(req.TunnelID)

	err = s.svc.RemoveSSHConnRequest(c.Context(), req.TunnelID)
	c.Assert(err, tc.ErrorIsNil)
	w.AssertChange(req.TunnelID)
}

func (s *watcherSuite) addMachine(c *tc.C, name string) string {
	netNodeUUID := internaluuid.MustNewUUID().String()
	machineUUID := internaluuid.MustNewUUID().String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ?, ?)`, machineUUID, netNodeUUID, name, 0)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}
