// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spool_test

import (
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"runtime"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/metrics/spool"
)

var _ = gc.Suite(&listenerSuite{})

type stopper interface {
	Stop()
}

type listenerSuite struct {
	socketPath string
	handler    *mockHandler
	listener   stopper
}

func sockPath(c *gc.C) string {
	sockPath := filepath.Join(c.MkDir(), "test.listener")
	if runtime.GOOS == "windows" {
		return `\\.\pipe` + sockPath[2:]
	}
	return sockPath
}

func (s *listenerSuite) SetUpTest(c *gc.C) {
	s.handler = &mockHandler{}
	s.socketPath = sockPath(c)
	listener, err := spool.NewSocketListener(s.socketPath, s.handler)
	c.Assert(err, jc.ErrorIsNil)
	s.listener = listener
}

func (s *listenerSuite) TearDownTest(c *gc.C) {
	s.listener.Stop()
}

func (s *listenerSuite) TestDial(c *gc.C) {
	readCloser, err := dial(s.socketPath)
	c.Assert(err, jc.ErrorIsNil)
	defer readCloser.Close()

	data, err := ioutil.ReadAll(readCloser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "Hello socket.")
	s.handler.CheckCall(c, 0, "Handle")
}

type mockHandler struct {
	testing.Stub
}

// Handle implements the spool.ConnectionHandler interface.
func (h *mockHandler) Handle(c net.Conn) error {
	defer c.Close()
	h.AddCall("Handle")
	fmt.Fprintf(c, "Hello socket.")
	return nil
}
