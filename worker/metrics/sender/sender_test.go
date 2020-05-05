// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sender_test

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"path"
	"path/filepath"
	"runtime"
	"time"

	corecharm "github.com/juju/charm/v7"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/metrics/sender"
	"github.com/juju/juju/worker/metrics/spool"
)

var _ = gc.Suite(&senderSuite{})

type senderSuite struct {
	jujutesting.BaseSuite

	spoolDir      string
	socketDir     string
	metricfactory spool.MetricFactory
}

func (s *senderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.spoolDir = c.MkDir()
	s.socketDir = c.MkDir()

	s.metricfactory = &stubMetricFactory{
		&testing.Stub{},
		s.spoolDir,
	}

	declaredMetrics := map[string]corecharm.Metric{
		"pings": {Description: "test pings", Type: corecharm.MetricTypeAbsolute},
		"pongs": {Description: "test pongs", Type: corecharm.MetricTypeGauge},
	}
	recorder, err := s.metricfactory.Recorder(declaredMetrics, "local:trusty/testcharm", "testcharm/0")
	c.Assert(err, jc.ErrorIsNil)

	err = recorder.AddMetric("pings", "50", time.Now(), nil)
	c.Assert(err, jc.ErrorIsNil)

	err = recorder.AddMetric("pongs", "51", time.Now(), map[string]string{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	err = recorder.Close()
	c.Assert(err, jc.ErrorIsNil)

	reader, err := s.metricfactory.Reader()
	c.Assert(err, jc.ErrorIsNil)
	batches, err := reader.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 1)

	testing.PatchValue(sender.SocketName, func(_, _ string) string {
		return sockPath(c)
	})
}

func (s *senderSuite) TestHandler(c *gc.C) {
	apiSender := newTestAPIMetricSender()
	tmpDir := c.MkDir()
	metricFactory := &stubMetricFactory{
		&testing.Stub{},
		tmpDir,
	}

	declaredMetrics := map[string]corecharm.Metric{
		"pings": {Description: "test pings", Type: corecharm.MetricTypeAbsolute},
		"pongs": {Description: "test pongs", Type: corecharm.MetricTypeGauge},
	}
	recorder, err := metricFactory.Recorder(declaredMetrics, "local:trusty/testcharm", "testcharm/0")
	c.Assert(err, jc.ErrorIsNil)

	err = recorder.AddMetric("pings", "50", time.Now(), nil)
	c.Assert(err, jc.ErrorIsNil)

	err = recorder.AddMetric("pongs", "51", time.Now(), map[string]string{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)

	err = recorder.Close()
	c.Assert(err, jc.ErrorIsNil)

	metricSender, err := sender.NewSender(apiSender, s.metricfactory, s.socketDir, "")
	c.Assert(err, jc.ErrorIsNil)

	conn := &mockConnection{data: []byte(fmt.Sprintf("%v\n", tmpDir))}
	ch := make(chan struct{})
	err = metricSender.Handle(conn, ch)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 1)
	c.Assert(apiSender.batches[0].Tag, gc.Equals, "testcharm/0")
	c.Assert(apiSender.batches[0].Batch.CharmURL, gc.Equals, "local:trusty/testcharm")
	c.Assert(apiSender.batches[0].Batch.Metrics, gc.HasLen, 2)
	c.Assert(apiSender.batches[0].Batch.Metrics[0].Key, gc.Equals, "pings")
	c.Assert(apiSender.batches[0].Batch.Metrics[0].Value, gc.Equals, "50")
	c.Assert(apiSender.batches[0].Batch.Metrics[0].Labels, gc.HasLen, 0)
	c.Assert(apiSender.batches[0].Batch.Metrics[1].Key, gc.Equals, "pongs")
	c.Assert(apiSender.batches[0].Batch.Metrics[1].Value, gc.Equals, "51")
	c.Assert(apiSender.batches[0].Batch.Metrics[1].Labels, gc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *senderSuite) TestMetricSendingSuccess(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	metricSender, err := sender.NewSender(apiSender, s.metricfactory, s.socketDir, "test-unit-0")
	c.Assert(err, jc.ErrorIsNil)
	stopCh := make(chan struct{})
	err = metricSender.Do(stopCh)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 1)

	reader, err := spool.NewJSONMetricReader(s.spoolDir)
	c.Assert(err, jc.ErrorIsNil)
	batches, err := reader.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 0)
}

func (s *senderSuite) TestSendingGetDuplicate(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	apiErr := &params.Error{Message: "already exists", Code: params.CodeAlreadyExists}
	select {
	case apiSender.errors <- apiErr:
	default:
		c.Fatalf("blocked error channel")
	}

	metricSender, err := sender.NewSender(apiSender, s.metricfactory, s.socketDir, "test-unit-0")
	c.Assert(err, jc.ErrorIsNil)
	stopCh := make(chan struct{})
	err = metricSender.Do(stopCh)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 1)

	reader, err := spool.NewJSONMetricReader(s.spoolDir)
	c.Assert(err, jc.ErrorIsNil)
	batches, err := reader.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 0)
}

