// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"log"
	"path/filepath"

	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
	"github.com/juju/replicaset"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
	raftworker "github.com/juju/juju/worker/raft"
	"github.com/juju/juju/worker/raft/rafttest"
)

type raftSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&raftSuite{})

func (s *raftSuite) TestBootstrapRaft(c *gc.C) {
	votes := 1
	noVotes := 0
	dataDir := c.MkDir()
	context := &mockContext{
		agentConfig: &mockAgentConfig{
			tag:     names.NewMachineTag("23"),
			dataDir: dataDir,
		},
		state: &mockState{
			members: []replicaset.Member{{
				Address: "somewhere.else:37012",
				Tags:    map[string]string{"juju-machine-id": "42"},
			}, {
				Address: "nowhere.else:37012",
				Tags:    map[string]string{"juju-machine-id": "23"},
				Votes:   &votes,
			}, {
				Address: "everywhere.else:37012",
				Tags:    map[string]string{"juju-machine-id": "7"},
				Votes:   &noVotes,
			}},
			info: state.StateServingInfo{APIPort: 1234},
		},
	}
	err := upgrades.BootstrapRaft(context)
	c.Assert(err, jc.ErrorIsNil)

	// Now make the raft node and check that the configuration is as
	// we expect.

	// Capture logging to include in test output.
	output := captureWriter{c}
	config := raft.DefaultConfig()
	config.LocalID = "23"
	config.Logger = log.New(output, "", 0)
	c.Assert(raft.ValidateConfig(config), jc.ErrorIsNil)

	raftDir := filepath.Join(dataDir, "raft")
	logStore, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(raftDir, "logs"),
	})
	c.Assert(err, jc.ErrorIsNil)

	snapshotStore, err := raft.NewFileSnapshotStore(raftDir, 1, output)
	c.Assert(err, jc.ErrorIsNil)
	_, transport := raft.NewInmemTransport(raft.ServerAddress("nowhere.else"))

	r, err := raft.NewRaft(
		config,
		&raftworker.SimpleFSM{},
		logStore,
		logStore,
		snapshotStore,
		transport,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		c.Assert(r.Shutdown().Error(), jc.ErrorIsNil)
	})

	rafttest.CheckConfiguration(c, r, []raft.Server{{
		ID:       "42",
		Address:  "somewhere.else:1234",
		Suffrage: raft.Voter,
	}, {
		ID:       "23",
		Address:  "nowhere.else:1234",
		Suffrage: raft.Voter,
	}, {
		ID:       "7",
		Address:  "everywhere.else:1234",
		Suffrage: raft.Nonvoter,
	}})
}

type mockState struct {
	upgrades.StateBackend
	stub    testing.Stub
	members []replicaset.Member
	info    state.StateServingInfo
}

func (s *mockState) ReplicaSetMembers() ([]replicaset.Member, error) {
	return s.members, s.stub.NextErr()
}

func (s *mockState) StateServingInfo() (state.StateServingInfo, error) {
	return s.info, s.stub.NextErr()
}

type captureWriter struct {
	c *gc.C
}

func (w captureWriter) Write(p []byte) (int, error) {
	w.c.Logf("%s", p[:len(p)-1]) // omit trailling newline
	return len(p), nil
}
