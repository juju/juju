// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"fmt"
	"net"
	"net/http"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/tomb.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/introspection/pprof"
)

var logger = loggo.GetLogger("juju.worker.introspection")

// DepEngineReporter provides insight into the running dependency engine of the agent.
type DepEngineReporter interface {
	// Report returns a map describing the state of the receiver. It is expected
	// to be goroutine-safe.
	Report() map[string]interface{}
}

// Config describes the arguments required to create the introspection worker.
type Config struct {
	SocketName string
	Reporter   DepEngineReporter
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
	reporter DepEngineReporter
	done     chan struct{}
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
		reporter: config.Reporter,
		done:     make(chan struct{}),
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
	mux.Handle("/depengine/", http.HandlerFunc(w.depengineReport))

	srv := http.Server{
		Handler: mux,
	}

	logger.Debugf("stats worker now servering")
	defer logger.Debugf("stats worker servering finished")
	defer close(w.done)
	srv.Serve(w.listener)
}

func (w *socketListener) run() {
	defer w.tomb.Done()
	defer logger.Debugf("stats worker finished")
	<-w.tomb.Dying()
	logger.Debugf("stats worker closing listener")
	w.listener.Close()
	// Don't mark the worker as done until the serve goroutine has finished.
	<-w.done
}

// Kill implements worker.Worker.
func (w *socketListener) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *socketListener) Wait() error {
	return w.tomb.Wait()
}

func (s *socketListener) depengineReport(w http.ResponseWriter, r *http.Request) {
	if s.reporter == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "missing reporter")
		return
	}
	bytes, err := yaml.Marshal(s.reporter.Report())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	fmt.Fprint(w, "Dependency Engine Report\n\n")
	w.Write(bytes)
}
