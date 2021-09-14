// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"context"
	"time"

	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/raftlease"
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

	config Config
}

var _ = gc.Suite(&RaftLeaseClientSuite{})

func (s *RaftLeaseClientSuite) SetUpTest(c *gc.C) {
	s.config = Config{
		Hub:     pubsub.NewStructuredHub(nil),
		APIInfo: &api.Info{},
		NewRemote: func(config RemoteConfig) Remote {
			return fakeRemote{config: config}
		},
		Logger:        fakeLogger{},
		ClientMetrics: fakeClientMetrics{},
	}
}

func (s *RaftLeaseClientSuite) TestNewClientWithNoAPIAddresses(c *gc.C) {
	_, err := NewClient(s.config)
	c.Assert(err, gc.ErrorMatches, `api addresses not found`)
}

func (s *RaftLeaseClientSuite) TestNewClientWithAPIAddresses(c *gc.C) {
	s.config.APIInfo.Addrs = []string{"localhost", "10.0.0.8"}

	client, err := NewClient(s.config)
	c.Assert(err, jc.ErrorIsNil)

	defer func() {
		_ = client.Close()
	}()

	c.Assert(client.servers, gc.HasLen, 2)
	c.Assert(client.servers, jc.DeepEquals, map[string]Remote{
		"0": fakeRemote{
			config: RemoteConfig{
				APIInfo: &api.Info{Addrs: []string{"localhost"}},
				Logger:  s.config.Logger,
			},
		},
		"1": fakeRemote{
			config: RemoteConfig{
				APIInfo: &api.Info{Addrs: []string{"10.0.0.8"}},
				Logger:  s.config.Logger,
			},
		},
	})
}

type fakeLogger struct{}

func (fakeLogger) Errorf(string, ...interface{}) {}
func (fakeLogger) Debugf(string, ...interface{}) {}

type fakeClientMetrics struct{}

func (fakeClientMetrics) RecordOperation(string, string, time.Time) {}

type fakeRemote struct {
	config RemoteConfig
}

func (f fakeRemote) Kill()       {}
func (f fakeRemote) Wait() error { return nil }

func (f fakeRemote) Address() string {
	return f.config.APIInfo.Addrs[0]
}

func (f fakeRemote) SetAddress(addr string) {
	f.config.APIInfo.Addrs = []string{addr}
}

func (f fakeRemote) Request(context.Context, *raftlease.Command) error {
	panic("request called")
}