func (s *senderSuite) TestSendingFails(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	select {
	case apiSender.sendError <- errors.New("something went wrong"):
	default:
		c.Fatalf("blocked error channel")
	}

	metricSender, err := sender.NewSender(apiSender, s.metricfactory, s.socketDir, "test-unit-0")
	c.Assert(err, jc.ErrorIsNil)
	stopCh := make(chan struct{})
	err = metricSender.Do(stopCh)
	c.Assert(err, gc.ErrorMatches, "could not send metrics: something went wrong")

	c.Assert(apiSender.batches, gc.HasLen, 1)

	reader, err := spool.NewJSONMetricReader(s.spoolDir)
	c.Assert(err, jc.ErrorIsNil)
	batches, err := reader.Read()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batches, gc.HasLen, 1)
}

func (s *senderSuite) TestDataErrorIgnored(c *gc.C) {
	err := ioutil.WriteFile(filepath.Join(s.spoolDir, "foo.meta"), []byte{}, 0644)
	c.Assert(err, jc.ErrorIsNil)
	apiSender := newTestAPIMetricSender()

	metricSender, err := sender.NewSender(apiSender, s.metricfactory, s.socketDir, "test-unit-0")
	c.Assert(err, jc.ErrorIsNil)
	stopCh := make(chan struct{})
	err = metricSender.Do(stopCh)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiSender.batches, gc.HasLen, 0)
}

func (s *senderSuite) TestNoSpoolDirectory(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	metricfactory := &stubMetricFactory{
		&testing.Stub{},
		"/some/random/spool/dir",
	}

	metricSender, err := sender.NewSender(apiSender, metricfactory, s.socketDir, "")
	c.Assert(err, jc.ErrorIsNil)
	stopCh := make(chan struct{})
	err = metricSender.Do(stopCh)
	c.Assert(err, gc.ErrorMatches, `failed to open spool directory "/some/random/spool/dir": .*`)

	c.Assert(apiSender.batches, gc.HasLen, 0)
}

func (s *senderSuite) TestNoMetricsToSend(c *gc.C) {
	apiSender := newTestAPIMetricSender()

	newTmpSpoolDir := c.MkDir()
	metricfactory := &stubMetricFactory{
		&testing.Stub{},
		newTmpSpoolDir,
	}

	metricSender, err := sender.NewSender(apiSender, metricfactory, s.socketDir, "test-unit-0")
	c.Assert(err, jc.ErrorIsNil)
	stopCh := make(chan struct{})
	err = metricSender.Do(stopCh)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(apiSender.batches, gc.HasLen, 0)
}

func newTestAPIMetricSender() *testAPIMetricSender {
	return &testAPIMetricSender{errors: make(chan error, 1), sendError: make(chan error, 1)}
}

type testAPIMetricSender struct {
	batches   []params.MetricBatchParam
	errors    chan error
	sendError chan error
}

func (t *testAPIMetricSender) AddMetricBatches(batches []params.MetricBatchParam) (map[string]error, error) {
	t.batches = batches

	var err error
	select {
	case e := <-t.errors:
		err = e
	default:
		err = (*params.Error)(nil)
	}

	var sendErr error
	select {
	case e := <-t.sendError:
		sendErr = e
	default:
		sendErr = nil
	}

	errors := make(map[string]error)
	for _, b := range batches {
		errors[b.Batch.UUID] = err
	}
	return errors, sendErr
}

type stubMetricFactory struct {
	*testing.Stub
	spoolDir string
}

func (s *stubMetricFactory) Recorder(declaredMetrics map[string]corecharm.Metric, charmURL, unitTag string) (spool.MetricRecorder, error) {
	s.MethodCall(s, "Recorder", declaredMetrics, charmURL, unitTag)
	config := spool.MetricRecorderConfig{
		SpoolDir: s.spoolDir,
		Metrics:  declaredMetrics,
		CharmURL: charmURL,
		UnitTag:  unitTag,
	}

	return spool.NewJSONMetricRecorder(config)
}

func (s *stubMetricFactory) Reader() (spool.MetricReader, error) {
	s.MethodCall(s, "Reader")
	return spool.NewJSONMetricReader(s.spoolDir)

}

type mockConnection struct {
	net.Conn
	testing.Stub
	data []byte
}

// SetDeadline implements the net.Conn interface.
func (c *mockConnection) SetDeadline(t time.Time) error {
	c.AddCall("SetDeadline", t)
	return nil
}

// Write implements the net.Conn interface.
func (c *mockConnection) Write(data []byte) (int, error) {
	c.AddCall("Write", data)
	c.data = data
	return len(data), nil
}

// Close implements the net.Conn interface.
func (c *mockConnection) Close() error {
	c.AddCall("Close")
	return nil
}

func (c *mockConnection) eof() bool {
	return len(c.data) == 0
}

func (c *mockConnection) readByte() byte {
	b := c.data[0]
	c.data = c.data[1:]
	return b
}

func (c *mockConnection) Read(p []byte) (n int, err error) {
	if c.eof() {
		err = io.EOF
		return
	}
	if cp := cap(p); cp > 0 {
		for n < cp {
			p[n] = c.readByte()
			n++
			if c.eof() {
				break
			}
		}
	}
	return
}

func sockPath(c *gc.C) string {
	sockPath := path.Join(c.MkDir(), "test.listener")
	if runtime.GOOS == "windows" {
		return `\\.\pipe` + sockPath[2:]
	}
	return sockPath
}
