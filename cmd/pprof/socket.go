// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pprof

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.cmd.pprof")

// Filename contains the filename to use for the pprof unix socket for this
// process. The name is derived from the process name, as provided in
// os.Args[0], and the process ID.
var Filename = fmt.Sprintf(
	"pprof.%s.%d",
	filepath.Base(os.Args[0]),
	os.Getpid(),
)

// Start starts a pprof server listening on a unix socket which will be
// created at the specified path.
func Start(path string) func() error {
	if runtime.GOOS != "linux" {
		logger.Infof("pprof debugging not supported on %q", runtime.GOOS)
		return func() error { return nil }
	}

	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", http.HandlerFunc(Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(Symbol))

	srv := http.Server{
		Handler: mux,
	}

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		logger.Errorf("unable to resolve unix socket: %v", err)
		return func() error { return nil }
	}

	// Try to remove the socket if already present.
	os.Remove(path)

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		logger.Errorf("unable to listen on unix socket: %v", err)
		return func() error { return nil }
	}

	go func() {
		defer os.Remove(path)

		// Ignore the error from calling l.Close.
		srv.Serve(l)
	}()

	return l.Close
}
