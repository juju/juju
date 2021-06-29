// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttest

import (
	"log"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft/raftutil"
)

// RaftFixture is a fixture to embed into test suites, providing
// a raft.Raft and in-memory transport.
type RaftFixture struct {
	testing.IsolationSuite

	FSM           raft.FSM
	Config        *raft.Config
	Transport     *raft.InmemTransport
	Store         *raft.InmemStore
	SnapshotStore *raft.InmemSnapshotStore
	Raft          *raft.Raft
}

func (s *RaftFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	c.Assert(s.FSM, gc.NotNil, gc.Commentf("FSM must be set by embedding test suite"))

	s.Raft, s.Config, s.Transport, s.Store, s.SnapshotStore = s.NewRaft(c, "0", s.FSM)
	c.Assert(s.Raft.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{{
			ID:      s.Config.LocalID,
			Address: s.Transport.LocalAddr(),
		}},
	}).Error(), jc.ErrorIsNil)
	select {
	case leader := <-s.Raft.LeaderCh():
		c.Assert(leader, jc.IsTrue)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for raft leadership")
	}
}

func (s *RaftFixture) NewRaft(c *gc.C, id raft.ServerID, fsm raft.FSM) (
	*raft.Raft,
	*raft.Config,
	*raft.InmemTransport,
	*raft.InmemStore,
	*raft.InmemSnapshotStore,
) {
	_, transport := raft.NewInmemTransport("")
	s.AddCleanup(func(*gc.C) { transport.Close() })

	store := raft.NewInmemStore()
	snapshotStore := raft.NewInmemSnapshotStore()
	config := s.DefaultConfig(id)

	r, err := raft.NewRaft(config, fsm, store, store, snapshotStore, transport)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		f := r.Shutdown()
		c.Assert(f.Error(), jc.ErrorIsNil)
	})
	return r, config, transport, store, snapshotStore
}

func (s *RaftFixture) DefaultConfig(id raft.ServerID) *raft.Config {
	raftConfig := raft.DefaultConfig()
	raftConfig.ShutdownOnRemove = false
	raftConfig.LocalID = id
	raftConfig.HeartbeatTimeout = 100 * time.Millisecond
	raftConfig.ElectionTimeout = raftConfig.HeartbeatTimeout
	raftConfig.LeaderLeaseTimeout = raftConfig.HeartbeatTimeout
	raftConfig.Logger = log.New(&raftutil.LoggoWriter{
		loggo.GetLogger("juju.worker.raft.raftclusterer_test_" + string(id)),
		loggo.DEBUG,
	}, "", 0)
	return raftConfig
}
