// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logreader_test

import (
	"net/url"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/logreader"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/logfwd"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type LogRecordReaderSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&LogRecordReaderSuite{})

func (s *LogRecordReaderSuite) TestLogRecordReader(c *gc.C) {
	cUUID := "feebdaed-2f18-4fd2-967d-db9663db7bea"
	ts := time.Now()
	apiRec := params.LogStreamRecord{
		ModelUUID: "deadbeef-2f18-4fd2-967d-db9663db7bea",
		Timestamp: ts,
		Module:    "api.logreader.test",
		Location:  "test.go:42",
		Level:     loggo.INFO.String(),
		Message:   "test message",
	}
	stub := &testing.Stub{}
	stream := mockStream{stub: stub}
	stream.ReturnReadJSON = apiRec
	conn := &mockConnector{stub: stub}
	conn.ReturnConnectStream = stream
	var cfg params.LogStreamConfig
	r, err := logreader.OpenLogRecordReader(conn, cfg, cUUID)
	c.Assert(err, gc.IsNil)

	channel := r.Channel()
	c.Assert(channel, gc.NotNil)

	stub.CheckCall(c, 0, "ConnectStream", `/logstream`, url.Values{})
	select {
	case logRecord := <-channel:
		c.Check(logRecord, jc.DeepEquals, logfwd.Record{
			Origin: logfwd.Origin{
				ControllerUUID: cUUID,
				ModelUUID:      "deadbeef-2f18-4fd2-967d-db9663db7bea",
				JujuVersion:    version.Current,
			},
			Timestamp: ts,
			Level:     loggo.INFO,
			Location: logfwd.SourceLocation{
				Module:   "api.logreader.test",
				Filename: "test.go",
				Line:     42,
			},
			Message: "test message",
		})
	case <-time.After(coretesting.LongWait):
		c.Errorf("timed out waiting for kill")
	}

	r.Kill()
	c.Assert(r.Wait(), jc.ErrorIsNil)

	stub.CheckCallNames(c, "ConnectStream", "ReadJSON", "ReadJSON", "Close")
}

func (s *LogRecordReaderSuite) TestNewAPIReadLogError(c *gc.C) {
	cUUID := "feebdaed-2f18-4fd2-967d-db9663db7bea"
	stub := &testing.Stub{}
	conn := &mockConnector{stub: stub}
	failure := errors.New("foo")
	stub.SetErrors(failure)
	var cfg params.LogStreamConfig

	_, err := logreader.OpenLogRecordReader(conn, cfg, cUUID)

	stub.CheckCallNames(c, "ConnectStream")
	c.Check(err, gc.ErrorMatches, "cannot connect to /logstream: foo")
}

func (s *LogRecordReaderSuite) TestNewAPIWriteError(c *gc.C) {
	cUUID := "feebdaed-2f18-4fd2-967d-db9663db7bea"
	stub := &testing.Stub{}
	stream := mockStream{stub: stub}
	conn := &mockConnector{stub: stub}
	conn.ReturnConnectStream = stream
	failure := errors.New("an error")
	stub.SetErrors(nil, failure)
	var cfg params.LogStreamConfig
	r, err := logreader.OpenLogRecordReader(conn, cfg, cUUID)
	c.Assert(err, gc.IsNil)

	channel := r.Channel()
	c.Assert(channel, gc.NotNil)

	select {
	case <-channel:
		c.Assert(r.Wait(), gc.ErrorMatches, "an error")
	case <-time.After(coretesting.LongWait):
		c.Fail()
	}
	stub.CheckCallNames(c, "ConnectStream", "ReadJSON", "Close")
}

type mockConnector struct {
	basetesting.APICallerFunc
	stub *testing.Stub

	ReturnConnectStream base.Stream
}

func (c *mockConnector) ConnectStream(path string, values url.Values) (base.Stream, error) {
	c.stub.AddCall("ConnectStream", path, values)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return c.ReturnConnectStream, nil
}

type mockStream struct {
	base.Stream
	stub *testing.Stub

	ReturnReadJSON params.LogStreamRecord
}

func (s mockStream) ReadJSON(v interface{}) error {
	s.stub.AddCall("ReadJSON", v)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	switch vt := v.(type) {
	case *params.LogStreamRecord:
		*vt = s.ReturnReadJSON
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
