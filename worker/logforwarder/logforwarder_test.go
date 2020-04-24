// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/logfwd"
	"github.com/juju/juju/logfwd/syslog"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/logforwarder"
)

type LogForwarderSuite struct {
	testing.IsolationSuite

	stream *stubStream
	sender *stubSender
	rec    logfwd.Record
}

var _ = gc.Suite(&LogForwarderSuite{})

func (s *LogForwarderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stream = newStubStream()
	s.sender = newStubSender()
	s.rec = logfwd.Record{
		Origin: logfwd.Origin{
			ControllerUUID: "feebdaed-2f18-4fd2-967d-db9663db7bea",
			ModelUUID:      "deadbeef-2f18-4fd2-967d-db9663db7bea",
			Hostname:       "machine-99.deadbeef-2f18-4fd2-967d-db9663db7bea",
			Type:           logfwd.OriginTypeMachine,
			Name:           "99",
			Software: logfwd.Software{
				PrivateEnterpriseNumber: 28978,
				Name:                    "jujud-machine-agent",
				Version:                 version.Current,
			},
		},
		ID:        10,
		Timestamp: time.Now(),
		Level:     loggo.INFO,
		Location: logfwd.SourceLocation{
			Module:   "api.logstream.test",
			Filename: "test.go",
			Line:     42,
		},
		Message: "send to 10.0.0.1",
	}
}

func (s *LogForwarderSuite) newLogForwarderArgs(
	c *gc.C,
	stream logforwarder.LogStream,
	sender *stubSender,
) logforwarder.OpenLogForwarderArgs {
	api := &mockLogForwardConfig{
		enabled: stream != nil,
		host:    "10.0.0.1",
	}
	return s.newLogForwarderArgsWithAPI(c, api, stream, sender)
}

func (s *LogForwarderSuite) newLogForwarderArgsWithAPI(
	c *gc.C,
	configAPI logforwarder.LogForwardConfig,
	stream logforwarder.LogStream,
	sender *stubSender,
) logforwarder.OpenLogForwarderArgs {
	return logforwarder.OpenLogForwarderArgs{
		Caller:           &mockCaller{},
		LogForwardConfig: configAPI,
		ControllerUUID:   "feebdaed-2f18-4fd2-967d-db9663db7bea",
		OpenSink: func(cfg *syslog.RawConfig) (*logforwarder.LogSink, error) {
			sender.host = cfg.Host
			sink := &logforwarder.LogSink{
				sender,
			}
			return sink, nil
		},
		OpenLogStream: func(_ base.APICaller, _ params.LogStreamConfig, controllerUUID string) (logforwarder.LogStream, error) {
			c.Assert(controllerUUID, gc.Equals, "feebdaed-2f18-4fd2-967d-db9663db7bea")
			return stream, nil
		},
		Logger: loggo.GetLogger("test"),
	}
}

func (s *LogForwarderSuite) TestOne(c *gc.C) {
	s.stream.addRecords(c, s.rec)
	lf, err := logforwarder.NewLogForwarder(s.newLogForwarderArgs(c, s.stream, s.sender))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, lf)

	s.sender.waitForSend(c)
	workertest.CleanKill(c, lf)
	s.sender.stub.CheckCalls(c, []testing.StubCall{
		{"Send", []interface{}{[]logfwd.Record{s.rec}}},
		{"Close", nil},
	})
}

func (s *LogForwarderSuite) TestConfigChange(c *gc.C) {
	rec0 := s.rec
	rec1 := s.rec
	rec1.ID = 11

	api := &mockLogForwardConfig{
		enabled: true,
		host:    "10.0.0.1",
	}
	lf, err := logforwarder.NewLogForwarder(s.newLogForwarderArgsWithAPI(c, api, s.stream, s.sender))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, lf)

	// Send the first record.
	s.stream.addRecords(c, rec0)
	s.sender.waitForSend(c)

	// Config change.
	api.host = "10.0.0.2"
	api.changes <- struct{}{}
	s.sender.waitForClose(c)

	// Send the second record.
	s.stream.addRecords(c, rec1)
	s.sender.waitForSend(c)

	workertest.CleanKill(c, lf)

	// Check that both records were sent with the config change
	// applied for the second send.
	rec1.Message = "send to 10.0.0.2"
	s.sender.stub.CheckCalls(c, []testing.StubCall{
		{"Send", []interface{}{[]logfwd.Record{rec0}}},
		{"Close", nil},
		{"Send", []interface{}{[]logfwd.Record{rec1}}},
		{"Close", nil},
	})
}

func (s *LogForwarderSuite) TestNotEnabled(c *gc.C) {
	lf, err := logforwarder.NewLogForwarder(s.newLogForwarderArgs(c, nil, s.sender))
	c.Assert(err, jc.ErrorIsNil)

	time.Sleep(coretesting.ShortWait)
	workertest.CleanKill(c, lf)

	// There should be no stream or sender activity when log
	// forwarding is disabled.
	s.stream.stub.CheckCallNames(c)
	s.sender.stub.CheckCallNames(c)
}

