// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logreader_test

import (
	"net/url"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/logreader"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type logReaderSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&logReaderSuite{})

func (s *logReaderSuite) TestLoggingConfigError(c *gc.C) {
	tag := names.NewMachineTag("42")
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			c.Assert(objType, gc.Equals, "RsyslogConfig")
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(args, gc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: tag.String()}},
			})
			called = true
			switch request {
			case "RsyslogConfig":
				return errors.New("permission denied")
			default:
				c.Fatalf("unknown request: %v", request)
			}
			return nil
		})
	api := logreader.NewAPI(apiCaller)
	c.Assert(api, gc.NotNil)

	config, err := api.RsyslogConfig(tag)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(config, gc.IsNil)
	c.Assert(called, jc.IsTrue)
	called = false
}

func (s *logReaderSuite) TestLoggingConfig(c *gc.C) {
	tag := names.NewMachineTag("0")
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			c.Assert(objType, gc.Equals, "RsyslogConfig")
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(args, gc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: tag.String()}},
			})
			called = true
			switch request {
			case "RsyslogConfig":
				result, ok := response.(*params.RsyslogConfigResults)
				c.Assert(ok, jc.IsTrue)
				result.Results = []params.RsyslogConfigResult{{
					URL:        "localhost:1234",
					CACert:     coretesting.CACert,
					ClientCert: coretesting.OtherCACert,
					ClientKey:  coretesting.OtherCAKey,
				}}
			default:
				c.Fatalf("unknown request: %v", request)
			}
			return nil
		})
	api := logreader.NewAPI(apiCaller)
	c.Assert(api, gc.NotNil)

	config, err := api.RsyslogConfig(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(config.URL, gc.Equals, "localhost:1234")
	c.Assert(config.CACert, gc.Equals, coretesting.CACert)
	c.Assert(config.ClientCert, gc.Equals, coretesting.OtherCACert)
	c.Assert(config.ClientKey, gc.Equals, coretesting.OtherCAKey)
	c.Assert(called, jc.IsTrue)
	called = false
}

func (s *logReaderSuite) TestWatchRsyslogConfigMoreResults(c *gc.C) {
	tag := names.NewMachineTag("0")
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			c.Check(objType, gc.Equals, "RsyslogConfig")
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "WatchRsyslogConfig")
			c.Check(args, gc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: tag.String()}},
			})
			c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
			result := response.(*params.NotifyWatchResults)
			result.Results = make([]params.NotifyWatchResult, 2)
			called = true
			return nil
		})
	api := logreader.NewAPI(apiCaller)
	c.Assert(api, gc.NotNil)
	w, err := api.WatchRsyslogConfig(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Assert(w, gc.IsNil)
}

func (s *logReaderSuite) TestWatchRsyslogConfigResultError(c *gc.C) {
	tag := names.NewMachineTag("0")
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			c.Check(objType, gc.Equals, "RsyslogConfig")
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "WatchRsyslogConfig")
			c.Check(args, gc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: tag.String()}},
			})
			c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
			result := response.(*params.NotifyWatchResults)
			result.Results = []params.NotifyWatchResult{{
				Error: &params.Error{
					Message: "well, this is embarrassing",
					Code:    params.CodeNotAssigned,
				},
			}}
			called = true
			return nil
		})
	api := logreader.NewAPI(apiCaller)
	c.Assert(api, gc.NotNil)
	w, err := api.WatchRsyslogConfig(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "well, this is embarrassing")
	c.Assert(w, gc.IsNil)
}

func (s *logReaderSuite) TestWatchRsyslogConfig(c *gc.C) {
	tag := names.NewMachineTag("0")
	var called bool
	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			c.Check(objType, gc.Equals, "RsyslogConfig")
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "WatchRsyslogConfig")
			c.Check(args, gc.DeepEquals, params.Entities{
				Entities: []params.Entity{{Tag: tag.String()}},
			})
			c.Assert(response, gc.FitsTypeOf, &params.NotifyWatchResults{})
			result := response.(*params.NotifyWatchResults)
			result.Results = []params.NotifyWatchResult{{
				Error: &params.Error{
					Message: "well, this is embarrassing",
					Code:    params.CodeNotAssigned,
				},
			}}
			called = true
			return nil
		})
	api := logreader.NewAPI(apiCaller)
	c.Assert(api, gc.NotNil)
	w, err := api.WatchRsyslogConfig(tag)
	c.Assert(called, jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "well, this is embarrassing")
	c.Assert(w, gc.IsNil)
}

func (s *logReaderSuite) TestNewAPI(c *gc.C) {
	conn := &mockConnector{}
	a := logreader.NewAPI(conn)
	r, err := a.LogReader()
	c.Assert(err, gc.IsNil)

	channel := r.ReadLogs()
	c.Assert(channel, gc.NotNil)

	select {
	case logRecord := <-channel:
		c.Assert(logRecord.Error, gc.IsNil)
		c.Assert(logRecord.Message, gc.Equals, "test message")
		c.Assert(logRecord.Level, gc.Equals, loggo.INFO)
		c.Assert(logRecord.Module, gc.Equals, "api.logreader.test")
	case <-time.After(coretesting.LongWait):
		c.Fail()
	}

	err = r.Close()
	c.Assert(err, gc.IsNil)
	c.Assert(conn.closeCount, gc.Equals, 1)
}

func (s *logReaderSuite) TestNewAPIReadLogError(c *gc.C) {
	conn := &mockConnector{
		connectError: errors.New("foo"),
	}
	a := logreader.NewAPI(conn)
	r, err := a.LogReader()
	c.Assert(err, gc.ErrorMatches, "cannot connect to /log: foo")
	c.Assert(r, gc.Equals, nil)
}

func (s *logReaderSuite) TestNewAPIWriteError(c *gc.C) {
	conn := &mockConnector{
		readError: errors.New("foo"),
	}
	a := logreader.NewAPI(conn)
	r, err := a.LogReader()
	c.Assert(err, gc.IsNil)

	channel := r.ReadLogs()
	c.Assert(channel, gc.NotNil)

	select {
	case logRecord := <-channel:
		c.Assert(logRecord.Error, gc.DeepEquals, &params.Error{Message: "failed to read JSON: foo"})
	case <-time.After(coretesting.LongWait):
		c.Fail()
	}
}

type mockConnector struct {
	basetesting.APICallerFunc
	connectError error
	readError    error
	closeCount   int
}

func (c *mockConnector) ConnectStream(path string, values url.Values) (base.Stream, error) {
	if path != "/log" {
		return nil, errors.New("unexpected path: " + path)
	}
	if len(values) != 3 {
		return nil, errors.New("unexpected values")
	}
	if values.Get("format") != "json" {
		return nil, errors.New("must request JSON format")
	}
	if values.Get("all") != "true" {
		return nil, errors.New("must all logs")
	}
	if values.Get("backlog") != "10" {
		return nil, errors.New("backlog not equal to 10")
	}
	if c.connectError != nil {
		return nil, c.connectError
	}
	return mockStream{conn: c}, nil
}

type mockStream struct {
	base.Stream
	conn *mockConnector
}

func (s mockStream) ReadJSON(v interface{}) error {
	if s.conn.readError != nil {
		return s.conn.readError
	}
	record := params.LogRecord{Message: "test message", Level: loggo.INFO, Location: "test.go", Module: "api.logreader.test"}
	switch v.(type) {
	case *params.LogRecord:
		vt := v.(*params.LogRecord)
		*vt = record
		return nil
	default:
		return errors.Errorf("unexpected output type: %T", v)
	}
}

func (s mockStream) Close() error {
	s.conn.closeCount++
	return nil
}
