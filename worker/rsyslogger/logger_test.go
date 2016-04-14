// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslogger_test

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"time"

	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/syslog"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/httprequest"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logreader"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/rsyslogger"
	"github.com/juju/juju/worker/workertest"
)

type RsysloggerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&RsysloggerSuite{})

func (s *RsysloggerSuite) TestStartMissingAgent(c *gc.C) {
	stubResource := dt.NewStubResources(map[string]interface{}{
		"agent":      dependency.ErrMissing,
		"api-caller": &mockAPICaller{},
	})
	manifold := rsyslogger.Manifold(rsyslogger.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
	w, err := manifold.Start(stubResource.Context())
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, dependency.ErrMissing.Error())
}

func (s *RsysloggerSuite) TestStartMissingAPICaller(c *gc.C) {
	stubResource := dt.NewStubResources(map[string]interface{}{
		"agent":      &mockAgent{tag: names.NewMachineTag("0")},
		"api-caller": dependency.ErrMissing,
	})
	manifold := rsyslogger.Manifold(rsyslogger.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
	w, err := manifold.Start(stubResource.Context())
	c.Assert(w, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, dependency.ErrMissing.Error())
}

var (
	fixedTime = time.Date(2016, time.March, 22, 8, 12, 0, 0, time.UTC)

	testLogs = []params.LogRecordResult{{
		LogRecord: params.LogRecord{
			ModelUUID: "test-uuid",
			Time:      fixedTime,
			Module:    "test-module",
			Location:  "test-location",
			Level:     loggo.CRITICAL,
			Message:   "hello critical",
		},
	}, {
		LogRecord: params.LogRecord{
			ModelUUID: "test-uuid",
			Time:      fixedTime,
			Module:    "test-module",
			Location:  "test-location",
			Level:     loggo.ERROR,
			Message:   "hello error",
		},
	}, {
		LogRecord: params.LogRecord{
			ModelUUID: "test-uuid",
			Time:      fixedTime,
			Module:    "test-module",
			Location:  "test-location",
			Level:     loggo.WARNING,
			Message:   "hello warning",
		},
	}, {
		LogRecord: params.LogRecord{
			ModelUUID: "test-uuid",
			Time:      fixedTime,
			Module:    "test-module",
			Location:  "test-location",
			Level:     loggo.INFO,
			Message:   "hello info",
		},
	}, {
		LogRecord: params.LogRecord{
			ModelUUID: "test-uuid",
			Time:      fixedTime,
			Module:    "test-module",
			Location:  "test-location",
			Level:     loggo.DEBUG,
			Message:   "hello debug",
		},
	}, {
		LogRecord: params.LogRecord{
			ModelUUID: "test-uuid",
			Time:      fixedTime,
			Module:    "test-module",
			Location:  "test-location",
			Level:     loggo.TRACE,
			Message:   "hello trace",
		},
	},
	}
)

func assertSendLogs(c *gc.C, logs chan params.LogRecordResult) {
	for i, logRecord := range testLogs {
		select {
		case logs <- logRecord:
			c.Logf("sent log record %v", i)
		case <-time.After(coretesting.ShortWait):
			c.Fatalf("time out sending log record %v", i)
		}
	}
}

func assertFailToSendLogs(c *gc.C, logs chan params.LogRecordResult) {
	for i, logRecord := range testLogs {
		select {
		case logs <- logRecord:
			c.Fatalf("managed to send log record %v", i)
		case <-time.After(coretesting.ShortWait):
		}
	}
}

func assertConfigRead(c *gc.C, changes chan struct{}) {
	select {
	case <-changes:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("config values not read")
	}
}

func assertConfigNotRead(c *gc.C, changes chan struct{}) {
	select {
	case <-changes:
		c.Fatalf("config values read")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *RsysloggerSuite) TestStartManifold(c *gc.C) {
	// create a syslog writer.
	writer := newMockSyslogger()
	c.Assert(writer, gc.NotNil)
	// patch the DialSyslog function.
	cleanup := jtesting.PatchValue(
		rsyslogger.DialSyslog,
		func(network, raddr string,
			priority syslog.Priority,
			tag string,
			tlsCfg *tls.Config,
		) (rsyslogger.SysLogger, error) {
			return writer, nil
		})
	defer cleanup()

	// create a mock facade.
	facade := &mockFacade{
		Stub:       &jtesting.Stub{},
		changes:    make(chan struct{}, 1),
		logs:       make(chan params.LogRecordResult),
		configRead: make(chan struct{}, 2),
	}

	// create and run a new worker.
	w, err := rsyslogger.NewRsysWorker(facade, &mockConfig{tag: names.NewMachineTag("0")})
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Logf("stopping worker")
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()
	assertConfigNotRead(c, facade.configRead)

	// assert that the worker does not read logs - rsyslog
	// forwarding is not yet configured.
	assertFailToSendLogs(c, facade.logs)
	c.Assert(writer.Calls(), gc.HasLen, 0)

	// set rsyslog config and trigger the watcher.
	facade.url = "https://localhost:1234"
	facade.caCert = coretesting.CACert
	facade.clientCert = coretesting.OtherCACert
	facade.clientKey = coretesting.OtherCAKey
	facade.changes <- struct{}{}
	assertConfigRead(c, facade.configRead)

	// assert that the logs are being read.
	assertSendLogs(c, facade.logs)

	// check that the logs are being sent to syslog.
	// we expect only 5 log records since loggo.TRACE
	// is not forwarded to syslog.
	for i := 0; i < len(testLogs)-1; i++ {
		select {
		case <-writer.changes:
			c.Logf("received writer notification")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out %v", i)
		}
	}
	c.Assert(writer.Calls(), gc.HasLen, 5)
	writer.CheckCall(c, 0, "Crit", fmt.Sprintf("test-uuid: %v CRITICAL test-module test-location hello critical\n", fixedTime.Format(time.RFC3339)))
	writer.CheckCall(c, 1, "Err", fmt.Sprintf("test-uuid: %v ERROR test-module test-location hello error\n", fixedTime.Format(time.RFC3339)))
	writer.CheckCall(c, 2, "Warning", fmt.Sprintf("test-uuid: %v WARNING test-module test-location hello warning\n", fixedTime.Format(time.RFC3339)))
	writer.CheckCall(c, 3, "Info", fmt.Sprintf("test-uuid: %v INFO test-module test-location hello info\n", fixedTime.Format(time.RFC3339)))
	writer.CheckCall(c, 4, "Debug", fmt.Sprintf("test-uuid: %v DEBUG test-module test-location hello debug\n", fixedTime.Format(time.RFC3339)))

	// reset syslog writer calls.
	writer.ResetCalls()

	// set only the CA cert config.
	facade.url = ""
	facade.caCert = coretesting.CACert
	facade.clientCert = ""
	facade.clientKey = ""
	facade.changes <- struct{}{}
	assertConfigRead(c, facade.configRead)

	// assert that logs are not being read.
	assertFailToSendLogs(c, facade.logs)
	c.Assert(writer.Calls(), gc.HasLen, 0)

	// set only the syslog url.
	facade.url = "https://localhost:12345"
	facade.caCert = ""
	facade.clientCert = ""
	facade.clientKey = ""
	facade.changes <- struct{}{}
	assertConfigRead(c, facade.configRead)

	// assert that logs are not being read.
	assertFailToSendLogs(c, facade.logs)
	c.Assert(writer.Calls(), gc.HasLen, 0)

	// set wrong client cert/key.
	facade.url = "https://localhost:12345"
	facade.caCert = coretesting.CACert
	facade.clientCert = coretesting.CACert
	facade.clientKey = coretesting.OtherCAKey
	facade.changes <- struct{}{}
	assertConfigRead(c, facade.configRead)

	// assert that logs are not being read.
	assertFailToSendLogs(c, facade.logs)
	c.Assert(writer.Calls(), gc.HasLen, 0)

	// set rsyslog config and trigger the watcher again to
	// make sure the read loop is restarted.
	facade.url = "https://localhost:1234"
	facade.caCert = coretesting.CACert
	facade.clientCert = coretesting.OtherCACert
	facade.clientKey = coretesting.OtherCAKey
	facade.changes <- struct{}{}
	assertConfigRead(c, facade.configRead)

	// assert that the logs are being read.
	assertSendLogs(c, facade.logs)

	// check that the logs are being sent to syslog.
	// we expect only 5 log records since loggo.TRACE
	// is not forwarded to syslog.
	for i := 0; i < len(testLogs)-1; i++ {
		select {
		case <-writer.changes:
			c.Logf("received writer notification")
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out %v", i)
		}
	}
	c.Assert(writer.Calls(), gc.HasLen, 5)
	writer.CheckCall(c, 0, "Crit", fmt.Sprintf("test-uuid: %v CRITICAL test-module test-location hello critical\n", fixedTime.Format(time.RFC3339)))
	writer.CheckCall(c, 1, "Err", fmt.Sprintf("test-uuid: %v ERROR test-module test-location hello error\n", fixedTime.Format(time.RFC3339)))
	writer.CheckCall(c, 2, "Warning", fmt.Sprintf("test-uuid: %v WARNING test-module test-location hello warning\n", fixedTime.Format(time.RFC3339)))
	writer.CheckCall(c, 3, "Info", fmt.Sprintf("test-uuid: %v INFO test-module test-location hello info\n", fixedTime.Format(time.RFC3339)))
	writer.CheckCall(c, 4, "Debug", fmt.Sprintf("test-uuid: %v DEBUG test-module test-location hello debug\n", fixedTime.Format(time.RFC3339)))
}

type mockAgent struct {
	agent.Agent
	tag names.Tag
}

func (mock *mockAgent) CurrentConfig() agent.Config {
	return &mockConfig{tag: mock.tag}
}

type mockConfig struct {
	agent.Config
	tag names.Tag
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func newMockSyslogger() *mockSyslogger {
	return &mockSyslogger{Stub: &jtesting.Stub{}, changes: make(chan struct{}, len(testLogs))}
}

type mockSyslogger struct {
	*jtesting.Stub

	changes chan struct{}
}

func (m *mockSyslogger) notifyChanges() {
	select {
	case m.changes <- struct{}{}:
	default:
	}
}

func (m *mockSyslogger) Crit(msg string) error {
	m.AddCall("Crit", msg)
	m.notifyChanges()
	return m.NextErr()
}

func (m *mockSyslogger) Err(msg string) error {
	m.AddCall("Err", msg)
	m.notifyChanges()
	return m.NextErr()
}

func (m *mockSyslogger) Warning(msg string) error {
	m.AddCall("Warning", msg)
	m.notifyChanges()
	return m.NextErr()
}

func (m *mockSyslogger) Notice(msg string) error {
	m.AddCall("Notice", msg)
	m.notifyChanges()
	return m.NextErr()
}

func (m *mockSyslogger) Info(msg string) error {
	m.AddCall("Info", msg)
	m.notifyChanges()
	return m.NextErr()
}

func (m *mockSyslogger) Debug(msg string) error {
	m.AddCall("Debug", msg)
	m.notifyChanges()
	return m.NextErr()
}

func (m *mockSyslogger) Write(msg []byte) (int, error) {
	m.AddCall("Write", msg)
	m.notifyChanges()
	return len(msg), m.NextErr()
}

type mockFacade struct {
	*jtesting.Stub

	url        string
	caCert     string
	clientCert string
	clientKey  string
	changes    chan struct{}
	configRead chan struct{}
	logs       chan params.LogRecordResult
}

func (m *mockFacade) WatchRsyslogConfig(tag names.Tag) (watcher.NotifyWatcher, error) {
	m.AddCall("WatchRsyslogConfig", tag)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return newMockWatcher(m.changes), nil
}

func (m *mockFacade) RsyslogConfig(tag names.Tag) (*logreader.RsyslogConfig, error) {
	m.AddCall("RsyslogURLConfig", tag)
	m.notifyConfigRead()
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return &logreader.RsyslogConfig{
		URL:        m.url,
		CACert:     m.caCert,
		ClientCert: m.clientCert,
		ClientKey:  m.clientKey,
	}, nil
}

func (m *mockFacade) notifyConfigRead() {
	select {
	case m.configRead <- struct{}{}:
	default:
	}
}

func (m *mockFacade) LogReader() (logreader.LogReader, error) {
	m.AddCall("LogReader")
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return &mockLogReader{logs: m.logs}, nil
}

func newMockWatcher(changes chan struct{}) *mockWatcher {
	return &mockWatcher{
		Worker:  workertest.NewErrorWorker(nil),
		changes: changes,
	}
}

type mockWatcher struct {
	worker.Worker
	changes chan struct{}
}

func (m *mockWatcher) Changes() watcher.NotifyChannel {
	return m.changes
}

type mockLogReader struct {
	logs chan params.LogRecordResult
}

func (m *mockLogReader) ReadLogs() chan params.LogRecordResult {
	return m.logs
}

func (m *mockLogReader) Close() error {
	return nil
}

var _ base.APICaller = (*mockAPICaller)(nil)

type mockAPICaller struct {
	*jtesting.Stub
}

func (s *mockAPICaller) APICall(objType string, version int, id, request string, params, response interface{}) error {
	s.MethodCall(s, "APICall", objType, version, id, request, params, response)
	return nil
}

func (s *mockAPICaller) BestFacadeVersion(facade string) int {
	s.MethodCall(s, "BestFacadeVersion", facade)
	return 42
}

func (s *mockAPICaller) ModelTag() (names.ModelTag, error) {
	s.MethodCall(s, "ModelTag")
	return names.NewModelTag("foobar"), nil
}

func (s *mockAPICaller) ConnectStream(string, url.Values) (base.Stream, error) {
	panic("should not be called")
}

func (s *mockAPICaller) HTTPClient() (*httprequest.Client, error) {
	panic("should not be called")
}