func (s *LogForwarderSuite) TestStreamError(c *gc.C) {
	failure := errors.New("<failure>")
	s.stream.stub.SetErrors(nil, failure)
	s.stream.addRecords(c, s.rec)

	lf, err := logforwarder.NewLogForwarder(s.newLogForwarderArgs(c, s.stream, s.sender))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, lf)

	err = workertest.CheckKilled(c, lf)
	c.Check(errors.Cause(err), gc.Equals, failure)

	s.sender.stub.CheckCalls(c, []testing.StubCall{
		{"Send", []interface{}{[]logfwd.Record{s.rec}}},
		{"Close", nil},
	})
}

func (s *LogForwarderSuite) TestSenderError(c *gc.C) {
	failure := errors.New("<failure>")
	s.sender.stub.SetErrors(nil, failure)

	rec0 := s.rec
	rec1 := s.rec
	rec1.ID = 11
	s.stream.addRecords(c, rec0, rec1)

	lf, err := logforwarder.NewLogForwarder(s.newLogForwarderArgs(c, s.stream, s.sender))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, lf)

	err = workertest.CheckKilled(c, lf)
	c.Check(errors.Cause(err), gc.Equals, failure)

	s.sender.stub.CheckCalls(c, []testing.StubCall{
		{"Send", []interface{}{[]logfwd.Record{rec0}}},
		{"Send", []interface{}{[]logfwd.Record{rec1}}},
		{"Close", nil},
	})
}

type mockLogForwardConfig struct {
	enabled bool
	host    string
	changes chan struct{}
}

type mockWatcher struct {
	watcher.NotifyWatcher
	changes chan struct{}
}

func (m *mockWatcher) Changes() watcher.NotifyChannel {
	return m.changes
}

func (*mockWatcher) Kill() {
}

func (*mockWatcher) Wait() error {
	return nil
}

type mockCaller struct {
	base.APICaller
}

func (*mockCaller) APICall(objType string, version int, id, request string, params, response interface{}) error {
	return nil
}

func (*mockCaller) BestFacadeVersion(facade string) int {
	return 0
}

func (c *mockLogForwardConfig) WatchForLogForwardConfigChanges() (watcher.NotifyWatcher, error) {
	c.changes = make(chan struct{}, 1)
	c.changes <- struct{}{}
	return &mockWatcher{
		changes: c.changes,
	}, nil
}

func (c *mockLogForwardConfig) LogForwardConfig() (*syslog.RawConfig, bool, error) {
	return &syslog.RawConfig{
		Enabled:    c.enabled,
		Host:       c.host,
		CACert:     coretesting.CACert,
		ClientCert: coretesting.ServerCert,
		ClientKey:  coretesting.ServerKey,
	}, true, nil
}

type stubStream struct {
	stub     *testing.Stub
	nextRecs chan logfwd.Record
}

func newStubStream() *stubStream {
	return &stubStream{
		stub:     new(testing.Stub),
		nextRecs: make(chan logfwd.Record, 16),
	}
}

func (s *stubStream) addRecords(c *gc.C, recs ...logfwd.Record) {
	for _, rec := range recs {
		s.nextRecs <- rec
	}
}

func (s *stubStream) Next() ([]logfwd.Record, error) {
	s.stub.AddCall("Next")
	if err := s.stub.NextErr(); err != nil {
		return []logfwd.Record{}, errors.Trace(err)
	}
	return []logfwd.Record{<-s.nextRecs}, nil
}

type stubSender struct {
	stub     *testing.Stub
	activity chan string
	host     string
}

func newStubSender() *stubSender {
	return &stubSender{
		stub:     new(testing.Stub),
		activity: make(chan string, 16),
	}
}

func (s *stubSender) Send(records []logfwd.Record) error {
	for i, rec := range records {
		rec.Message = "send to " + s.host
		records[i] = rec
	}
	s.stub.AddCall("Send", records)
	s.activity <- "Send"
	return errors.Trace(s.stub.NextErr())
}

func (s *stubSender) Close() error {
	s.stub.AddCall("Close")
	s.activity <- "Close"
	return errors.Trace(s.stub.NextErr())
}

func (s *stubSender) waitForSend(c *gc.C) {
	s.waitForActivity(c, "Send")
}

func (s *stubSender) waitForClose(c *gc.C) {
	s.waitForActivity(c, "Close")
}

func (s *stubSender) waitForActivity(c *gc.C, name string) {
	select {
	case a := <-s.activity:
		if a != name {
			c.Fatalf("expected %v, got %v", name, a)
		}
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timeout out waiting for %v", name)
	}
}
