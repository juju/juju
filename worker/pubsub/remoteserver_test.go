// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/pubsub/centralhub"
	"github.com/juju/juju/pubsub/forwarder"
	coretesting "github.com/juju/juju/testing"
	psworker "github.com/juju/juju/worker/pubsub"
	"github.com/juju/juju/worker/workertest"
)

type RemoteServerSuite struct {
	testing.IsolationSuite
	connectionOpener *fakeConnectionOpener
	config           psworker.RemoteServerConfig
	clock            *testing.Clock
	hub              *pubsub.StructuredHub
	origin           string
}

var _ = gc.Suite(&RemoteServerSuite{})

func (s *RemoteServerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	logger := loggo.GetLogger("juju.worker.pubsub")
	logger.SetLogLevel(loggo.TRACE)
	s.connectionOpener = &fakeConnectionOpener{}
	tag := names.NewMachineTag("42")
	s.clock = testing.NewClock(time.Now())
	s.hub = centralhub.New(tag)
	s.origin = tag.String()
	s.config = psworker.RemoteServerConfig{
		Hub:    s.hub,
		Origin: s.origin,
		Target: "target",
		Clock:  s.clock,
		Logger: logger,
		APIInfo: &api.Info{
			Addrs:  []string{"localhost"},
			CACert: "fake as",
			Tag:    tag,
		},
		NewWriter: s.connectionOpener.newWriter,
	}
}

func (s *RemoteServerSuite) TestCleanShutdown(c *gc.C) {
	server, err := psworker.NewRemoteServer(s.config)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, server)
}

func (s *RemoteServerSuite) TestConnectPublished(c *gc.C) {
	done := make(chan struct{})
	unsub, err := s.config.Hub.Subscribe(forwarder.ConnectedTopic, func(_ string, data map[string]interface{}) {
		c.Check(data["target"], gc.Equals, "target")
		c.Check(data["origin"], gc.Equals, "machine-42")
		close(done)
	})
	c.Assert(err, jc.ErrorIsNil)
	defer unsub()
	server, err := psworker.NewRemoteServer(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, server)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("no connect message published")
	}
	// Make sure that it is reported as started.
	r, ok := server.(psworker.Reporter)
	c.Assert(ok, jc.IsTrue)
	// Since we are just testing the remote, the code that makes sure the
	// published message is forwarded is the subscriber, so we will always
	// show empty queue and none sent.
	c.Check(r.IntrospectionReport(), gc.Equals, ""+
		"  Status: connected\n"+
		"  Addresses: [localhost]\n"+
		"  Queue length: 0\n"+
		"  Sent count: 0\n")
}

func (s *RemoteServerSuite) TestDisconnectPublishedOnWriteError(c *gc.C) {
	done := make(chan struct{})
	unsub, err := s.config.Hub.Subscribe(forwarder.DisconnectedTopic, func(_ string, data map[string]interface{}) {
		c.Check(data["target"], gc.Equals, "target")
		c.Check(data["origin"], gc.Equals, "machine-42")
		select {
		case <-done:
			c.Fatal("closed already")
		default:
			close(done)
		}
	})
	c.Assert(err, jc.ErrorIsNil)
	defer unsub()
	s.connectionOpener.forwardErr = errors.New("forward fail")

	server := s.newConnectedServer(c)
	server.Publish(&params.PubSubMessage{
		Topic: "some topic",
	})

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("no disconnect message published")
	}
}

func (s *RemoteServerSuite) TestConnectErrorRetryDelay(c *gc.C) {
	now := s.clock.Now()
	delays := make([]string, 0)
	s.connectionOpener.err = errors.New("oops")
	s.connectionOpener.callback = func(_ *api.Info) {
		delay := s.clock.Now().Sub(now)
		now = s.clock.Now()
		delays = append(delays, fmt.Sprint(delay))
	}

	server, err := psworker.NewRemoteServer(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, server)

	for i := 0; i < 1200; i++ {
		s.clock.WaitAdvance(time.Second, coretesting.ShortWait, 1)
	}
	// Starts immediately, with a one second delay doubling each failure
	// up to a max wait time of 5 minutes.
	c.Assert(delays, jc.DeepEquals, []string{
		"0s", "1s", "2s", "4s", "8s", "16s", "32s",
		"1m4s", "2m8s", "4m16s",
		"5m0s", "5m0s",
	})
}

