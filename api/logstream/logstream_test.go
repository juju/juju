// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logstream_test

import (
	"context"
	"net/url"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/logstream"
	"github.com/juju/juju/internal/logfwd"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type LogReaderSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&LogReaderSuite{})

func (s *LogReaderSuite) TestOpenFullConfig(c *gc.C) {
	cUUID := "feebdaed-2f18-4fd2-967d-db9663db7bea"
	stub := &testing.Stub{}
	conn := &mockConnector{stub: stub}
	stream := mockStream{stub: stub}
	conn.ReturnConnectStream = stream
	cfg := params.LogStreamConfig{
		Sink: "spam",
	}

	_, err := logstream.Open(context.Background(), conn, cfg, cUUID)
	c.Assert(err, gc.IsNil)

	stub.CheckCallNames(c, "ConnectStream")
	stub.CheckCall(c, 0, "ConnectStream", `/logstream`, url.Values{
		"sink": []string{"spam"},
	})
}

func (s *LogReaderSuite) TestOpenError(c *gc.C) {
	cUUID := "feebdaed-2f18-4fd2-967d-db9663db7bea"
	stub := &testing.Stub{}
	conn := &mockConnector{stub: stub}
	failure := errors.New("foo")
	stub.SetErrors(failure)
	var cfg params.LogStreamConfig

	_, err := logstream.Open(context.Background(), conn, cfg, cUUID)

	c.Check(err, gc.ErrorMatches, "cannot connect to /logstream: foo")
	stub.CheckCallNames(c, "ConnectStream")
}

func (s *LogReaderSuite) TestNextOneRecord(c *gc.C) {
	ts := time.Now()
	apiRec := params.LogStreamRecord{
		ModelUUID: "deadbeef-2f18-4fd2-967d-db9663db7bea",
		Entity:    "machine-99",
		Version:   version.Current.String(),
		Timestamp: ts,
		Module:    "api.logstream.test",
		Location:  "test.go:42",
		Level:     loggo.INFO.String(),
		Message:   "test message",
	}
	apiRecords := params.LogStreamRecords{
		Records: []params.LogStreamRecord{apiRec},
	}
	cUUID := "feebdaed-2f18-4fd2-967d-db9663db7bea"
	stub := &testing.Stub{}
	conn := &mockConnector{stub: stub}
	jsonReader := mockStream{stub: stub}
	logsCh := make(chan params.LogStreamRecords, 1)
	logsCh <- apiRecords
	jsonReader.ReturnReadJSON = logsCh
	conn.ReturnConnectStream = jsonReader
	var cfg params.LogStreamConfig
	stream, err := logstream.Open(context.Background(), conn, cfg, cUUID)
	c.Assert(err, gc.IsNil)
	stub.ResetCalls()

	// Check the record we injected into the stream.
	var records []logfwd.Record
	done := make(chan struct{})
	go func() {
		records, err = stream.Next()
		c.Assert(err, jc.ErrorIsNil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for record")
	}
	c.Assert(records, gc.HasLen, 1)
	c.Check(records[0], jc.DeepEquals, logfwd.Record{
		Origin: logfwd.Origin{
			ControllerUUID: cUUID,
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
		Timestamp: ts,
		Level:     loggo.INFO,
		Location: logfwd.SourceLocation{
			Module:   "api.logstream.test",
			Filename: "test.go",
			Line:     42,
		},
		Message: "test message",
	})
	stub.CheckCallNames(c, "ReadJSON")

	// Make sure we don't get extras.
	done = make(chan struct{})
	go func() {
		records, err = stream.Next()
		c.Assert(err, jc.ErrorIsNil)
		close(done)
	}()
	select {
	case <-done:
		c.Errorf("got extra record: %#v", records)
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *LogReaderSuite) TestNextError(c *gc.C) {
	cUUID := "feebdaed-2f18-4fd2-967d-db9663db7bea"
	stub := &testing.Stub{}
	conn := &mockConnector{stub: stub}
	jsonReader := mockStream{stub: stub}
	conn.ReturnConnectStream = jsonReader
	failure := errors.New("an error")
	stub.SetErrors(nil, failure)
	var cfg params.LogStreamConfig
	stream, err := logstream.Open(context.Background(), conn, cfg, cUUID)
	c.Assert(err, gc.IsNil)

	var nextErr error
	done := make(chan struct{})
	go func() {
		_, nextErr = stream.Next()
		c.Check(errors.Cause(nextErr), gc.Equals, failure)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for record")
	}
	stub.CheckCallNames(c, "ConnectStream", "ReadJSON")
}

func (s *LogReaderSuite) TestClose(c *gc.C) {
	cUUID := "feebdaed-2f18-4fd2-967d-db9663db7bea"
	stub := &testing.Stub{}
	conn := &mockConnector{stub: stub}
	jsonReader := mockStream{stub: stub}
	conn.ReturnConnectStream = jsonReader
	var cfg params.LogStreamConfig
	stream, err := logstream.Open(context.Background(), conn, cfg, cUUID)
	c.Assert(err, gc.IsNil)
	stub.ResetCalls()

	err = stream.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = stream.Close() // idempotent
	c.Assert(err, jc.ErrorIsNil)

	_, err = stream.Next()
	c.Check(err, gc.ErrorMatches, `cannot read from closed stream`)
	stub.CheckCallNames(c, "Close")
}

type mockConnector struct {
	basetesting.APICallerFunc
	stub *testing.Stub

	ReturnConnectStream base.Stream
}

func (c *mockConnector) ConnectStream(_ context.Context, path string, values url.Values) (base.Stream, error) {
	c.stub.AddCall("ConnectStream", path, values)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.ReturnConnectStream, nil
}

type mockStream struct {
	base.Stream
	stub *testing.Stub

	ReturnReadJSON chan params.LogStreamRecords
}

func (s mockStream) ReadJSON(v interface{}) error {
	s.stub.AddCall("ReadJSON", v)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	switch vt := v.(type) {
	case *params.LogStreamRecords:
		*vt = <-s.ReturnReadJSON
		return nil
	default:
		return errors.Errorf("unexpected output type: %T", v)
	}
}

func (s mockStream) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
