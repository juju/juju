// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"context"
	"math/rand"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/pubsub/apiserver"
)

type RaftLeaseClientValidationSuite struct {
	testing.IsolationSuite

	config Config
}

var _ = gc.Suite(&RaftLeaseClientValidationSuite{})

func (s *RaftLeaseClientValidationSuite) SetUpTest(c *gc.C) {
	s.config = Config{
		Hub:     pubsub.NewStructuredHub(nil),
		APIInfo: &api.Info{},
		NewRemote: func(config RemoteConfig) Remote {
			return nil
		},
		Logger:        fakeLogger{},
		ClientMetrics: fakeClientMetrics{},
		Random:        rand.New(rand.NewSource(time.Now().UnixNano())),
		Clock:         clock.WallClock,
	}
}

func (s *RaftLeaseClientValidationSuite) TestValidateConfig(c *gc.C) {
	c.Assert(s.config.Validate(), jc.ErrorIsNil)
}

func (s *RaftLeaseClientValidationSuite) TestValidateErrors(c *gc.C) {
	type test struct {
		f      func(*Config)
		expect string
	}
	tests := []test{{
		func(cfg *Config) { cfg.Hub = nil },
		"nil Hub not valid",
	}, {
		func(cfg *Config) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *Config) { cfg.APIInfo = nil },
		"nil APIInfo not valid",
	}, {
		func(cfg *Config) { cfg.NewRemote = nil },
		"nil NewRemote not valid",
	}, {
		func(cfg *Config) { cfg.ClientMetrics = nil },
		"nil ClientMetrics not valid",
	}, {
		func(cfg *Config) { cfg.Random = nil },
		"nil Random not valid",
	}, {
		func(cfg *Config) { cfg.Clock = nil },
		"nil Clock not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *RaftLeaseClientValidationSuite) testValidateError(c *gc.C, f func(*Config), expect string) {
	config := s.config
	f(&config)
	c.Check(config.Validate(), gc.ErrorMatches, expect)
}

type RaftLeaseClientSuite struct {
	testing.IsolationSuite

	remote *MockRemote
	config Config
}

var _ = gc.Suite(&RaftLeaseClientSuite{})

func (s *RaftLeaseClientSuite) TestNewClientWithNoAPIAddresses(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := NewClient(s.config)
	c.Assert(err, gc.ErrorMatches, `api addresses not found`)
}

func (s *RaftLeaseClientSuite) TestNewClientWithAPIAddresses(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.config.APIInfo.Addrs = []string{"localhost", "10.0.0.8"}

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	c.Assert(client.servers, gc.HasLen, 2)
}

func (s *RaftLeaseClientSuite) TestRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := &raftlease.Command{}

	s.config.APIInfo.Addrs = []string{"localhost"}

	s.remote.EXPECT().Request(gomock.Any(), cmd).Return(nil)

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	err = client.Request(context.TODO(), cmd)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that we have a known remote to reuse.
	c.Assert(client.lastKnownRemote, gc.NotNil)
}

func (s *RaftLeaseClientSuite) TestRequestWithNotLeaderError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := &raftlease.Command{}

	s.config.APIInfo.Addrs = []string{"localhost"}

	s.remote.EXPECT().Address().Return("localhost").AnyTimes()
	s.remote.EXPECT().Request(gomock.Any(), cmd).Return(apiservererrors.NewNotLeaderError("", ""))

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	err = client.Request(context.TODO(), cmd)
	c.Assert(err, gc.ErrorMatches, `lease operation dropped`)
	c.Assert(client.lastKnownRemote, gc.IsNil)
}

func (s *RaftLeaseClientSuite) TestRequestWithNotLeaderErrorWithSuggestion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := &raftlease.Command{}

	s.config.APIInfo.Addrs = []string{"localhost", "10.0.0.8"}

	s.remote.EXPECT().Address().Return("localhost")
	s.remote.EXPECT().Address().Return("10.0.0.8").AnyTimes()
	s.remote.EXPECT().Request(gomock.Any(), cmd).Return(apiservererrors.NewNotLeaderError("10.0.0.8", "1"))
	s.remote.EXPECT().Request(gomock.Any(), cmd).Return(nil)

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	err = client.Request(context.TODO(), cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(client.lastKnownRemote, gc.NotNil)
}

