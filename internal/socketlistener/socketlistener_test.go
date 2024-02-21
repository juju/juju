// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package socketlistener

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
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/core/testing"
)

type socketListenerSuite struct {
	logger *fakeLogger
}

var _ = gc.Suite(&socketListenerSuite{})

func (s *socketListenerSuite) SetUpTest(c *gc.C) {
	s.logger = &fakeLogger{}
}

func handleTestEndpoint1(resp http.ResponseWriter, req *http.Request) {
	resp.WriteHeader(http.StatusOK)
}

func registerTestHandlers(r *mux.Router) {
	r.HandleFunc("/test-endpoint", handleTestEndpoint1).
		Methods(http.MethodGet)
}

func (s *socketListenerSuite) TestStartStopWorker(c *gc.C) {
	tmpDir := c.MkDir()
	socket := path.Join(tmpDir, "test.socket")

	sl, err := NewSocketListener(Config{
		Logger:           s.logger,
		SocketName:       socket,
		RegisterHandlers: registerTestHandlers,
		ShutdownTimeout:  coretesting.LongWait,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check socket is created with correct permissions.
	fi, err := os.Stat(socket)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fi.Mode(), gc.Equals, fs.ModeSocket|0700)

	// Check server is up.
	cl := client(socket)
	resp, err := cl.Get("http://localhost:8080/foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)

	// Check server is serving.
	cl = client(socket)
	resp, err = cl.Get("http://localhost:8080/test-endpoint")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)

	sl.Kill()
	err = sl.Wait()
	c.Assert(err, jc.ErrorIsNil)

	// Check server has stopped.
	resp, err = cl.Get("http://localhost:8080/foo")
	c.Assert(err, gc.ErrorMatches, ".*connection refused")

	// No warnings/errors should have been logged.
	for _, entry := range s.logger.entries {
		if entry.level == "ERROR" || entry.level == "WARNING" {
			c.Errorf("%s: %s", entry.level, entry.msg)
		}
	}
}

// TestEnsureShutdown checks that a slow handler does not return an error if the
// socket listener is shutdown as it handles.
func (s *socketListenerSuite) TestEnsureShutdown(c *gc.C) {
	tmpDir := c.MkDir()
	socket := path.Join(tmpDir, "test.socket")

	start := make(chan struct{})
	sl, err := NewSocketListener(Config{
		Logger:     s.logger,
		SocketName: socket,
		RegisterHandlers: func(r *mux.Router) {
			r.HandleFunc("/slow-handler", func(resp http.ResponseWriter, req *http.Request) {
				// Signal that the handler has started.
				close(start)
				time.Sleep(time.Second)
				resp.WriteHeader(http.StatusOK)
			}).Methods(http.MethodGet)
		},
		ShutdownTimeout: coretesting.LongWait,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, sl)
	done := make(chan struct{})
	go func() {
		// Send request to slow handler and ensure it does not return error,
		// even though server is shut down as soon as it starts.
		cl := client(socket)
		_, err := cl.Get("http://localhost:8080/slow-handler")
		c.Assert(err, jc.ErrorIsNil)
	}()

	go func() {
		// Kill socket listener once handler has started.
		select {
		case <-start:
		case <-time.After(coretesting.ShortWait):
			c.Errorf("took too long to start")
		}
		workertest.CleanKill(c, sl)
		close(done)
	}()
	// Wait for server to cleanly shutdown
	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Errorf("took too long to finish")
	}
}

// Return an *http.Client with custom transport that allows it to connect to
// the given Unix socket.
func client(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (conn net.Conn, err error) {
				return net.Dial("unix", socketPath)
			},
		},
	}
}

type fakeLogger struct {
	entries []logEntry
}

type logEntry struct{ level, msg string }

func (f *fakeLogger) write(level string, format string, args ...any) {
	f.entries = append(f.entries, logEntry{level, fmt.Sprintf(format, args...)})
}

func (f *fakeLogger) Warningf(format string, args ...any) {
	f.write("WARNING", format, args...)
}

func (f *fakeLogger) Debugf(format string, args ...any) {
	f.write("DEBUG", format, args...)
}
