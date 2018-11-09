// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub_test

import (
	"fmt"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/pubsub/apiserver"
	"github.com/juju/juju/pubsub/centralhub"
	coretesting "github.com/juju/juju/testing"
	psworker "github.com/juju/juju/worker/pubsub"
)

type WorkerConfigSuite struct {
}

var _ = gc.Suite(&WorkerConfigSuite{})

func (*WorkerConfigSuite) TestValidate(c *gc.C) {
	logger := loggo.GetLogger("juju.worker.pubsub")
	for i, test := range []struct {
		cfg      psworker.WorkerConfig
		errMatch string
	}{
		{
			errMatch: "missing origin not valid",
		}, {
			cfg: psworker.WorkerConfig{
				Origin: "origin",
			},
			errMatch: "missing clock not valid",
		}, {
			cfg: psworker.WorkerConfig{
				Origin: "origin",
				Clock:  testclock.NewClock(time.Now()),
			},
			errMatch: "missing hub not valid",
		}, {
			cfg: psworker.WorkerConfig{
				Origin: "origin",
				Clock:  testclock.NewClock(time.Now()),
				Hub:    pubsub.NewStructuredHub(nil),
			},
			errMatch: "missing logger not valid",
		}, {
			cfg: psworker.WorkerConfig{
				Origin: "origin",
				Clock:  testclock.NewClock(time.Now()),
				Hub:    pubsub.NewStructuredHub(nil),
				Logger: logger,
			},
			errMatch: "missing api info not valid",
		}, {
			cfg: psworker.WorkerConfig{
				Origin: "origin",
				Clock:  testclock.NewClock(time.Now()),
				Hub:    pubsub.NewStructuredHub(nil),
				Logger: logger,
				APIInfo: &api.Info{
					Addrs: []string{"localhost"},
				},
			},
			errMatch: "missing new writer not valid",
		}, {
			cfg: psworker.WorkerConfig{
				Origin: "origin",
				Clock:  testclock.NewClock(time.Now()),
				Hub:    pubsub.NewStructuredHub(nil),
				Logger: logger,
				APIInfo: &api.Info{
					Addrs: []string{"localhost"},
				},
				NewWriter: func(*api.Info) (psworker.MessageWriter, error) {
					return &messageWriter{}, nil
				},
			},
			errMatch: "missing new remote not valid",
		}, {
			cfg: psworker.WorkerConfig{
				Origin: "origin",
				Clock:  testclock.NewClock(time.Now()),
				Hub:    pubsub.NewStructuredHub(nil),
				Logger: logger,
				APIInfo: &api.Info{
					Addrs: []string{"localhost"},
				},
				NewWriter: func(*api.Info) (psworker.MessageWriter, error) {
					return &messageWriter{}, nil
				},
				NewRemote: func(psworker.RemoteServerConfig) (psworker.RemoteServer, error) {
					return &fakeRemote{}, nil
				},
			},
		},
	} {
		c.Logf("test %d", i)
		err := test.cfg.Validate()
		if test.errMatch != "" {
			c.Check(err, gc.ErrorMatches, test.errMatch)
			c.Check(err, jc.Satisfies, errors.IsNotValid)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
	}
}

type SubscriberSuite struct {
	testing.IsolationSuite
	config  psworker.WorkerConfig
	clock   *testclock.Clock
	hub     *pubsub.StructuredHub
	origin  string
	remotes *fakeRemoteTracker
}

var _ = gc.Suite(&SubscriberSuite{})

func (s *SubscriberSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	logger := loggo.GetLogger("juju.worker.pubsub")
	logger.SetLogLevel(loggo.TRACE)
	// loggo.GetLogger("pubsub").SetLogLevel(loggo.TRACE)
	tag := names.NewMachineTag("42")
	s.clock = testclock.NewClock(time.Now())
	s.hub = centralhub.New(tag)
	s.origin = tag.String()
	s.remotes = &fakeRemoteTracker{
		remotes: make(map[string]*fakeRemote),
	}
	s.config = psworker.WorkerConfig{
		Origin: s.origin,
		Clock:  s.clock,
		Hub:    s.hub,
		Logger: logger,
		APIInfo: &api.Info{
			Addrs:  []string{"localhost"},
			CACert: "fake as",
			Tag:    tag,
		},
		NewWriter: func(*api.Info) (psworker.MessageWriter, error) {
			return &messageWriter{}, nil
		},
		NewRemote: s.remotes.new,
	}
}

func (s *SubscriberSuite) TestBadConfig(c *gc.C) {
	s.config.Clock = nil
	w, err := psworker.NewWorker(s.config)
	c.Assert(err, gc.ErrorMatches, "missing clock not valid")
	c.Assert(w, gc.IsNil)
}

func (s *SubscriberSuite) TestCleanShutdown(c *gc.C) {
	w, err := psworker.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *SubscriberSuite) TestNoInitialRemotes(c *gc.C) {
	w, err := psworker.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Assert(s.remotes.remotes, gc.HasLen, 0)
}

func (s *SubscriberSuite) enableHA(c *gc.C) {
	done, err := s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"3": {
				ID:        "3",
				Addresses: []string{"10.1.2.3"},
			},
			"5": {
				ID:        "5",
				Addresses: []string{"10.1.2.5"},
			},
			"42": {
				ID:        "42",
				Addresses: []string{"10.1.2.42"},
			},
		},
		LocalOnly: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("message handling not completed")
	}
}