func (s *RaftLeaseClientSuite) TestRequestWithCancelledContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := &raftlease.Command{}

	s.config.APIInfo.Addrs = []string{"localhost"}

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	ctx, cancel := context.WithCancel(context.TODO())
	cancel()

	err = client.Request(ctx, cmd)
	c.Assert(err, gc.ErrorMatches, `lease operation timed out`)
	c.Assert(client.lastKnownRemote, gc.IsNil)
}

func (s *RaftLeaseClientSuite) TestRequestWithNoRemotes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := &raftlease.Command{}

	s.config.APIInfo.Addrs = []string{"localhost"}

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	client.servers = make(map[string]Remote)

	err = client.Request(context.TODO(), cmd)
	c.Assert(err, gc.ErrorMatches, `remote servers not found`)
	c.Assert(client.lastKnownRemote, gc.IsNil)
}

func (s *RaftLeaseClientSuite) TestSelectRemote(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.config.APIInfo.Addrs = []string{"localhost"}

	s.remote.EXPECT().Address().Return("localhost")

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	remote, err := client.selectRemote()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remote, gc.NotNil)
	c.Assert(remote.Address(), gc.Equals, "localhost")
}

func (s *RaftLeaseClientSuite) TestSelectRemoteWithNoAddrs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.config.APIInfo.Addrs = []string{"localhost"}

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	client.servers = nil

	_, err = client.selectRemote()
	c.Assert(err, gc.ErrorMatches, `remote servers not found`)
}

func (s *RaftLeaseClientSuite) TestSelectRemoteFromError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.config.APIInfo.Addrs = []string{"localhost"}

	s.remote.EXPECT().Address().Return("localhost")

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	remote, err := client.selectRemoteFromError("localhost", apiservererrors.NewNotLeaderError("localhost", "0"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remote, gc.NotNil)
	c.Assert(remote.Address(), gc.Equals, "localhost")
}

func (s *RaftLeaseClientSuite) TestSelectRemoteFromErrorIDMissmatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.config.APIInfo.Addrs = []string{"localhost"}

	s.remote.EXPECT().Address().Return("localhost")

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	_, err = client.selectRemoteFromError("localhost", apiservererrors.NewNotLeaderError("localhost", "1"))
	c.Assert(err, gc.ErrorMatches, `no leader found: remote server connection not found`)
}

func (s *RaftLeaseClientSuite) TestSelectRemoteFromErrorNoData(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.config.APIInfo.Addrs = []string{"localhost"}

	s.remote.EXPECT().Address().Return("localhost")

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	_, err = client.selectRemoteFromError("localhost", apiservererrors.NewNotLeaderError("", ""))
	c.Assert(err, gc.ErrorMatches, `no leader found: remote server connection not found`)
}

func (s *RaftLeaseClientSuite) TestSelectRemoteFromErrorMatchingAnother(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.config.APIInfo.Addrs = []string{"localhost", "10.0.0.8"}

	s.remote.EXPECT().Address().Return("localhost")
	s.remote.EXPECT().Address().Return("10.0.0.8").Times(2)

	client := &Client{
		config:  s.config,
		servers: make(map[string]Remote),
	}

	err := client.initServers()
	c.Assert(err, jc.ErrorIsNil)

	// Try and locate another remote to use.
	remote, err := client.selectRemoteFromError("localhost", apiservererrors.NewNotLeaderError("", ""))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(remote, gc.NotNil)
	c.Assert(remote.Address(), gc.Equals, "10.0.0.8")
}

func (s *RaftLeaseClientSuite) TestGatherAddresses(c *gc.C) {
	client := &Client{}
	servers := client.gatherAddresses(apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"0": {
				InternalAddress: "10.0.0.8",
			},
			"1": {
				Addresses: []string{"10.0.0.7"},
			},
			"2": {},
		},
	})
	c.Assert(servers, gc.DeepEquals, map[string]string{
		"0": "10.0.0.8",
		"1": "10.0.0.7",
	})
}

