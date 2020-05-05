// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftbackstop_test

import (
	"bytes"
	"sync"
	"time"

	"github.com/hashicorp/go-msgpack/codec"
	"github.com/hashicorp/raft"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/raft/raftbackstop"
)

type workerFixture struct {
	testing.IsolationSuite
	raft     *mockRaft
	logStore *mockLogStore
	hub      *pubsub.StructuredHub
	config   raftbackstop.Config
}

func (s *workerFixture) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	tag := names.NewMachineTag("23")
	s.raft = &mockRaft{}
	s.logStore = &mockLogStore{}
	s.hub = centralhub.New(tag)
	s.config = raftbackstop.Config{
		Raft:     s.raft,
		LogStore: s.logStore,
		Hub:      s.hub,
		LocalID:  "23",
		Logger:   loggo.GetLogger("raftbackstop_test"),
	}
}

type WorkerValidationSuite struct {
	workerFixture
}

var _ = gc.Suite(&WorkerValidationSuite{})

func (s *WorkerValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*raftbackstop.Config)
		expect string
	}
	tests := []test{{
		func(cfg *raftbackstop.Config) { cfg.Raft = nil },
		"nil Raft not valid",
	}, {
		func(cfg *raftbackstop.Config) { cfg.Hub = nil },
		"nil Hub not valid",
	}, {
		func(cfg *raftbackstop.Config) { cfg.LogStore = nil },
		"nil LogStore not valid",
	}, {
		func(cfg *raftbackstop.Config) { cfg.LocalID = "" },
		"empty LocalID not valid",
	}, {
		func(cfg *raftbackstop.Config) { cfg.Logger = nil },
		"nil Logger not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerValidationSuite) testValidateError(c *gc.C, f func(*raftbackstop.Config), expect string) {
	config := s.config
	f(&config)
	w, err := raftbackstop.NewWorker(config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

type WorkerSuite struct {
	workerFixture
	worker worker.Worker
	reqs   chan apiserver.DetailsRequest
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) SetUpTest(c *gc.C) {
	s.workerFixture.SetUpTest(c)
	s.reqs = make(chan apiserver.DetailsRequest, 10)

	// Use a local variable to send to the channel in the callback, so
	// we don't get races when a subsequent test overwrites s.reqs
	// with a new channel.
	reqs := s.reqs
	unsubscribe, err := s.hub.Subscribe(
		apiserver.DetailsRequestTopic,
		func(topic string, req apiserver.DetailsRequest, err error) {
			c.Check(err, jc.ErrorIsNil)
			reqs <- req
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { unsubscribe() })

	worker, err := raftbackstop.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) {
		workertest.DirtyKill(c, worker)
	})
	s.worker = worker
}

func (s *WorkerSuite) TestCleanKill(c *gc.C) {
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestRequestsDetails(c *gc.C) {
	// The worker is started in SetUpTest.
	select {
	case req := <-s.reqs:
		c.Assert(req, gc.Equals, apiserver.DetailsRequest{
			Requester: "raft-backstop",
			LocalOnly: true,
		})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for details request")
	}
}

func (s *WorkerSuite) findStoreLogCalls() []*raft.Log {
	var results []*raft.Log
	for _, call := range s.logStore.Calls() {
		if call.FuncName != "StoreLog" {
			continue
		}
		results = append(results, call.Args[0].(*raft.Log))
	}
	return results
}

func (s *WorkerSuite) assertRecovery(c *gc.C, index, term uint64, server raft.Server) {
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		if len(s.findStoreLogCalls()) != 0 {
			break
		}
	}
	storedLogs := s.findStoreLogCalls()
	c.Assert(storedLogs, gc.HasLen, 1)
	log := storedLogs[0]
	c.Assert(log.Type, gc.Equals, raft.LogConfiguration)
	c.Assert(log.Index, gc.Equals, index)
	c.Assert(log.Term, gc.Equals, term)
	c.Assert(decodeConfiguration(c, log.Data), gc.DeepEquals, raft.Configuration{
		Servers: []raft.Server{server},
	})
}

func (s *WorkerSuite) TestRecoversClusterOneNonvoter(c *gc.C) {
	s.raft.setValues(raft.Follower, &mockConfigFuture{conf: raft.Configuration{
		Servers: []raft.Server{{
			ID:       "23",
			Address:  "address",
			Suffrage: raft.Nonvoter,
		}},
	}})
	// We don't care about other fields in this case.
	s.logStore.setLastLog(&raft.Log{
		Index: 451,
		Term:  66,
	})
	s.publishDetails(c, map[string]string{"23": "address"})
	s.assertRecovery(c, 452, 66, raft.Server{
		ID:       "23",
		Address:  "address",
		Suffrage: raft.Voter,
	})
}

func (s *WorkerSuite) TestRecoversClusterTwoVoters(c *gc.C) {
	s.raft.setValues(raft.Follower, &mockConfigFuture{conf: raft.Configuration{
		Servers: []raft.Server{{
			ID:       "23",
			Address:  "address",
			Suffrage: raft.Voter,
		}, {
			ID:       "100",
			Address:  "otheraddress",
			Suffrage: raft.Voter,
		}},
	}})
	s.logStore.setLastLog(&raft.Log{
		Index: 451,
		Term:  66,
	})
	s.publishDetails(c, map[string]string{"23": "address"})
	s.assertRecovery(c, 452, 66, raft.Server{
		ID:       "23",
		Address:  "address",
		Suffrage: raft.Voter,
	})
}

func (s WorkerSuite) TestRecoversClusterNoVoters(c *gc.C) {
	s.raft.setValues(raft.Follower, &mockConfigFuture{conf: raft.Configuration{
		Servers: []raft.Server{},
	}})
	// we don't setLastLog because there is no log entry in this case
	s.publishDetails(c, map[string]string{"23": "address"})
	var conf raft.Configuration
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		conf = s.raft.GetConfiguration().Configuration()
		if len(conf.Servers) != 0 {
			break
		}
	}
	c.Check(conf.Servers, gc.DeepEquals, []raft.Server{{
		Suffrage: raft.Voter,
		ID:       "23",
		Address:  "address",
	}})
}

func (s *WorkerSuite) assertNoRecovery(c *gc.C) {
	time.Sleep(coretesting.ShortWait)
	c.Assert(s.findStoreLogCalls(), gc.HasLen, 0)
}

func (s *WorkerSuite) TestOnlyRecoversClusterOnce(c *gc.C) {
	s.raft.setValues(raft.Follower, &mockConfigFuture{conf: raft.Configuration{
		Servers: []raft.Server{{
			ID:       "23",
			Address:  "address",
			Suffrage: raft.Nonvoter,
		}},
	}})
	// We don't care about other fields in this case.
	s.logStore.setLastLog(&raft.Log{
		Index: 451,
		Term:  66,
	})
	s.publishDetails(c, map[string]string{"23": "address"})
	s.assertRecovery(c, 452, 66, raft.Server{
		ID:       "23",
		Address:  "address",
		Suffrage: raft.Voter,
	})
	s.logStore.ResetCalls()
	s.publishDetails(c, map[string]string{"23": "address"})
	s.assertNoRecovery(c)
}

func (s *WorkerSuite) TestNoRecoveryIfMultipleMachines(c *gc.C) {
	s.raft.setValues(raft.Follower, &mockConfigFuture{conf: raft.Configuration{
		Servers: []raft.Server{{
			ID:       "23",
			Address:  "address",
			Suffrage: raft.Voter,
		}, {
			ID:       "100",
			Address:  "otheraddress",
			Suffrage: raft.Voter,
		}},
	}})
	s.logStore.setLastLog(&raft.Log{
		Index: 451,
		Term:  66,
	})
	s.publishDetails(c, map[string]string{
		"23":  "address",
		"100": "otheraddress",
	})
	s.assertNoRecovery(c)
}

func (s *WorkerSuite) TestNoRecoveryIfNotInServerDetails(c *gc.C) {
	s.raft.setValues(raft.Follower, &mockConfigFuture{conf: raft.Configuration{
		Servers: []raft.Server{{
			ID:       "23",
			Address:  "address",
			Suffrage: raft.Voter,
		}, {
			ID:       "100",
			Address:  "otheraddress",
			Suffrage: raft.Voter,
		}},
	}})
	s.logStore.setLastLog(&raft.Log{
		Index: 451,
		Term:  66,
	})
	s.publishDetails(c, map[string]string{"100": "otheraddress"})
	s.assertNoRecovery(c)
}

func (s *WorkerSuite) TestNoRecoveryIfNotInRaftConfig(c *gc.C) {
	s.raft.setValues(raft.Follower, &mockConfigFuture{conf: raft.Configuration{
		Servers: []raft.Server{{
			ID:       "100",
			Address:  "otheraddress",
			Suffrage: raft.Voter,
		}},
	}})
	s.logStore.setLastLog(&raft.Log{
		Index: 451,
		Term:  66,
	})
	s.publishDetails(c, map[string]string{
		"23": "address",
	})
	s.assertNoRecovery(c)
}

func (s *WorkerSuite) TestNoRecoveryIfOneRaftNodeAndVoter(c *gc.C) {
	s.raft.setValues(raft.Follower, &mockConfigFuture{conf: raft.Configuration{
		Servers: []raft.Server{{
			ID:       "23",
			Address:  "address",
			Suffrage: raft.Voter,
		}},
	}})
	s.logStore.setLastLog(&raft.Log{
		Index: 451,
		Term:  66,
	})
	s.publishDetails(c, map[string]string{"23": "address"})
	s.assertNoRecovery(c)
}

func (s *WorkerSuite) publishDetails(c *gc.C, serverAddrs map[string]string) {
	details := makeDetails(serverAddrs)
	received, err := s.hub.Publish(apiserver.DetailsTopic, details)
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-received:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for details to be received")
	}
}