func (s *SubscriberSuite) newHAWorker(c *gc.C) worker.Worker {
	w, err := psworker.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })
	s.enableHA(c)
	return w
}

func (s *SubscriberSuite) TestEnableHA(c *gc.C) {
	s.newHAWorker(c)

	c.Assert(s.remotes.remotes, gc.HasLen, 2)
	remote3 := s.remotes.remotes["machine-3"]
	c.Assert(remote3.config.APIInfo.Addrs, jc.DeepEquals, []string{"10.1.2.3"})
	remote5 := s.remotes.remotes["machine-5"]
	c.Assert(remote5.config.APIInfo.Addrs, jc.DeepEquals, []string{"10.1.2.5"})
}

func (s *SubscriberSuite) TestEnableHAInternalAddress(c *gc.C) {
	w, err := psworker.NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })
	done, err := s.hub.Publish(apiserver.DetailsTopic, apiserver.Details{
		Servers: map[string]apiserver.APIServer{
			"3": {
				ID:              "3",
				Addresses:       []string{"10.1.2.3"},
				InternalAddress: "10.5.4.3",
			},
			"5": {
				ID:              "5",
				Addresses:       []string{"10.1.2.5"},
				InternalAddress: "10.5.4.4",
			},
			"42": {
				ID:              "42",
				Addresses:       []string{"10.1.2.42"},
				InternalAddress: "10.5.4.5",
			},
		},
		LocalOnly: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("message handling not completed")
	}
	c.Assert(s.remotes.remotes, gc.HasLen, 2)
	remote3 := s.remotes.remotes["machine-3"]
	c.Assert(remote3.config.APIInfo.Addrs, jc.DeepEquals, []string{"10.5.4.3"})
	remote5 := s.remotes.remotes["machine-5"]
	c.Assert(remote5.config.APIInfo.Addrs, jc.DeepEquals, []string{"10.5.4.4"})
}

func (s *SubscriberSuite) TestSameMessagesForwarded(c *gc.C) {
	s.newHAWorker(c)

	var expected []*params.PubSubMessage
	var last <-chan struct{}
	for i := 0; i < 10; i++ {
		message := &params.PubSubMessage{
			Topic: fmt.Sprintf("topic.%d", i),
			Data:  map[string]interface{}{"origin": "machine-42"},
		}
		expected = append(expected, message)
		done, err := s.hub.Publish(message.Topic, nil)
		c.Assert(err, jc.ErrorIsNil)
		last = done
	}
	select {
	case <-last:
		c.Logf("message processing complete")
	case <-time.After(coretesting.LongWait):
		c.Fatal("messages not handled")
	}

	c.Assert(s.remotes.remotes, gc.HasLen, 2)
	remote3 := s.remotes.remotes["machine-3"]
	remote5 := s.remotes.remotes["machine-5"]

	c.Assert(remote3.messages, jc.DeepEquals, expected)
	c.Assert(remote5.messages, jc.DeepEquals, expected)
}

