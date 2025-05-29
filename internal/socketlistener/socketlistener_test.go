// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package socketlistener_test

import (
	"context"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/gorilla/mux"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/logger"
	coretesting "github.com/juju/juju/core/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/socketlistener"
	"github.com/juju/juju/juju/sockets"
)

type socketListenerSuite struct {
	logger logger.Logger
}

func TestSocketListenerSuite(t *testing.T) {
	tc.Run(t, &socketListenerSuite{})
}

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
	tmpDir := c.MkDir()
	socket := path.Join(tmpDir, "test.socket")

	start := make(chan struct{})
	done := make(chan struct{})
	sl, err := socketlistener.NewSocketListener(socketlistener.Config{
		Logger:     s.logger,
		SocketName: socket,
		RegisterHandlers: func(r *mux.Router) {
			r.HandleFunc("/slow-handler", func(resp http.ResponseWriter, req *http.Request) {
				// Signal that the handler has started.
				close(start)
				<-done
			}).Methods(http.MethodGet)
		},
		ShutdownTimeout: coretesting.LongWait,
	})
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, sl)

	go func() {
		defer close(done)
		cl := client(socket)
		// Ignore error, as we're only interested in the fact that the request
		// was made.
		_, _ = cl.Get("http://localhost:8080/slow-handler")
	}()

	// Kill socket listener once handler has started.
	select {
	case <-start:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for listener to start")
	}
	workertest.CleanKill(c, sl)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timed out waiting for caller to exit")
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