func makeDetails(serverInfo map[string]string) apiserver.Details {
	servers := make(map[string]apiserver.APIServer)
	for id, address := range serverInfo {
		servers[id] = apiserver.APIServer{
			ID:              id,
			InternalAddress: address,
		}
	}
	return apiserver.Details{Servers: servers}
}

func decodeConfiguration(c *gc.C, data []byte) (out raft.Configuration) {
	buf := bytes.NewReader(data)
	hd := codec.MsgpackHandle{}
	dec := codec.NewDecoder(buf, &hd)
	err := dec.Decode(&out)
	c.Assert(err, jc.ErrorIsNil)
	return out
}

type mockRaft struct {
	mu    sync.Mutex
	state raft.RaftState
	cf    *mockConfigFuture
}

func (r *mockRaft) State() raft.RaftState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *mockRaft) GetConfiguration() raft.ConfigurationFuture {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cf
}

func (r *mockRaft) BootstrapCluster(conf raft.Configuration) raft.Future {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cf.conf = conf
	r.state = raft.Leader
	return &mockFuture{err: nil}
}

func (r *mockRaft) setValues(state raft.RaftState, cf *mockConfigFuture) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = state
	cf.mu = &r.mu
	r.cf = cf
}

type mockFuture struct {
	err error
}

func (f *mockFuture) Error() error {
	return f.err
}

var _ raft.Future = (*mockFuture)(nil)

type mockConfigFuture struct {
	raft.IndexFuture
	mu *sync.Mutex // Shared mutex passed in from mockRaft
	testing.Stub
	conf raft.Configuration
}

func (f *mockConfigFuture) Error() error {
	f.AddCall("Error")
	return f.NextErr()
}

func (f *mockConfigFuture) Configuration() raft.Configuration {
	f.AddCall("Configuration")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conf.Clone()
}

type mockLogStore struct {
	raft.LogStore
	testing.Stub
	mu      sync.Mutex
	lastLog raft.Log
}

func (s *mockLogStore) LastIndex() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AddCall("LastIndex")
	return s.lastLog.Index, s.NextErr()
}

func (s *mockLogStore) GetLog(index uint64, out *raft.Log) error {
	s.AddCall("GetLog", index, out)
	*out = s.lastLog
	return s.NextErr()
}

func (s *mockLogStore) StoreLog(log *raft.Log) error {
	s.AddCall("StoreLog", log)
	return s.NextErr()
}

func (s *mockLogStore) setLastLog(lastLog *raft.Log) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastLog = *lastLog
}
