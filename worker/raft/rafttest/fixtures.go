// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rafttest

import (
	"log"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/loggo"
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
	Transport     *raft.InmemTransport
	Store         *raft.InmemStore
	SnapshotStore *raft.InmemSnapshotStore
	Raft          *raft.Raft
}

func (s *RaftFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	c.Assert(s.FSM, gc.NotNil, gc.Commentf("FSM must be set by embedding test suite"))

	s.Store = raft.NewInmemStore()
	s.SnapshotStore = raft.NewInmemSnapshotStore()
	addr, transport := raft.NewInmemTransport("machine-0")
	s.Transport = transport
	s.AddCleanup(func(*gc.C) { s.Transport.Close() })

	raftConfig := s.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(string(addr))

	r, err := raft.NewRaft(raftConfig, s.FSM, s.Store, s.Store, s.SnapshotStore, s.Transport)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		f := r.Shutdown()
		c.Assert(f.Error(), jc.ErrorIsNil)
	})
	s.Raft = r

	c.Assert(s.Raft.BootstrapCluster(raft.Configuration{
		Servers: []raft.Server{{
			ID:      raftConfig.LocalID,
			Address: addr,
		}},
	}).Error(), jc.ErrorIsNil)
	select {
	case <-s.Raft.LeaderCh():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for raft leadership")
	}
}

func (s *RaftFixture) DefaultConfig() *raft.Config {
	raftConfig := raft.DefaultConfig()
	raftConfig.HeartbeatTimeout = 100 * time.Millisecond
	raftConfig.ElectionTimeout = raftConfig.HeartbeatTimeout
	raftConfig.LeaderLeaseTimeout = raftConfig.HeartbeatTimeout
	raftConfig.Logger = log.New(&raftutil.LoggoWriter{
		loggo.GetLogger("juju.worker.raft.raftclusterer_test"),
		loggo.DEBUG,
	}, "", 0)
	return raftConfig
}
