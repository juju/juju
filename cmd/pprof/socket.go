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

// Start starts a pprof server listening on a unix socket in /tmp.
// The name of the file is derived from the name of the process, as
// provided by os.Args[0], and the pid of the process.
// Start returns a function which will stop the pprof server and clean
// up the socket file.
func Start() func() error {
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

	path := socketpath()
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

// socketpath returns the path for this processes' pprof socket.
func socketpath() string {
	cmd := filepath.Base(os.Args[0])
	name := fmt.Sprintf("pprof.%s.%d", cmd, os.Getpid())
	path := filepath.Join(os.TempDir(), name)
	return path
}