func (s *RaftLeaseClientSuite) TestEnsureServers(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.remote.EXPECT().Address().Return("10.0.0.8")
	s.remote.EXPECT().Address().Return("10.0.0.7")

	// This one overrides the localhost one.
	s.remote.EXPECT().SetAddress("10.0.0.8")
	s.remote.EXPECT().Kill().AnyTimes()
	s.remote.EXPECT().Wait().AnyTimes()

	s.config.APIInfo.Addrs = []string{"localhost"}

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	err = client.ensureServers(map[string]string{
		"0": "10.0.0.8",
		"1": "10.0.0.7",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(client.servers, gc.HasLen, 2)
	c.Assert(client.servers["0"].Address(), gc.Equals, "10.0.0.8")
	c.Assert(client.servers["1"].Address(), gc.Equals, "10.0.0.7")
}

func (s *RaftLeaseClientSuite) TestEnsureServersRemovesOld(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.remote.EXPECT().Address().Return("10.0.0.8")
	s.remote.EXPECT().Address().Return("10.0.0.7")

	// This one overrides the localhost one.
	s.remote.EXPECT().SetAddress("10.0.0.8")
	s.remote.EXPECT().SetAddress("10.0.0.7")
	s.remote.EXPECT().Kill().AnyTimes()
	s.remote.EXPECT().Wait().AnyTimes()

	s.config.APIInfo.Addrs = []string{"localhost", "10.0.0.6"}

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	err = client.ensureServers(map[string]string{
		"0": "10.0.0.8",
		"1": "10.0.0.7",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(client.servers, gc.HasLen, 2)
	c.Assert(client.servers["0"].Address(), gc.Equals, "10.0.0.8")
	c.Assert(client.servers["1"].Address(), gc.Equals, "10.0.0.7")
}

func (s *RaftLeaseClientSuite) TestEnsureServersWithNoNewAddresses(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.config.APIInfo.Addrs = []string{"localhost"}

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	err = client.ensureServers(map[string]string{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(client.servers, gc.HasLen, 0)
}

func (s *RaftLeaseClientSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.remote = NewMockRemote(ctrl)

	// We always expect kill and wait for the remote, as we close the client
	// which kills any remotes around.
	s.remote.EXPECT().Kill().AnyTimes()
	s.remote.EXPECT().Wait().AnyTimes()

	s.config = Config{
		Hub:     pubsub.NewStructuredHub(nil),
		APIInfo: &api.Info{},
		NewRemote: func(config RemoteConfig) Remote {
			return s.remote
		},
		Logger:        fakeLogger{},
		ClientMetrics: fakeClientMetrics{},
		Clock:         clock.WallClock,
		Random:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	return ctrl
}

type RaftLeaseRemoteSuite struct {
	testing.IsolationSuite

	raftLeaseApplier *MockRaftLeaseApplier

	config RemoteConfig
}

var _ = gc.Suite(&RaftLeaseRemoteSuite{})

func (s *RaftLeaseRemoteSuite) TestAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	remote := NewRemote(s.config)
	c.Assert(remote.Address(), gc.Equals, "")
}

func (s *RaftLeaseRemoteSuite) TestSetAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	remote := NewRemote(s.config)
	remote.SetAddress("localhost")
	c.Assert(remote.Address(), gc.Equals, "localhost")
}

func (s *RaftLeaseRemoteSuite) TestRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	remote := remote{
		config: s.config,
		client: s.raftLeaseApplier,
	}

	s.raftLeaseApplier.EXPECT().ApplyLease("version: 0\noperation: \"\"\n").Return(nil)

	err := remote.Request(context.TODO(), &raftlease.Command{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RaftLeaseRemoteSuite) TestRequestErrDropped(c *gc.C) {
	defer s.setupMocks(c).Finish()

	remote := remote{
		config: s.config,
	}
	// This will cause the lease manager to attempt again.
	err := remote.Request(context.TODO(), &raftlease.Command{})
	c.Assert(lease.IsDropped(err), jc.IsTrue)
}

func (s *RaftLeaseRemoteSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.raftLeaseApplier = NewMockRaftLeaseApplier(ctrl)

	s.config = RemoteConfig{
		APIInfo: &api.Info{},
		Logger:  fakeLogger{},
		Clock:   testclock.NewClock(time.Now()),
	}

	return ctrl
}

type fakeLogger struct{}

func (fakeLogger) Errorf(string, ...interface{}) {}
func (fakeLogger) Debugf(string, ...interface{}) {}

type fakeClientMetrics struct{}

func (fakeClientMetrics) RecordOperation(string, string, time.Time) {}
