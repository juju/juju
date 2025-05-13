// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package socketlistener_test

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/core/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/socketlistener"
	"github.com/juju/juju/juju/sockets"
)

type socketListenerSuite struct {
	logger logger.Logger
}

var _ = tc.Suite(&socketListenerSuite{})

func (s *socketListenerSuite) SetUpTest(c *tc.C) {
	s.logger = loggertesting.WrapCheckLog(c)
}

func handleTestEndpoint1(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusOK)
}

func registerTestHandlers(r *mux.Router) {
	r.HandleFunc("/test-endpoint", handleTestEndpoint1).
		Methods(http.MethodGet)
}

func (s *socketListenerSuite) TestStartStopWorker(c *tc.C) {
	tmpDir := c.MkDir()
	socket := path.Join(tmpDir, "test.socket")

	sl, err := socketlistener.NewSocketListener(socketlistener.Config{
		Logger:           s.logger,
		SocketName:       socket,
		RegisterHandlers: registerTestHandlers,
		ShutdownTimeout:  coretesting.LongWait,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Check socket is created with correct permissions.
	fi, err := os.Stat(socket)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fi.Mode(), tc.Equals, fs.ModeSocket|0700)

	// Check server is up.
	cl := client(socket)
	resp, err := cl.Get("http://localhost:8080/foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)

	// Check server is serving.
	cl = client(socket)
	resp, err = cl.Get("http://localhost:8080/test-endpoint")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)

	sl.Kill()
	err = sl.Wait()
	c.Assert(err, tc.ErrorIsNil)

	// Check server has stopped.
	_, err = cl.Get("http://localhost:8080/foo")
	c.Assert(err, tc.ErrorMatches, ".*(connection refused|no such file or directory)")
}

// TestEnsureShutdown checks that a slow handler will not prevent a clean
// shutdown. An example of this, would be running a db query, that isn't letting
// the handler return immediately.
func (s *socketListenerSuite) TestEnsureShutdown(c *tc.C) {
	for i := 0; i < 100; i++ {
		tmpDir := c.MkDir()
		socket := path.Join(tmpDir, "test.socket")

		start := make(chan struct{})
		sl, err := socketlistener.NewSocketListener(socketlistener.Config{
			Logger:     s.logger,
			SocketName: socket,
			RegisterHandlers: func(r *mux.Router) {
				r.HandleFunc("/slow-handler", func(resp http.ResponseWriter, req *http.Request) {
					// Signal that the handler has started.
					close(start)
					time.Sleep(time.Second)
				}).Methods(http.MethodGet)
			},
			ShutdownTimeout: coretesting.LongWait,
		})
		c.Assert(err, tc.ErrorIsNil)
		defer workertest.DirtyKill(c, sl)
		var tomb tomb.Tomb
		tomb.Go(func() error {
			cl := client(socket)
			// Ignore error, as we're only interested in the fact that the request
			// was made.
			cl.Get("http://localhost:8080/slow-handler")
			return nil
		})

		tomb.Go(func() error {
			// Kill socket listener once handler has started.
			select {
			case <-start:
			case <-time.After(coretesting.ShortWait):
				return fmt.Errorf("took too long to start")
			}
			workertest.CleanKill(c, sl)
			return nil
		})
		// Wait for server to cleanly shutdown
		select {
		case <-tomb.Dead():
			c.Assert(tomb.Err(), tc.IsNil)
		case <-time.After(coretesting.LongWait):
			tomb.Kill(fmt.Errorf("took too long to finish"))
			c.Errorf("took too long to finish")
		}
	}
}

// Return an *http.Client with custom transport that allows it to connect to
// the given Unix socket.
func client(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (conn net.Conn, err error) {
				return sockets.Dialer(sockets.Socket{
					Network: "unix",
					Address: socketPath,
				})
			},
		},
	}
}