func (s *RemoteServerSuite) TestConnectRetryInterruptedOnTargetConnection(c *gc.C) {
	now := s.clock.Now()
	delays := make([]string, 0)
	s.connectionOpener.err = errors.New("oops")
	s.connectionOpener.callback = func(_ *api.Info) {
		delay := s.clock.Now().Sub(now)
		now = s.clock.Now()
		delays = append(delays, fmt.Sprint(delay))
	}

	server, err := psworker.NewRemoteServer(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, server)

	for i := 0; i < 35; i++ {
		s.clock.WaitAdvance(time.Second, coretesting.ShortWait, 1)
	}
	// This leaves us 4s into a 32s retry wait.
	done, err := s.hub.Publish(forwarder.ConnectedTopic, forwarder.OriginTarget{
		Target: s.origin,
		Origin: "target",
	})
	c.Assert(err, jc.ErrorIsNil)
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatal("worker didn't consume the event")
	}

	// Now advance the clock some more
	for i := 0; i < 10; i++ {
		s.clock.WaitAdvance(time.Second, coretesting.ShortWait, 2)
	}

	c.Assert(delays, jc.DeepEquals, []string{
		"0s", "1s", "2s", "4s", "8s", "16s", // standard fallback
		"5s",             // 4s due to interruption, 1s due to loop delay on failure
		"1s", "2s", "4s", // standard fallback
	})
}

func (s *RemoteServerSuite) TestConnectRetryInterruptedWithNewAddresses(c *gc.C) {
	now := s.clock.Now()
	delays := make([]string, 0)
	expected := []string{"localhost"}
	s.connectionOpener.err = errors.New("oops")
	s.connectionOpener.callback = func(info *api.Info) {
		c.Check(info.Addrs, jc.DeepEquals, expected)
		delay := s.clock.Now().Sub(now)
		now = s.clock.Now()
		delays = append(delays, fmt.Sprint(delay))
	}

	server, err := psworker.NewRemoteServer(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, server)

	for i := 0; i < 35; i++ {
		s.clock.WaitAdvance(time.Second, coretesting.ShortWait, 1)
	}
	// This leaves us 4s into a 32s retry wait.

	expected = []string{"new addresses"}
	server.UpdateAddresses(expected)

	// Now advance the clock some more
	for i := 0; i < 10; i++ {
		s.clock.WaitAdvance(time.Second, coretesting.ShortWait, 2)
	}

	c.Assert(delays, jc.DeepEquals, []string{
		"0s", "1s", "2s", "4s", "8s", "16s", // standard fallback
		"5s",             // 4s due to interruption, 1s due to loop delay on failure
		"1s", "2s", "4s", // standard fallback
	})
}

func (s *RemoteServerSuite) newConnectedServer(c *gc.C) psworker.RemoteServer {
	connected := make(chan struct{})
	unsub, err := s.config.Hub.Subscribe(forwarder.ConnectedTopic, func(_ string, _ map[string]interface{}) {
		close(connected)
	})
	c.Assert(err, jc.ErrorIsNil)
	defer unsub()

	server, err := psworker.NewRemoteServer(s.config)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { workertest.CleanKill(c, server) })

	select {
	case <-connected:
	case <-time.After(coretesting.LongWait):
		c.Fatal("no connect message published")
	}
	return server
}

func (s *RemoteServerSuite) TestSendsMessages(c *gc.C) {
	numMessages := 10
	done := make(chan struct{})
	// Close the done channel when the writer has received the
	// appropriate number of messages
	go func() {
		defer close(done)
		for {
			if s.writer().count() == numMessages {
				return
			}
		}
	}()

	server := s.newConnectedServer(c)

	for i := 0; i < numMessages; i++ {
		server.Publish(&params.PubSubMessage{
			Topic: fmt.Sprintf("topic.%d", i),
		})
	}

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("not all messages received, got %d", s.writer().count())
	}

	for i := 0; i < numMessages; i++ {
		c.Check(s.writer().messages[i].Topic, gc.Equals, fmt.Sprintf("topic.%d", i))
	}
}

func (s *RemoteServerSuite) writer() *messageWriter {
	writer := s.connectionOpener.getWriter()
	if writer == nil {
		return &messageWriter{}
	}
	return writer
}

type fakeConnectionOpener struct {
	mutex      sync.Mutex
	err        error
	callback   func(*api.Info)
	writer     *messageWriter
	forwardErr error
}

func (f *fakeConnectionOpener) getWriter() *messageWriter {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.writer
}

func (f *fakeConnectionOpener) newWriter(info *api.Info) (psworker.MessageWriter, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if f.callback != nil {
		f.callback(info)
	}
	if f.err != nil {
		return nil, f.err
	}
	f.writer = &messageWriter{err: f.forwardErr}
	return f.writer, nil
}

type messageWriter struct {
	messages []*params.PubSubMessage
	mutex    sync.Mutex
	err      error
}

func (m *messageWriter) count() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return len(m.messages)
}

func (m *messageWriter) ForwardMessage(message *params.PubSubMessage) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, message)
	return nil
}

func (*messageWriter) Close() {}
