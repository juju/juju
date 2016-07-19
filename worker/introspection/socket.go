// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"net"
	"net/http"
	"runtime"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/introspection/pprof"
)

// Config describes the arguments required to create the introspection worker.
type Config struct {
	SocketName string
}

// Validate checks the config values to assert they are valid to create the worker.
func (c *Config) Validate() error {
	if c.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	return nil
}

// socketListener is a worker and constructed with NewWorker.
type socketListener struct {
	tomb     tomb.Tomb
	listener *net.UnixListener
}

// NewWorker starts an http server listening on an abstract domain socket
// which will be created with the specified name.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if runtime.GOOS != "linux" {
		return nil, errors.NotSupportedf("os %q", runtime.GOOS)
	}

	path := "@" + config.SocketName
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, errors.Annotate(err, "unable to resolve unix socket")
	}

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, errors.Annotate(err, "unable to listen on unix socket")
	}
	logger.Debugf("introspection worker listening on %q", path)

	w := &socketListener{
		listener: l,
	}
	go w.serve()
	go w.run()
	return w, nil
}

func (w *socketListener) serve() {
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))

	srv := http.Server{
		Handler: mux,
	}

	logger.Debugf("stats worker now servering")
	srv.Serve(w.listener)
	logger.Debugf("stats worker servering finished")
}

func (w *socketListener) run() {
	<-w.tomb.Dying()
	logger.Debugf("stats worker closing listener")
	w.listener.Close()
	w.tomb.Done()
}

// Kill implements worker.Worker.
func (w *socketListener) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *socketListener) Wait() error {
	return w.tomb.Wait()
}
