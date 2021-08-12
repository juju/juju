// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package raftleaseconsumer

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/websocket/websockettest"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type HandlerSuite struct {
	testing.IsolationSuite

	logger *MockLogger
	clock  *MockClock
}

var _ = gc.Suite(&HandlerSuite{})

func (s *HandlerSuite) TestHandlerStartsAndCloses(c *gc.C) {
	defer s.setupMocks(c).Finish()

	operations := make(chan operation)

	handler := NewHandler(operations, nil, s.clock, s.logger)

	url, shutdown := s.setupServer(c, handler)
	defer shutdown()

	conn := s.dial(c, url)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	// Close connection.
	err := conn.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HandlerSuite) TestHandlerSendsMessage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	operations := make(chan operation)

	done := make(chan struct{})
	defer close(done)

	handler := NewHandler(operations, nil, s.clock, s.logger)

	go func() {
		for {
			select {
			case <-done:
				return
			case op := <-operations:
				op.Callback(nil)
				done <- struct{}{}
			}
		}
	}()

	url, shutdown := s.setupServer(c, handler)
	defer shutdown()

	conn := s.dial(c, url)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	writeOp := params.LeaseOperation{
		UUID:    "aaaabbbbcccc",
		Command: "claim",
	}
	err := conn.WriteJSON(&writeOp)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no first message")
	}

	var readOp params.LeaseOperationResult
	err = conn.ReadJSON(&readOp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readOp, gc.DeepEquals, params.LeaseOperationResult{
		UUID: "aaaabbbbcccc",
	})

	// Close connection.
	err = conn.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HandlerSuite) TestHandlerSendsMessagesOutOfOrder(c *gc.C) {
	defer s.setupMocks(c).Finish()

	operations := make(chan operation)

	done := make(chan struct{})
	defer close(done)

	handler := NewHandler(operations, nil, s.clock, s.logger)

	go func() {
		var stashed *operation
		for {
			select {
			case <-done:
				return
			case op := <-operations:
				if op.Commands[0] == "claim" {
					stashed = &op
					continue
				}

				op.Callback(nil)
				done <- struct{}{}

				stashed.Callback(nil)
				done <- struct{}{}
			}
		}
	}()

	url, shutdown := s.setupServer(c, handler)
	defer shutdown()

	conn := s.dial(c, url)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	// Allow out of order read/write.
	writeOp1 := params.LeaseOperation{
		UUID:    "aaaabbbbcccc",
		Command: "claim",
	}
	err := conn.WriteJSON(&writeOp1)
	c.Assert(err, jc.ErrorIsNil)

	writeOp2 := params.LeaseOperation{
		UUID:    "xxxxyyyyzzzz",
		Command: "extend",
	}
	err = conn.WriteJSON(&writeOp2)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no first message")
	}

	var readOp params.LeaseOperationResult
	err = conn.ReadJSON(&readOp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readOp, gc.DeepEquals, params.LeaseOperationResult{
		UUID: "xxxxyyyyzzzz",
	})

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no second message")
	}

	err = conn.ReadJSON(&readOp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readOp, gc.DeepEquals, params.LeaseOperationResult{
		UUID: "aaaabbbbcccc",
	})

	// Close connection.
	err = conn.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HandlerSuite) TestHandlerSendsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	operations := make(chan operation)

	done := make(chan struct{})
	defer close(done)

	handler := NewHandler(operations, nil, s.clock, s.logger)

	go func() {
		for {
			select {
			case <-done:
				return
			case op := <-operations:
				op.Callback(errors.New("boom"))
				done <- struct{}{}
			}
		}
	}()

	url, shutdown := s.setupServer(c, handler)
	defer shutdown()

	conn := s.dial(c, url)
	defer conn.Close()

	// Read back the nil error, indicating that all is well.
	websockettest.AssertJSONInitialErrorNil(c, conn)

	writeOp := params.LeaseOperation{
		UUID:    "aaaabbbbcccc",
		Command: "claim",
	}
	err := conn.WriteJSON(&writeOp)
	c.Assert(err, jc.ErrorIsNil)

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no first message")
	}

	var readOp params.LeaseOperationResult
	err = conn.ReadJSON(&readOp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readOp, gc.DeepEquals, params.LeaseOperationResult{
		UUID: "aaaabbbbcccc",
		Error: &params.Error{
			Message: "boom",
			Code:    params.CodeBadRequest,
		},
	})

	// Close connection.
	err = conn.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HandlerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = NewMockLogger(ctrl)
	s.clock = NewMockClock(ctrl)

	s.logger.EXPECT().Tracef(gomock.Any(), gomock.Any()).AnyTimes()
	s.logger.EXPECT().Debugf(gomock.Any(), gomock.Any()).AnyTimes()
	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()

	return ctrl
}

func (s *HandlerSuite) setupServer(c *gc.C, handler http.Handler) (*url.URL, func()) {
	server := httptest.NewServer(handler)

	return &url.URL{
		Scheme: "ws",
		Host:   server.Listener.Addr().String(),
		Path:   "",
	}, server.Close
}

func (s *HandlerSuite) dial(c *gc.C, url *url.URL) *websocket.Conn {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(url.String(), http.Header{})
	c.Assert(err, jc.ErrorIsNil)

	return conn
}