func (s *SubscriberSuite) TestLocalMessagesNotForwarded(c *gc.C) {
	s.newHAWorker(c)

	var last <-chan struct{}
	for i := 0; i < 10; i++ {
		done, err := s.hub.Publish("local.message", map[string]interface{}{
			"foo":        "bar",
			"local-only": true,
		})
		c.Assert(err, jc.ErrorIsNil)
		last = done
	}
	select {
	case <-last:
		c.Logf("message processing complete")
	case <-time.After(coretesting.LongWait):
		c.Fatal("messages not handled")
	}

	c.Assert(s.remotes.remotes, gc.HasLen, 2)
	remote3 := s.remotes.remotes["machine-3"]
	remote5 := s.remotes.remotes["machine-5"]

	c.Assert(remote3.messages, gc.HasLen, 0)
	c.Assert(remote5.messages, gc.HasLen, 0)
}

func (s *SubscriberSuite) TestOtherOriginMessagesNotForwarded(c *gc.C) {
	s.newHAWorker(c)

	var last <-chan struct{}
	for i := 0; i < 10; i++ {
		done, err := s.hub.Publish("not.ours", map[string]interface{}{
			"foo":    "bar",
			"origin": "other",
		})
		c.Assert(err, jc.ErrorIsNil)
		last = done
	}
	select {
	case <-last:
		c.Logf("message processing complete")
	case <-time.After(coretesting.LongWait):
		c.Fatal("messages not handled")
	}

	c.Assert(s.remotes.remotes, gc.HasLen, 2)
	remote3 := s.remotes.remotes["machine-3"]
	remote5 := s.remotes.remotes["machine-5"]

	c.Assert(remote3.messages, gc.HasLen, 0)
	c.Assert(remote5.messages, gc.HasLen, 0)
}

func (s *SubscriberSuite) TestIntrospectionReport(c *gc.C) {
	w := s.newHAWorker(c)

	r, ok := w.(psworker.Reporter)
	c.Assert(ok, jc.IsTrue)
	c.Assert(r.IntrospectionReport(), gc.Equals, ""+
		"Source: machine-42\n"+
		"\n"+
		"Target: machine-3\n"+
		"  Status: connected\n"+
		"  Addresses: [10.1.2.3]\n"+
		"\n"+
		"Target: machine-5\n"+
		"  Status: connected\n"+
		"  Addresses: [10.1.2.5]\n")
}

func (s *SubscriberSuite) TestRequestsDetailsOnceSubscribed(c *gc.C) {
	subscribed := make(chan apiserver.DetailsRequest)
	s.config.Hub.Subscribe(apiserver.DetailsRequestTopic,
		func(_ string, req apiserver.DetailsRequest, err error) {
			c.Check(err, jc.ErrorIsNil)
			subscribed <- req
		},
	)

	s.newHAWorker(c)

	select {
	case req := <-subscribed:
		c.Assert(req, gc.Equals, apiserver.DetailsRequest{Requester: "pubsub-forwarder", LocalOnly: true})
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for details request")
	}
}

var logger = loggo.GetLogger("workertest")

type fakeRemoteTracker struct {
	remotes map[string]*fakeRemote
}

func (f *fakeRemoteTracker) new(config psworker.RemoteServerConfig) (psworker.RemoteServer, error) {
	remote := &fakeRemote{config: config}
	f.remotes[config.Target] = remote
	return remote, nil
}

type fakeRemote struct {
	psworker.RemoteServer
	config   psworker.RemoteServerConfig
	messages []*params.PubSubMessage
}

func (f *fakeRemote) IntrospectionReport() string {
	return fmt.Sprintf(""+
		"  Status: connected\n"+
		"  Addresses: %v\n",
		f.config.APIInfo.Addrs)
}

func (f *fakeRemote) Publish(message *params.PubSubMessage) {
	logger.Debugf("fakeRemote.Publish %s to %s", message.Topic, f.config.Target)
	f.messages = append(f.messages, message)
}
func (f *fakeRemote) UpdateAddresses(addresses []string) {
	f.config.APIInfo.Addrs = addresses
}
func (*fakeRemote) Kill()       {}
func (*fakeRemote) Wait() error { return nil }
