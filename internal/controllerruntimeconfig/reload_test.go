// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig_test

import (
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/controllerruntimeconfig"
	"github.com/juju/juju/internal/testhelpers"
)

type reloadSuite struct {
	testhelpers.IsolationSuite
}

func TestReloadSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &reloadSuite{})
	})
}

// newReloadSocket starts an HTTP server listening on a unix socket at the
// returned path and delegates to handler. The returned cleanup shuts the
// server down.
func newReloadSocket(c *tc.C, handler http.Handler) (socketPath string, cleanup func()) {
	dir := c.MkDir()
	socketPath = filepath.Join(dir, "configchange.socket")

	listener, err := net.Listen("unix", socketPath)
	c.Assert(err, tc.ErrorIsNil)

	srv := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(listener)
		close(done)
	}()
	cleanup = func() {
		_ = srv.Close()
		<-done
	}
	return socketPath, cleanup
}

// TestRequestReload_Success posts to the configchange socket and expects the
// worker to acknowledge with http.StatusNoContent.
func (s *reloadSuite) TestRequestReload_Success(c *tc.C) {
	var (
		gotMethod string
		gotPath   string
		calls     int
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	socketPath, cleanup := newReloadSocket(c, mux)
	defer cleanup()

	err := controllerruntimeconfig.RequestReload(socketPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(calls, tc.Equals, 1)
	c.Check(gotMethod, tc.Equals, http.MethodPost)
	c.Check(gotPath, tc.Equals, "/reload")
}

// TestRequestReload_ServerError returns an error when the worker responds
// with a status other than http.StatusNoContent.
func (s *reloadSuite) TestRequestReload_ServerError(c *tc.C) {
	mux := http.NewServeMux()
	mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	socketPath, cleanup := newReloadSocket(c, mux)
	defer cleanup()

	err := controllerruntimeconfig.RequestReload(socketPath)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "controller config reload via .* failed: 502 Bad Gateway")
}

// TestRequestReload_MissingSocket returns an error when no server is
// listening on the socket path.
func (s *reloadSuite) TestRequestReload_MissingSocket(c *tc.C) {
	dir := c.MkDir()
	missingPath := filepath.Join(dir, "no-such.socket")

	err := controllerruntimeconfig.RequestReload(missingPath)
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, "requesting controller config reload via .*")
}
